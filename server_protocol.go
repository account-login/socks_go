package socks_go

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// protocol state
const (
	PSInit = iota
	PSBad
	PSClose
	PSMethodsGot
	PSMethodSent
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

func readRequired(reader io.Reader, n int) (data []byte, err error) {
	data = make([]byte, n)
	_, err = io.ReadFull(reader, data)
	return
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
	buf, err = readRequired(proto.Transport, 2)
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
	methods, err = readRequired(proto.Transport, int(numMethods))
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
				proto.State = PSMethodSent
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

func (proto *ServerProtocol) GetRequest() (cmd byte, addr SocksAddr, port uint16, err error) {
	proto.checkState(PSMethodSent)
	defer func() {
		if err == nil {
			proto.State = PSReqConnectGot
		} else {
			proto.State = PSBad
		}
	}()

	return readReq(proto.Transport)
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

	err = writeResp(proto.Transport, ReplyOK, bindAddr, bindPort)
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

	return writeResp(proto.Transport, reply, NewSocksAddr(), 0)
}

func writeResp(writer io.Writer, reply byte, addr SocksAddr, port uint16) (err error) {
	data := make([]byte, 0, 10)
	data = append(data, 0x05, reply, 0)
	data = append(data, addr.ToBytes()...)
	data = append(data, 0, 0)
	binary.BigEndian.PutUint16(data[len(data)-2:], port)

	_, err = writer.Write(data)
	return
}

func readReq(reader io.Reader) (cmd byte, addr SocksAddr, port uint16, err error) {
	var buf []byte
	// ver
	buf, err = readRequired(reader, 4)
	ver := buf[0]
	if ver != 0x05 {
		err = errors.Errorf("bad version: %#x", ver)
		return
	}

	// cmd
	cmd = buf[1]

	// addr
	atype := buf[3]
	addr, err = readSocksAddr(atype, reader)
	if err != nil {
		err = errors.Wrap(err, "readSocksAddr() failed")
		return
	}

	// port
	buf, err = readRequired(reader, 2)
	if err != nil {
		err = errors.Wrap(err, "can not read port")
		return
	}
	port = binary.BigEndian.Uint16(buf)

	return
}

func readSocksAddr(atype byte, reader io.Reader) (addr SocksAddr, err error) {
	switch atype {
	case ATypeIPV4:
		addr.IP, err = readRequired(reader, 4)
		if err != nil {
			return
		}
	case ATypeIPV6:
		addr.IP, err = readRequired(reader, 16)
		if err != nil {
			return
		}
	case ATypeDomain:
		var buf []byte
		buf, err = readRequired(reader, 1)
		if err != nil {
			return
		}
		domainLen := buf[0]
		if domainLen <= 0 {
			err = errors.New("zero length domain")
			return
		}

		buf, err = readRequired(reader, int(domainLen))
		if err != nil {
			return
		}

		addr.Domain = string(buf)
	default:
		err = errors.Errorf("bad addr type: %#x", atype)
		return
	}

	addr.Type = atype
	return
}
