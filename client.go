package socks_go

import (
	"io"

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
			MethodNone: ClientNoAuthHandler,
		}
	}
	return Client{NewClientProtocol(transport), authHandlers}
}

func ClientNoAuthHandler(proto *ClientProtocol) error {
	return nil
}

func (c *Client) ConnectSockAddr(sockAddr SocksAddr, port uint16) (tunnel ClientTunnel, err error) {
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

	err = c.protocol.AuthDone()
	if err != nil {
		return
	}

	//// connect
	//var tcpAddr *net.TCPAddr
	//tcpAddr, err = net.ResolveTCPAddr("tcp", addr)	// TODO: allow passing domain name to server
	//if err != nil {
	//	return
	//}

	err = c.protocol.SendCommand(CmdConnect, sockAddr, port)
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

func (c *Client) Connect(host string, port uint16) (tunnel ClientTunnel, err error) {
	return c.ConnectSockAddr(NewSocksAddrFromString(host), port)
}
