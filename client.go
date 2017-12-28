package socks_go

import (
	"io"
	"net"

	"github.com/pkg/errors"
)

type Client struct {
	protocol     ClientProtocol
	authHandlers map[byte]ClientAuthHandlerFunc
}

type ClientAuthHandlerFunc func(proto *ClientProtocol) error

type ClientTunnel struct {
	io.ReadWriter
	BindAddr SocksAddr
	BindPort uint16
}

func NewClient(transport io.ReadWriter, authHandlers map[byte]ClientAuthHandlerFunc) Client {
	if len(authHandlers) == 0 {
		authHandlers = map[byte]ClientAuthHandlerFunc{
			MethodNone: clientNoAuthHandler,
		}
	}
	return Client{NewClientProtocol(transport), authHandlers}
}

func clientNoAuthHandler(proto *ClientProtocol) error {
	proto.AuthDone()
	return nil
}

func (c *Client) Connect(addr string) (tunnel ClientTunnel, err error) {
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

	// connect
	var tcpAddr *net.TCPAddr
	tcpAddr, err = net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return
	}

	err = c.protocol.SendCommand(CmdConnect, NewSocksAddrFromIP(tcpAddr.IP), uint16(tcpAddr.Port))
	if err != nil {
		return
	}

	var reply byte
	var bindAddr SocksAddr
	var bindPort uint16
	reply, bindAddr, bindPort, err = c.protocol.ReceiveReply()
	if err != nil {
		return
	}

	if reply != ReplyOK {
		err = errors.Errorf("bad reply from server: %#x", reply)
		return
	}

	tunnel = ClientTunnel{
		ReadWriter: c.protocol.GetConnection(),
		BindAddr:   bindAddr,
		BindPort:   bindPort,
	}
	return
}
