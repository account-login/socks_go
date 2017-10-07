package socks_go

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTransport struct {
	input  chan byte
	output []byte
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	size := len(p)
	if size == 0 {
		return
	}

	for b := range tr.input {
		p[n] = b
		n++
		size--
		if size == 0 {
			return
		}
	}

	if n < size {
		err = io.EOF
	}
	return
}

func (tr *fakeTransport) Write(p []byte) (n int, err error) {
	tr.output = append(tr.output, p...)
	n = len(p)
	return
}

func (tr *fakeTransport) Send(p []byte) {
	for _, b := range p {
		tr.input <- b
	}
}

func newFakeTransport() fakeTransport {
	return fakeTransport{make(chan byte, 1024), []byte{}}
}

func TestServerProtocol_Conversation(t *testing.T) {
	tr := newFakeTransport()
	proto := NewServerProtocol(&tr)

	// auth method
	tr.Send([]byte{0x05, 0x02, MethodNone, MethodUserName})

	methods, err := proto.GetAuthMethods()
	require.NoError(t, err)
	assert.Equal(t, []byte{MethodNone, MethodUserName}, methods)

	err = proto.AcceptAuthMethod(MethodNone)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x05, MethodNone}, tr.output)
	tr.output = []byte{}

	// req
	tr.Send([]byte{0x05, 0x01, 0x00, 0x01, 0x01, 0x02, 0x03, 0x04, 0x12, 0x34})

	cmd, addr, port, err := proto.GetRequest()
	require.NoError(t, err)
	assert.Equal(t, CmdConnect, cmd)
	assert.Equal(t, ATypeIPV4, addr.Type)
	assert.Equal(t, net.IP{1, 2, 3, 4}, addr.IP)
	assert.Equal(t, uint16(0x1234), port)

	// resp
	require.Empty(t, tr.output)
	tunnel, err := proto.AcceptConnection(NewSocksAddrFromIPV4(net.IP{2, 3, 4, 5}), uint16(0x2345))
	require.NoError(t, err)
	assert.Equal(t, []byte{0x05, 0x00, 0x00, 0x01, 2, 3, 4, 5, 0x23, 0x45}, tr.output)
	tr.output = []byte{}

	// read
	send := []byte{'a', 's', 'd', 'f'}
	tr.Send(send)
	buf := make([]byte, 4)
	_, err = tunnel.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, send, buf)

	// write
	buf = []byte{'1', '2', '3', '4'}
	_, err = tunnel.Write(buf)
	require.NoError(t, err)
	assert.Equal(t, buf, tr.output)
}

func TestServerProtocol_RejectAuthMethod(t *testing.T) {
	tr := newFakeTransport()
	proto := NewServerProtocol(&tr)

	// auth method
	tr.Send([]byte{0x05, 0x01, MethodNone})

	_, err := proto.GetAuthMethods()
	require.NoError(t, err)

	err = proto.RejectAuthMethod()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x05, MethodReject}, tr.output)
	assert.Equal(t, PSClose, proto.State)
}

func TestServerProtocol_RejectRequest(t *testing.T) {
	tr := newFakeTransport()
	proto := NewServerProtocol(&tr)

	// auth method
	tr.Send([]byte{0x05, 0x02, MethodNone, MethodUserName})

	_, err := proto.GetAuthMethods()
	err = proto.AcceptAuthMethod(MethodNone)
	require.NoError(t, err)
	tr.output = []byte{}

	// req
	tr.Send([]byte{0x05, 0x01, 0x00, 0x01, 0x01, 0x02, 0x03, 0x04, 0x12, 0x34})
	_, _, _, err = proto.GetRequest()
	require.NoError(t, err)

	// resp
	require.Empty(t, tr.output)
	err = proto.RejectRequest(ReplyCmdNotSupported)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0x00, 0x00}, tr.output)
}

func TestSocksAddr_ToBytes(t *testing.T) {
	assert.Equal(t,
		[]byte{0x03, 4, 'a', 's', 'd', 'f'},
		NewSocksAddrFromDomain("asdf").ToBytes())
	assert.Equal(t,
		[]byte{0x04, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		NewSocksAddrFromIPV6(net.IPv6loopback).ToBytes())
}

func TestReadSocksAddr(t *testing.T) {
	buf := []byte{0x03, 4, 'a', 's', 'd', 'f'}
	sa, err := readSocksAddr(buf[0], bytes.NewReader(buf[1:]))
	require.NoError(t, err)
	assert.Equal(t, NewSocksAddrFromDomain("asdf"), sa)
}
