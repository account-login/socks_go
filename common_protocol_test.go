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
