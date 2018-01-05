package socks_go

import (
	"io"

	"bytes"
	"encoding/binary"

	"github.com/account-login/socks_go/util"
	"github.com/pkg/errors"
)

// protocol state
const (
	PSInit = iota
	PSBad
	PSClose
	PSMethodsGot
	PSAuth
	PSAuthDone
	PSReqConnectGot
	PSReqUdpGot
	PSCmdConnect
	PSCmdUdp
)

type ServerProtocol struct {
	Transport io.ReadWriter
	State     int
}

func NewServerProtocol(transport io.ReadWriter) (proto ServerProtocol) {
	return ServerProtocol{transport, PSInit}
}

func (proto *ServerProtocol) checkState(expect int) {
	if proto.State != expect {
		panic("bad state")
	}
}

func (proto *ServerProtocol) GetAuthMethods() (methods []byte, err error) {
	proto.checkState(PSInit)
	// change protocol state on return
	defer func() {
		if err == nil {
			proto.State = PSMethodsGot
		} else {
			proto.State = PSBad
		}
	}()

	var buf []byte
	buf, err = util.ReadRequired(proto.Transport, 2)
	if err != nil {
		err = errors.Wrap(err, "can not read version and methods num")
		return
	}

	// version
	ver := buf[0]
	if ver != 0x05 {
		err = errors.Errorf("bad version: %#x", ver)
		return
	}

	// methods
	numMethods := buf[1]
	methods, err = util.ReadRequired(proto.Transport, int(numMethods))
	if err != nil {
		err = errors.Wrapf(err, "can not read methods. num: %d", numMethods)
		return
	}

	return
}

func (proto *ServerProtocol) AcceptAuthMethod(method byte) (err error) {
	proto.checkState(PSMethodsGot)
	defer func() {
		if err == nil {
			if method == MethodReject {
				proto.State = PSClose
			} else {
				proto.State = PSAuth
			}
		} else {
			proto.State = PSBad
		}
	}()

	_, err = proto.Transport.Write([]byte{0x05, method})
	return
}

func (proto *ServerProtocol) RejectAuthMethod() (err error) {
	return proto.AcceptAuthMethod(MethodReject)
}

func (proto *ServerProtocol) AuthDone() (err error) {
	proto.checkState(PSAuth)
	proto.State = PSAuthDone
	return
}

func (proto *ServerProtocol) GetRequest() (cmd byte, addr SocksAddr, port uint16, err error) {
	proto.checkState(PSAuthDone)
	defer func() {
		if err == nil {
			switch cmd {
			case CmdConnect:
				proto.State = PSReqConnectGot
			case CmdUDP:
				proto.State = PSReqUdpGot
			default:
				proto.State = PSBad // cmd not supported
			}
		} else {
			proto.State = PSBad
		}
	}()

	return readRequestOrReply(proto.Transport)
}

func (proto *ServerProtocol) AcceptConnection(bindAddr SocksAddr, bindPort uint16) (trans io.ReadWriter, err error) {
	proto.checkState(PSReqConnectGot)
	defer func() {
		if err == nil {
			proto.State = PSCmdConnect
		} else {
			proto.State = PSBad
		}
	}()

	err = writeResponseOrRequest(proto.Transport, ReplyOK, bindAddr, bindPort)
	if err != nil {
		return
	}
	trans = proto.Transport
	return
}

func (proto *ServerProtocol) AcceptUdpAssociation(bindAddr SocksAddr, bindPort uint16) (err error) {
	proto.checkState(PSReqUdpGot)
	defer func() {
		if err != nil {
			proto.State = PSCmdUdp
		} else {
			proto.State = PSBad
		}
	}()

	err = writeResponseOrRequest(proto.Transport, ReplyOK, bindAddr, bindPort)
	return
}

func (proto *ServerProtocol) RejectRequest(reply byte) (err error) {
	proto.checkState(PSReqConnectGot)
	defer func() {
		if err == nil {
			proto.State = PSClose
		} else {
			proto.State = PSBad
		}
	}()

	return writeResponseOrRequest(proto.Transport, reply, NewSocksAddr(), 0)
}

func ParseUDPProtocol(msg []byte) (addr SocksAddr, port uint16, data []byte, err error) {
	if len(msg) < 4+4+2 {
		err = errors.Errorf("udp request to short. size: %d", len(msg))
		return
	}

	// frag
	frag := msg[2]
	if frag != 0 {
		err = errors.Errorf("FRAG field not supported. frag: %d", frag)
	}

	// dst addr
	reader := bytes.NewReader(msg[4:])
	addr, err = readSocksAddr(msg[3], reader)
	if err != nil {
		err = errors.Wrapf(err, "can not read SocksAddr")
		return
	}

	// dst port
	var buf []byte
	buf, err = util.ReadRequired(reader, 2)
	if err != nil {
		err = errors.Wrap(err, "can not read port")
		return
	}
	port = binary.BigEndian.Uint16(buf)

	data = make([]byte, reader.Len())
	_, err = reader.Read(data) // err must be nil
	return
}

func MakeUDPProtocol(addr SocksAddr, port uint16, data []byte) (msg []byte) {
	msg = make([]byte, 0, 10+len(data))
	msg = append(msg, 0, 0, 0)
	msg = append(msg, addr.ToBytes()...)
	msg = append(msg, 0, 0)
	binary.BigEndian.PutUint16(msg[len(msg)-2:], port)
	msg = append(msg, data...)
	return
}
