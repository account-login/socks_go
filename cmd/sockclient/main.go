package main

import "os"

import (
	"flag"
	"io"
	"net"

	"github.com/account-login/socks_go"
	log "github.com/cihub/seelog"
)

// copied from server.go
func bridgeReaderWriter(reader io.Reader, writer io.Writer, errchan chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		var werr error
		if n > 0 {
			_, werr = writer.Write(buf[:n])
		}

		if err != nil || werr != nil {
			rerr := err
			if rerr == io.EOF {
				rerr = nil
			}

			errchan <- rerr
			errchan <- werr
			return
		}
	}
}

func realMain() int {
	defer log.Flush()

	proxyArg := flag.String("proxy", "127.0.0.1:1080", "socks5 proxy server")
	flag.Parse()
	target := flag.Arg(0)
	if len(target) == 0 {
		log.Errorf("must specify target address")
		return 1
	}

	conn, err := net.Dial("tcp", *proxyArg)
	if err != nil {
		log.Errorf("Dial to proxy failed: %v", err)
		return 2
	}

	client := socks_go.NewClient(conn, nil)
	tunnel, err := client.Connect(target)
	if err != nil {
		log.Errorf("client.Connect(%v) failed: %v", target, err)
		return 3
	}

	l2r := make(chan error)
	r2l := make(chan error)
	go bridgeReaderWriter(os.Stdin, tunnel, l2r)
	go bridgeReaderWriter(tunnel, os.Stdout, r2l)

	hasErr := false
	logErr := func(err error, local bool, reader bool) {
		if err != nil {
			hasErr = true
			dir := "local to remote"
			if !local {
				dir = "remote to local"
			}
			role := "reader"
			if !reader {
				role = "write"
			}
			log.Errorf("%s %s %s: %v", dir, role, err)
		}
	}

	select {
	case rerr := <-l2r:
		logErr(rerr, true, true)
		werr := <-l2r
		logErr(werr, true, false)
	case rerr := <-r2l:
		logErr(rerr, false, true)
		werr := <-r2l
		logErr(werr, false, false)
	}

	if hasErr {
		return 3
	}

	return 0
}

func main() {
	os.Exit(realMain())
}
