package socks_go

import (
	"encoding/binary"
	"io"
	"net"

	"bytes"

	"github.com/account-login/socks_go/util"
	"github.com/pkg/errors"
)

type SocksAddr struct {
	Type   byte
	IP     net.IP
	Domain string
}

func NewSocksAddr() SocksAddr {
	return SocksAddr{ATypeIPV4, net.IP{0, 0, 0, 0}, ""}
}

func NewSocksAddrFromIPV4(ip net.IP) SocksAddr {
	return SocksAddr{ATypeIPV4, ip, ""}
}

func NewSocksAddrFromIPV6(ip net.IP) SocksAddr {
	return SocksAddr{ATypeIPV6, ip, ""}
}

func NewSocksAddrFromDomain(domain string) SocksAddr {
	return SocksAddr{ATypeDomain, nil, domain}
}

func NewSocksAddrFromIP(ip net.IP) SocksAddr {
	if ip4 := ip.To4(); ip4 != nil {
		return NewSocksAddrFromIPV4(ip4)
	} else {
		return NewSocksAddrFromIPV6(ip.To16())
	}
}

func NewSocksAddrFromString(addr string) SocksAddr {
	if ip := net.ParseIP(addr); ip != nil {
		return NewSocksAddrFromIP(ip)
	} else {
		return NewSocksAddrFromDomain(addr)
	}
}

func (sa SocksAddr) ToBytes() (data []byte) {
	data = append(data, sa.Type)
	switch sa.Type {
	case ATypeIPV4:
		data = append(data, sa.IP.To4()...)
	case ATypeIPV6:
		data = append(data, sa.IP.To16()...)
	case ATypeDomain:
		if len(sa.Domain) > 256 {
			panic("domain name to long")
		}
		data = append(data, byte(len(sa.Domain)))
		data = append(data, sa.Domain...)
	default:
		panic("bad atype")
	}
	return
}

func (sa SocksAddr) String() string {
	switch sa.Type {
	case ATypeIPV4, ATypeIPV6:
		return sa.IP.String()
	case ATypeDomain:
		return sa.Domain
	default:
		panic("bad atype")
	}
}

const (
	MethodNone         byte = 0
	MethodGSSApi       byte = 1
	MethodUserName     byte = 2
	MethodPrivateBegin byte = 0x80
	MethodReject       byte = 0xff
)

const (
	CmdConnect byte = 1
	CmdBind    byte = 2
	CmdUDP     byte = 3
)

const (
	ATypeIPV4   byte = 1
	ATypeDomain byte = 3
	ATypeIPV6   byte = 4
)

const (
	ReplyOK              byte = 0
	ReplyFail            byte = 1
	ReplyCmdNotSupported byte = 7
)

func readSocksAddr(atype byte, reader io.Reader) (addr SocksAddr, err error) {
	switch atype {
	case ATypeIPV4:
		addr.IP, err = util.ReadRequired(reader, 4)
		if err != nil {
			return
		}
	case ATypeIPV6:
		addr.IP, err = util.ReadRequired(reader, 16)
		if err != nil {
			return
		}
	case ATypeDomain:
		var buf []byte
		buf, err = util.ReadRequired(reader, 1)
		if err != nil {
			return
		}
		domainLen := buf[0]
		if domainLen <= 0 {
			err = errors.New("zero length domain")
			return
		}

		buf, err = util.ReadRequired(reader, int(domainLen))
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

func readRequestOrReply(reader io.Reader) (cmdOrRep byte, addr SocksAddr, port uint16, err error) {
	var buf []byte
	// ver
	buf, err = util.ReadRequired(reader, 4)
	if err != nil {
		err = errors.Wrap(err, "readRequestOrReply: can not read header")
		return
	}

	ver := buf[0]
	if ver != 0x05 {
		err = errors.Errorf("readRequestOrReply: bad version: %#x, bytes: %v", ver, buf)
		return
	}

	// cmd or reply
	cmdOrRep = buf[1]

	// addr
	atype := buf[3]
	addr, err = readSocksAddr(atype, reader)
	if err != nil {
		err = errors.Wrap(err, "readSocksAddr() failed")
		return
	}

	// port
	buf, err = util.ReadRequired(reader, 2)
	if err != nil {
		err = errors.Wrap(err, "can not read port")
		return
	}
	port = binary.BigEndian.Uint16(buf)

	return
}

func writeResponseOrRequest(writer io.Writer, replyOrCmd byte, addr SocksAddr, port uint16) (err error) {
	data := make([]byte, 0, 10)
	data = append(data, 0x05, replyOrCmd, 0)
	data = append(data, addr.ToBytes()...)
	data = append(data, 0, 0)
	binary.BigEndian.PutUint16(data[len(data)-2:], port)

	_, err = writer.Write(data)
	return
}

func ParseUDPMsg(msg []byte) (addr SocksAddr, port uint16, data []byte, err error) {
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

func MakeUDPMsg(addr SocksAddr, port uint16, data []byte) (msg []byte) {
	msg = make([]byte, 0, 10+len(data))
	msg = append(msg, 0, 0, 0)
	msg = append(msg, addr.ToBytes()...)
	msg = append(msg, 0, 0)
	binary.BigEndian.PutUint16(msg[len(msg)-2:], port)
	msg = append(msg, data...)
	return
}
