package socks_go

import "net"

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
	MethodNone     byte = 0
	MethodGSSApi   byte = 1
	MethodUserName byte = 2
	MethodReject   byte = 0xff
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
