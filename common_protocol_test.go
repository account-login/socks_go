package socks_go

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func doUDPProtocolTest(t *testing.T, addr SocksAddr, port uint16, data []byte, msg []byte) {
	assert.Equal(t, msg, MakeUDPMsg(addr, port, data))

	paddr, pport, pdata, err := ParseUDPMsg(msg)
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
