package socks_go

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientProtocol_Conversation(t *testing.T) {
	tr := newFakeTransport()
	proto := NewClientProtocol(&tr)

	// auth
	proto.SendAuthMethods([]byte{MethodNone, MethodUserName})
	buf := []byte{0x05, 0x02, MethodNone, MethodUserName}
	assert.Equal(t, buf, tr.output)
	tr.output = []byte{}

	tr.Send([]byte{0x05, MethodNone})
	method, err := proto.ReceiveAuthMethod()
	require.NoError(t, err)
	assert.Equal(t, MethodNone, method)

	err = proto.AuthDone()
	require.NoError(t, err)

	// connect cmd
	sendaddr := NewSocksAddrFromIPV4(net.IP{2, 3, 4, 5})
	err = proto.SendCommand(CmdConnect, sendaddr, uint16(0x2345))
	require.NoError(t, err)
	assert.Equal(t, []byte{0x05, CmdConnect, 0x00, 0x01, 2, 3, 4, 5, 0x23, 0x45}, tr.output)
	tr.output = []byte{}

	tr.Send([]byte{0x05, ReplyOK, 0})
	tr.Send(sendaddr.ToBytes())
	tr.Send([]byte{0x23, 0x45})
	reply, addr, port, err := proto.ReceiveReply()
	require.NoError(t, err)
	assert.Equal(t, ReplyOK, reply)
	assert.Equal(t, sendaddr, addr)
	assert.Equal(t, uint16(0x2345), port)

	// tunnel
	tunnel := proto.GetConnection()
	tr.Send([]byte{1, 2, 3})
	buf, err = readRequired(tunnel, 3)
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, buf)
}
