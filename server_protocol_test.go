package socks_go

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	err = proto.AuthDone()
	require.NoError(t, err)

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

	err = proto.AuthDone()
	require.NoError(t, err)

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

func doUDPProtocolTest(t *testing.T, addr SocksAddr, port uint16, data []byte, msg []byte) {
	assert.Equal(t, msg, MakeUDPProtocol(addr, port, data))

	paddr, pport, pdata, err := ParseUDPProtocol(msg)
	require.NoError(t, err)
	assert.Equal(t, addr, paddr)
	assert.Equal(t, pport, pport)
	assert.Equal(t, pdata, data)
}

func TestUDPProtocol(t *testing.T) {
	doUDPProtocolTest(t,
		NewSocksAddrFromString("127.0.0.1"), 0x1234, []byte{0x56},
		[]byte{0, 0, 0, 0x01, 0x7f, 0, 0, 1, 0x12, 0x34, 0x56},
	)
}
