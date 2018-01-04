package socks_go

import (
	"fmt"
	"io"
	"net"

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
	default:
		err = errors.Errorf("unsupported cmd: %#x", cmd)
		proto.RejectRequest(ReplyCmdNotSupported) // ignore err
	}
	return
}

func makeConnection(addr SocksAddr, port uint16) (net.Conn, error) {
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

	if ip := rawIP.To4(); ip != nil {
		addr = NewSocksAddrFromIPV4(ip)
	} else if ip := rawIP.To16(); ip != nil {
		addr = NewSocksAddrFromIPV6(ip)
	} else {
		err = errors.Errorf("bad ip: %v", rawIP)
	}
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
