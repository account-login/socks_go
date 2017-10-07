package socks_go

import (
	"fmt"
	"io"
	"net"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type Server struct {
	Addr string
}

func (s *Server) Run() (err error) {
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return
	}

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

		log.Infof("client: %v gone", conn.RemoteAddr())
	}()

	proto := NewServerProtocol(conn)
	_, err = proto.GetAuthMethods()
	if err != nil {
		return
	}

	err = proto.AcceptAuthMethod(MethodNone)
	if err != nil {
		return
	}

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
		proto.RejectRequest(ReplyCmdNotSupported)
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
		err = errors.Wrapf(err, "can not parse LocalAddr: $v", targetConn.LocalAddr())
		return
	}

	var tunnel io.ReadWriter
	tunnel, err = proto.AcceptConnection(bindAddr, bindPort)
	if err != nil {
		return
	}

	cr, cw := make(chan error), make(chan error)
	go bridgeReaderWriter(tunnel, targetConn, cr)
	go bridgeReaderWriter(targetConn, tunnel, cw)

	// wait for client or target
	merr := makeMultipleErrors()
	select {
	case rerr := <-cr:
		merr.Add("ReadClient", rerr)
		merr.Add("WriteClient", <-cr)
	case rerr := <-cw:
		merr.Add("ReadTarget", rerr)
		merr.Add("WriteClient", <-cw)
	}
	err = merr.ToError()

	return
}

type multipleErrors map[string]error

func (merr *multipleErrors) Error() string {
	if len(*merr) == 1 {
		for k, err := range *merr {
			return fmt.Sprintf("%s: %v", k, err)
		}
	}

	errstr := "Multiple errors:\n"
	for k, err := range *merr {
		errstr += fmt.Sprintf("\t%s: %v", k, err)
	}
	return errstr
}

func makeMultipleErrors() multipleErrors {
	return multipleErrors(make(map[string]error))
}

func (merr *multipleErrors) Add(key string, err error) {
	if err != nil {
		(*merr)[key] = err
	}
}

func (merr *multipleErrors) ToError() (err error) {
	if len(*merr) > 0 {
		return merr
	} else {
		return nil
	}
}

func bridgeReaderWriter(reader io.Reader, writer io.Writer, errchan chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		var werr error
		if n > 0 {
			_, werr = writer.Write(buf[:n])
		}

		if err != nil || werr != nil {
			rerr := err
			if rerr == io.EOF {
				rerr = nil
			}

			errchan <- rerr
			errchan <- werr
			return
		}
	}
}
