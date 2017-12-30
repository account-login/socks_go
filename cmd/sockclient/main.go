package main

import "os"

import (
	"flag"
	"io"
	"net"

	"strconv"

	"github.com/account-login/socks_go"
	"github.com/account-login/socks_go/cmd"
	log "github.com/cihub/seelog"
)

const MethodMyExtended = socks_go.MethodPrivateBegin + 1

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

// copied from common_protocol.go
func readRequired(reader io.Reader, n int) (data []byte, err error) {
	data = make([]byte, n)
	_, err = io.ReadFull(reader, data)
	return
}

func extendedAuthHandler(proto *socks_go.ClientProtocol) (err error) {
	var ip net.IP
	ip, err = readRequired(proto.Transport, 4)
	if err == nil {
		log.Infof("ip echo: %v", ip)
	}
	return err
}

func realMain() int {
	defer log.Flush()
	cmd.ConfigLogging()

	fifoDefers := make([]func(), 0)
	defer func() {
		for _, f := range fifoDefers {
			f()
		}
	}()

	// parse args
	proxyArg := flag.String("proxy", "127.0.0.1:1080", "socks5 proxy server")
	flag.Parse()
	target := flag.Arg(0)
	if len(target) == 0 {
		log.Errorf("must specify target address")
		return 1
	}

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		log.Errorf("can not parse host:port: %s", target)
		return 4
	}
	portInt, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		log.Errorf("can not parse host:port: %s", target)
		return 4
	}
	port := uint16(portInt)

	// connect to proxy server
	conn, err := net.Dial("tcp", *proxyArg)
	if err != nil {
		log.Errorf("Dial to proxy failed: %v", err)
		return 2
	}
	fifoDefers = append(fifoDefers, func() { conn.Close() })

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
	l2r, r2l := make(chan error, 2), make(chan error, 2)
	go bridgeReaderWriter(os.Stdin, tunnel, l2r)
	go bridgeReaderWriter(tunnel, os.Stdout, r2l)

	// error handling
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
				role = "writer"
			}
			log.Errorf("%s %s error: %v", dir, role, err)
		}
	}

	handleErr := func() {
		select {
		case rerr := <-l2r:
			logErr(rerr, true, true)
			logErr(<-l2r, true, false)
			l2r = nil
		case rerr := <-r2l:
			logErr(rerr, false, true)
			logErr(<-r2l, false, false)
			r2l = nil
		}
	}

	handleErr() // first channel of error

	if hasErr {
		fifoDefers = append(fifoDefers, handleErr) // second channel of error will be handled after close
		return 3
	} else {
		handleErr() // one side finished, wait for another side
		return 0
	}
}

func main() {
	os.Exit(realMain())
}
