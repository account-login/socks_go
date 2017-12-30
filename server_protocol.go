package socks_go

import (
	"io"

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
	PSCmdConnect
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
			proto.State = PSReqConnectGot
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
