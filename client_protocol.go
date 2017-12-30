package socks_go

import (
	"io"

	"github.com/account-login/socks_go/util"
	"github.com/pkg/errors"
)

// client protocol state
const (
	PSCInit = iota
	PSCBad
	PSCClose
	PSCMethodsSent
	PSCAuth
	PSCAuthDone
	PSCReqConnectSent
	PSCReplyConectGot
	PSCCmdConnected
)

type ClientProtocol struct {
	Transport io.ReadWriter
	State     int
}

func NewClientProtocol(transport io.ReadWriter) ClientProtocol {
	return ClientProtocol{transport, PSCInit}
}

func (proto *ClientProtocol) checkState(expect int) {
	if proto.State != expect {
		panic("bad state")
	}
}

func (proto *ClientProtocol) SendAuthMethods(methods []byte) (err error) {
	proto.checkState(PSCInit)
	defer func() {
		if err == nil {
			proto.State = PSCMethodsSent
		} else {
			proto.State = PSCBad
		}
	}()

	data := make([]byte, 0, 2+len(methods))
	data = append(data, 0x05)
	data = append(data, byte(len(methods)))
	data = append(data, methods...)
	_, err = proto.Transport.Write(data)
	return
}

func (proto *ClientProtocol) ReceiveAuthMethod() (method byte, err error) {
	proto.checkState(PSCMethodsSent)
	defer func() {
		if err == nil {
			if method == MethodReject {
				proto.State = PSCClose
			} else {
				proto.State = PSCAuth
			}
		} else {
			proto.State = PSCBad
		}
	}()

	var buf []byte
	buf, err = util.ReadRequired(proto.Transport, 2)
	if err != nil {
		err = errors.Wrap(err, "ReceiveAuthMethod: can not read data")
		return
	}

	ver := buf[0]
	if ver != 0x05 {
		err = errors.Errorf("ReceiveAuthMethod: bad version: %#x", ver)
		return
	}

	method = buf[1]
	return
}

func (proto *ClientProtocol) AuthDone() error {
	proto.checkState(PSCAuth)
	proto.State = PSCAuthDone
	return nil
}

func (proto *ClientProtocol) SendCommand(cmd byte, addr SocksAddr, port uint16) (err error) {
	proto.checkState(PSCAuthDone)
	defer func() {
		if err == nil {
			proto.State = PSCReqConnectSent
		} else {
			proto.State = PSCBad
		}
	}()

	return writeResponseOrRequest(proto.Transport, cmd, addr, port)
}

func (proto *ClientProtocol) ReceiveReply() (reply byte, addr SocksAddr, port uint16, err error) {
	proto.checkState(PSCReqConnectSent)
	defer func() {
		if err == nil {
			if reply == ReplyOK {
				proto.State = PSCReplyConectGot
			} else {
				proto.State = PSCClose
			}
		} else {
			proto.State = PSCBad
		}
	}()

	return readRequestOrReply(proto.Transport)
}

func (proto *ClientProtocol) GetConnection() (trans io.ReadWriter) {
	proto.checkState(PSCReplyConectGot)
	proto.State = PSCCmdConnected
	return proto.Transport
}
