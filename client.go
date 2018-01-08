package socks_go

import (
	"io"

	log "github.com/cihub/seelog"

	"net"
	"time"

	"github.com/pkg/errors"
)

type Client struct {
	protocol     ClientProtocol
	authHandlers map[byte]ClientAuthHandlerFunc
}

type ClientAuthHandlerFunc func(proto *ClientProtocol) error

// TODO: implement net.Conn
type ClientTunnel struct {
	io.ReadWriter
	BindAddr SocksAddr
	BindPort uint16
}

func NewClient(transport io.ReadWriter, authHandlers map[byte]ClientAuthHandlerFunc) Client {
	if len(authHandlers) == 0 {
		authHandlers = map[byte]ClientAuthHandlerFunc{
			MethodNone: ClientNoAuthHandler,
		}
	}
	return Client{NewClientProtocol(transport), authHandlers}
}

func ClientNoAuthHandler(proto *ClientProtocol) error {
	return nil
}

func (c *Client) doAuth() (err error) {
	// send auth methods
	methods := make([]byte, 0, len(c.authHandlers))
	var method byte
	for method = range c.authHandlers {
		methods = append(methods, method)
	}

	err = c.protocol.SendAuthMethods(methods)
	if err != nil {
		return
	}

	// auth methods selected by server
	method, err = c.protocol.ReceiveAuthMethod()
	if err != nil {
		return
	}

	if method == MethodReject {
		err = errors.Errorf("methods rejected by server: %v", methods)
		return
	}

	// handle auth
	handler, ok := c.authHandlers[method]
	if !ok {
		err = errors.Errorf("method not implemented by client: %#x", method)
		return
	}

	err = handler(&c.protocol)
	if err != nil {
		return
	}

	err = c.protocol.AuthDone()
	return
}

func (c *Client) ConnectSockAddr(sockAddr SocksAddr, port uint16) (tunnel ClientTunnel, err error) {
	err = c.doAuth()
	if err != nil {
		return
	}

	err = c.protocol.SendCommand(CmdConnect, sockAddr, port)
	if err != nil {
		return
	}

	var reply byte
	reply, tunnel.BindAddr, tunnel.BindPort, err = c.protocol.ReceiveReply()
	if err != nil {
		return
	}

	if reply != ReplyOK {
		err = errors.Errorf("bad reply from server: %#x", reply)
		return
	}

	tunnel.ReadWriter = c.protocol.GetConnection()
	return
}

func (c *Client) Connect(host string, port uint16) (tunnel ClientTunnel, err error) {
	// TODO: allow resolve host on local machine
	return c.ConnectSockAddr(NewSocksAddrFromString(host), port)
}

func (c *Client) UDPAssociation() (tunnel ClientUDPTunnel, err error) {
	err = c.doAuth()
	if err != nil {
		return
	}

	err = c.protocol.SendCommand(CmdUDP, NewSocksAddr(), 0)
	if err != nil {
		return
	}

	var reply byte
	reply, tunnel.BindAddr, tunnel.BindPort, err = c.protocol.ReceiveReply()
	if err != nil {
		return
	}

	if reply != ReplyOK {
		err = errors.Errorf("bad reply from server: %#x", reply)
		return
	}

	tunnel.server = &net.UDPAddr{IP: tunnel.BindAddr.IP, Port: int(tunnel.BindPort)}
	if tunnel.server.IP.IsUnspecified() {
		// fix 0.0.0.0 address
		type HasRemoteAddr interface {
			RemoteAddr() net.Addr
		}

		if remoteTrans, ok := c.protocol.Transport.(HasRemoteAddr); ok {
			serverAddr := remoteTrans.RemoteAddr()
			if tcpAddr, ok := serverAddr.(*net.TCPAddr); ok {
				tunnel.server.IP = tcpAddr.IP
			}
		}
	}
	//log.Debugf("server udp addr: %v", tunnel.server)

	// create udp sockets
	tunnel.conn, err = net.ListenUDP("udp", nil)
	if err != nil {
		return
	}

	// monitor tcp connection
	tunnel.ctrlChannel = make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		trans := c.protocol.Transport

		type HasRemoteAddr interface {
			RemoteAddr() net.Addr
		}

		var remoteTCPAddr net.Addr
		if remoteConn, ok := trans.(HasRemoteAddr); ok {
			remoteTCPAddr = remoteConn.RemoteAddr()
		}

		for {
			n, tcpErr := trans.Read(buf) // TODO: timeout?
			if n != 0 {
				log.Warnf("server: %v, data received after udp association cmd", remoteTCPAddr)
			}

			if tcpErr != nil {
				tunnel.ctrlChannel <- tcpErr // maybe io.EOF
				return
			}
		}
	}()

	return
}

// implements net.PacketConn io.ReadWriteCloser
type ClientUDPTunnel struct {
	BindAddr SocksAddr
	BindPort uint16

	server      *net.UDPAddr
	conn        *net.UDPConn
	ctrlChannel chan error
}

func (ut *ClientUDPTunnel) checkCtrlChannel() (done bool, err error) {
	select {
	case err = <-ut.ctrlChannel:
		done = true
	default:
		done = false
	}
	return
}

func (ut *ClientUDPTunnel) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	var done bool
	done, err = ut.checkCtrlChannel()
	if done {
		err = errors.Wrapf(err, "TCP control connection closed, can not receive packet")
		return
	}

	// read packet
	nread, _, perr := ut.conn.ReadFrom(b)
	if perr != nil {
		err = perr
		return
	}

	// parse packet
	sockAddr, port, data, perr := ParseUDPMsg(b[:nread])
	if perr != nil {
		err = perr
		return
	}

	addr = &net.UDPAddr{IP: sockAddr.IP, Port: int(port)}
	n = len(data)
	copy(b, data)
	return
}

func (ut *ClientUDPTunnel) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		err = errors.Errorf("requires net.UDPAddr, %v (%T) got", addr, addr)
		return
	}
	return ut.WriteToSocksAddr(b, NewSocksAddrFromIP(udpAddr.IP), uint16(udpAddr.Port))
}

func (ut *ClientUDPTunnel) WriteToSocksAddr(b []byte, addr SocksAddr, port uint16) (n int, err error) {
	var done bool
	done, err = ut.checkCtrlChannel()
	if done {
		err = errors.Wrapf(err, "TCP control connection closed, can not send packet")
		return
	}

	msg := MakeUDPMsg(addr, port, b)
	n, err = ut.conn.WriteTo(msg, ut.server)
	headerLen := len(msg) - len(b)
	n -= headerLen
	return
}

func (ut *ClientUDPTunnel) Close() error {
	return ut.conn.Close()
}

func (ut *ClientUDPTunnel) LocalAddr() net.Addr {
	return ut.conn.LocalAddr()
}

func (ut *ClientUDPTunnel) SetDeadline(t time.Time) error {
	return ut.conn.SetDeadline(t)
}

func (ut *ClientUDPTunnel) SetReadDeadline(t time.Time) error {
	return ut.conn.SetReadDeadline(t)
}

func (ut *ClientUDPTunnel) SetWriteDeadline(t time.Time) error {
	return ut.conn.SetWriteDeadline(t)
}
