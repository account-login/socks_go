package util

import (
	"net"
	"strconv"

	"github.com/pkg/errors"
)

func SplitHostPort(hostPort string) (host string, port uint16, err error) {
	var portStr string
	var portInt uint64
	host, portStr, err = net.SplitHostPort(hostPort)
	if err != nil {
		return
	}

	portInt, err = strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		err = errors.Wrapf(err, "SplitHostPort: can not parse port: %q", portStr)
		return
	}
	port = uint16(portInt)
	return
}
