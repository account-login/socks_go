package socks_go

import (
	"fmt"
	"io"
	"net"

	"bytes"

	"github.com/account-login/socks_go/util"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type AuthHandlerFunc func(methods []byte, proto *ServerProtocol) error

type Server struct {
	Addr        string
	AuthHandler AuthHandlerFunc
}

func NewServer(addr string, authHandler AuthHandlerFunc) Server {
	if authHandler == nil {
		authHandler = noAuthHandler
	}
	return Server{addr, authHandler}
}

func noAuthHandler(methods []byte, proto *ServerProtocol) error {
	return proto.AcceptAuthMethod(MethodNone)
}

// TODO: quitable?
func (s *Server) Run() (err error) {
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return
	}
	log.Infof("server started on %v", listener.Addr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("Accept failed: %v", err)
			continue
		} else {
			log.Infof("Accept %v", conn.RemoteAddr())
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	var err error

	defer func() {
		if err != nil {
			log.Errorf("client: %v, err: %v", conn.RemoteAddr(), err)
		}

		err = conn.Close()
		if err != nil {
			log.Errorf("client: %v, close err: %v", conn.RemoteAddr(), err)
		}

		log.Infof("client: %v, gone", conn.RemoteAddr())
	}()

	proto := NewServerProtocol(conn)

	// auth
	var methods []byte
	methods, err = proto.GetAuthMethods()
	if err != nil {
		return
	}

	err = s.AuthHandler(methods, &proto)
	if err != nil {
		return
	}

	err = proto.AuthDone()
	if err != nil {
		return
	}

	// request
	var cmd byte
	var addr SocksAddr
	var port uint16
	cmd, addr, port, err = proto.GetRequest()
	if err != nil {
		return
	}

	switch cmd {
	case CmdConnect:
		log.Infof("client: %v, cmd: connect, target: %v:%d", conn.RemoteAddr(), addr, port)
		err = s.cmdConnect(conn, &proto, addr, port)
	case CmdUDP:
		log.Infof("client: %v, cmd: udp, client_from: %v:%d", conn.RemoteAddr(), addr, port)
		err = s.cmdUDP(conn, &proto, addr, port)
	default:
		err = errors.Errorf("unsupported cmd: %#x", cmd)
		proto.RejectRequest(ReplyCmdNotSupported) // ignore err
	}
	return
}

func makeConnection(addr SocksAddr, port uint16) (net.Conn, error) {
	// TODO: timeout
	return net.Dial("tcp", fmt.Sprintf("%v:%d", addr, port))
}

func parseNetAddr(netAddr net.Addr) (addr SocksAddr, port uint16, err error) {
	var rawIP net.IP
	switch concreteAddr := netAddr.(type) {
	case *net.TCPAddr:
		rawIP = concreteAddr.IP
		port = uint16(concreteAddr.Port)
	case *net.UDPAddr:
		rawIP = concreteAddr.IP
		port = uint16(concreteAddr.Port)
	default:
		err = errors.Errorf("unknown addr: %v (%T)", netAddr, netAddr)
		return
	}

	addr = NewSocksAddrFromIP(rawIP)
	return
}

func (s *Server) cmdConnect(conn net.Conn, proto *ServerProtocol, addr SocksAddr, port uint16) (err error) {
	var targetConn net.Conn

	defer func() {
		if targetConn != nil {
			closeErr := targetConn.Close()
			if closeErr != nil {
				log.Errorf("close target conn err: %v", closeErr)
			}
		}
	}()

	targetConn, err = makeConnection(addr, port)
	if err != nil {
		return
	}
	log.Infof("connected to %v from %v", targetConn.RemoteAddr(), targetConn.LocalAddr())

	var bindAddr SocksAddr
	var bindPort uint16
	bindAddr, bindPort, err = parseNetAddr(targetConn.LocalAddr())
	if err != nil {
		err = errors.Wrapf(err, "can not parse LocalAddr: %v", targetConn.LocalAddr())
		return
	}

	var tunnel io.ReadWriter
	tunnel, err = proto.AcceptConnection(bindAddr, bindPort)
	if err != nil {
		return
	}

	cr := util.BridgeReaderWriter(tunnel, targetConn)
	cw := util.BridgeReaderWriter(targetConn, tunnel)

	// wait for client or target
	merr := util.NewMultipleErrors()
	select {
	case rerr := <-cr:
		merr.Add("ReadClient", rerr)
		merr.Add("WriteTarget", <-cr)
	case rerr := <-cw:
		log.Infof("target gone: %v", targetConn.RemoteAddr())
		merr.Add("ReadTarget", rerr)
		merr.Add("WriteClient", <-cw)
	}
	err = merr.ToError()

	return
}

func doClose(closer io.Closer, closed *bool, msg string) {
	if closer == nil || *closed {
		return
	}

	err := closer.Close()
	if err != nil {
		log.Errorf("close %s err: %v", msg, err)
	}
	*closed = true
}

func (s *Server) cmdUDP(conn net.Conn, proto *ServerProtocol, addr SocksAddr, port uint16) (err error) {
	// udp sockets will be close when:
	// 	a. tcp connnection is finished (success or not)
	//  b. reading/writing error on udp sockets
	//  c. other error before entering main loop
	var clientConn, remoteConn *net.UDPConn
	clientConnClosed, remoteConnClosed := false, false

	// clean up udp socket
	defer func() {
		// condition c
		doClose(remoteConn, &clientConnClosed, "remote udp conn")
		doClose(clientConn, &remoteConnClosed, "client udp conn")
	}()

	// create udp socket
	clientConn, err = net.ListenUDP("udp", nil)
	if err != nil {
		err = errors.Wrapf(err, "error creating client udp socket")
		return
	}
	remoteConn, err = net.ListenUDP("udp", nil)
	if err != nil {
		err = errors.Wrapf(err, "error creating remote udp socket")
		return
	}
	log.Infof("client: %v, client_udp_listen: %v, remote_udp_listen: %v",
		conn.RemoteAddr(), clientConn.LocalAddr(), remoteConn.LocalAddr())

	bindAddr, bindPort, parseErr := parseNetAddr(clientConn.LocalAddr())
	if parseErr != nil { // unlikely to happen
		err = errors.Wrapf(parseErr, "can not parse LocalAddr: %v", clientConn.LocalAddr())
		return
	}

	// reply client
	err = proto.AcceptUdpAssociation(bindAddr, bindPort)
	if err != nil {
		return
	}

	// monitor tcp connection
	ctrlChannel := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		for {
			n, tcpErr := conn.Read(buf) // TODO: timeout?
			if n != 0 {
				log.Warnf("client: %v, data received after udp association cmd", conn.RemoteAddr())
			}

			if tcpErr != nil {
				if tcpErr == io.EOF {
					tcpErr = nil
				}
				ctrlChannel <- tcpErr
				return
			}
		}
	}()

	// main loop
	clientChannel := readUDP(clientConn)
	remoteChannel := readUDP(remoteConn)
	var clientAddr *net.UDPAddr

	for remoteChannel != nil || clientChannel != nil {
		select {
		case err = <-ctrlChannel:
			if err == nil {
				log.Debugf("client: %v, udp client leave", conn.RemoteAddr())
			} else {
				err = errors.Wrapf(err, "client tcp conn broken")
			}
			ctrlChannel = nil
			// condition a: tcp connection finished, close udp socket
		case clientEvent := <-clientChannel:
			if clientEvent.err != nil { // terminate client udp socket
				if err != nil {
					err = errors.Wrapf(err, "client udp read error")
				}
				clientChannel = nil
				break
			}

			if err != nil {
				break
			}

			log.Debugf("client: %v, client udp: %v, got data from client", conn.RemoteAddr(), clientEvent.addr)

			// set clientAddr
			if clientAddr != nil && !UDPAddrEqual(clientAddr, clientEvent.addr) {
				log.Errorf(
					"client: %v, client udp source addr changing from %v to %v",
					conn.RemoteAddr(), *clientAddr, *clientEvent.addr)
			}
			clientAddr = clientEvent.addr

			// parse protocol
			sockAddr, port, data, parseErr := ParseUDPMsg(clientEvent.data)
			if parseErr != nil {
				err = errors.Wrapf(parseErr, "ParseUDPMsg error")
				break
			}

			// find out destination addr
			// TODO: create domain to ip mapping
			var toAddr *net.UDPAddr
			toAddr, err = socksAddrToUDPAddr(sockAddr, port)
			if err != nil {
				err = errors.Wrapf(err, "socksAddrToUDPAddr error")
				break
			}
			log.Debugf("client: %v, remote udp dest: %v", conn.RemoteAddr(), toAddr)

			// fwd data
			var n int
			n, err = remoteConn.WriteToUDP(data, toAddr)
			if err != nil {
				err = errors.Wrapf(err, "remote udp write error")
				break
			}
			if n != len(data) {
				log.Warnf("client: %v, udp short write to remote: %d of %d bytes",
					conn.RemoteAddr(), n, len(data))
			}
		case remoteEvent := <-remoteChannel:
			if remoteEvent.err != nil { // terminate remote udp socket
				if err != nil {
					err = errors.Wrapf(err, "remote udp read error")
				}
				remoteChannel = nil
				break
			}

			if err != nil {
				break
			}

			log.Debugf("client: %v, remote udp: %v, got data from remote", conn.RemoteAddr(), remoteEvent.addr)
			if clientAddr == nil {
				log.Warnf("client: %v, got data from remote udp %v, but clientAddr == nil, data: %v",
					conn.RemoteAddr(), remoteEvent.addr, remoteEvent.data)
				break
			}

			// TODO: map ip to domain if client uses domain instead of ip
			// FIXME: fix wildcard ip
			packed := MakeUDPMsg(
				NewSocksAddrFromIP(remoteEvent.addr.IP), uint16(remoteEvent.addr.Port),
				remoteEvent.data,
			)

			// fwd data
			var n int
			n, err = clientConn.WriteToUDP(packed, clientAddr)
			if err != nil {
				err = errors.Wrapf(err, "client udp write error")
				break
			}
			if n != len(packed) {
				log.Warnf("client: %v, udp short write to client: %d of %d bytes",
					conn.RemoteAddr(), n, len(packed))
			}
		} // select

		if err != nil || ctrlChannel == nil {
			// condition a & b: close both udp sockets to finishing clientChannel and remoteChannel
			doClose(remoteConn, &clientConnClosed, "remote udp conn")
			doClose(clientConn, &remoteConnClosed, "client udp conn")
		}
	} // while udp socket not finished

	return
}

func socksAddrToUDPAddr(sockAddr SocksAddr, port uint16) (*net.UDPAddr, error) {
	addr := &net.UDPAddr{}
	addr.Port = int(port)

	addr.IP = sockAddr.IP
	if sockAddr.Type == ATypeDomain {
		ipAddr, err := net.ResolveIPAddr("ip", sockAddr.Domain)
		if err != nil {
			return nil, errors.Wrapf(err, "socksAddrToUDPAddr: ResolveIPAddr error for %q", sockAddr.Domain)
		}
		addr.IP = ipAddr.IP
		addr.Zone = ipAddr.Zone // may be this is useless
	}

	// normalize ip
	if ip4 := addr.IP.To4(); ip4 != nil {
		addr.IP = ip4
	}
	return addr, nil
}

func UDPAddrEqual(a, b *net.UDPAddr) bool {
	return bytes.Equal(a.IP, b.IP) && a.Port == b.Port && a.Zone == b.Zone
}

type UDPReadEvent struct {
	data []byte
	addr *net.UDPAddr
	err  error
}

func readUDP(conn *net.UDPConn) chan UDPReadEvent {
	ch := make(chan UDPReadEvent)
	go func() {
		for {
			buf := make([]byte, 64*1024)
			n, clientAddr, err := conn.ReadFromUDP(buf)
			ch <- UDPReadEvent{buf[0:n], clientAddr, err}
			if err != nil {
				return
			}
		}
	}()
	return ch
}
