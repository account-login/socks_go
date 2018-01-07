package main

import (
	"flag"
	"net"
	"os"

	"github.com/account-login/socks_go"
	"github.com/account-login/socks_go/cmd"
	"github.com/account-login/socks_go/util"
	log "github.com/cihub/seelog"
)

const MethodMyExtended = socks_go.MethodPrivateBegin + 1

func extendedAuthHandler(proto *socks_go.ClientProtocol) (err error) {
	var ip net.IP
	ip, err = util.ReadRequired(proto.Transport, 4)
	if err == nil {
		log.Infof("ip echo: %v", ip)
	}
	return err
}

func realMain() int {
	// logging
	defer log.Flush()
	cmd.ConfigLogging()

	// parse args
	proxyArg := flag.String("proxy", "127.0.0.1:1080", "socks5 proxy server")
	flag.Parse()
	target := flag.Arg(0)
	if len(target) == 0 {
		log.Errorf("must specify target address")
		return 1
	}

	host, port, err := util.SplitHostPort(target)
	if err != nil {
		log.Errorf("can not parse host:port: %s", target)
		return 4
	}

	// clean up connection
	var conn net.Conn
	var connClosed = false
	doClose := func() {
		if conn != nil && !connClosed {
			err := conn.Close()
			if err != nil {
				log.Errorf("close proxy connection error: %v", err)
			}
			connClosed = true
		}
	}
	defer doClose()

	// connect to proxy server
	conn, err = net.Dial("tcp", *proxyArg)
	if err != nil {
		log.Errorf("Dial to proxy failed: %v", err)
		return 2
	}

	// make socks5 client
	client := socks_go.NewClient(
		conn,
		map[byte]socks_go.ClientAuthHandlerFunc{
			socks_go.MethodNone: socks_go.ClientNoAuthHandler,
			MethodMyExtended:    extendedAuthHandler,
		},
	)

	// issue command to server
	tunnel, err := client.Connect(host, port)
	if err != nil {
		log.Errorf("client.Connect(%v) failed: %v", target, err)
		return 3
	}

	// tunnel stdin and stdout through proxy
	l2r := util.BridgeReaderWriter(os.Stdin, tunnel)
	r2l := util.BridgeReaderWriter(tunnel, os.Stdout)

	// error handling
	retCode := 0
	handleErr := func(err error, dir string, role string) {
		if err != nil {
			// logging
			retCode = 3
			log.Errorf("%s %s error: %v", dir, role, err)
			// close connection to proxy on error
			doClose()
		}
	}

	// poll for l2r & r2l
	for i := 0; i < 2; i++ {
		select {
		case rerr := <-l2r:
			handleErr(rerr, "local to remote", "reader")
			handleErr(<-l2r, "local to remote", "writer")
			l2r = nil
		case rerr := <-r2l:
			handleErr(rerr, "remote to local", "reader")
			handleErr(<-r2l, "remote to local", "writer")
			r2l = nil
		}
	}

	return retCode
}

func main() {
	os.Exit(realMain())
}
