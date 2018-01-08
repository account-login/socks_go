package main

import (
	"os"

	log "github.com/cihub/seelog"

	"flag"
	"net"

	"runtime"
	"time"

	"github.com/account-login/socks_go/cmd"
	"github.com/account-login/socks_go/cmd/junkchat"
	"github.com/pkg/errors"
)

func runTCPServer(bind string, script []junkchat.Action) error {
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return errors.Wrapf(err, "runTCPServer█listen(%q) error", bind)
	}

	log.Infof("runTCPServer█server started on %v", listener.Addr())
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("runTCPServer█Accept error: %v", err)
			continue
		}

		log.Infof("runTCPServer█client:%v█connected", conn.RemoteAddr())
		go func() {
			err := junkchat.ExecuteScript(script, conn)
			if err != nil {
				log.Errorf("runTCPServer█client:%v█ExecuteScript error: %v", conn.RemoteAddr(), err)
			} else {
				log.Infof("runTCPServer█client:%v█leave", conn.RemoteAddr())
			}
			conn.Close()
		}()
	}
}

func runUDPServer(bind string, size int) error {
	if size < 8 {
		log.Warnf("runUDPServer█minimum size is 8, got %d", size)
		size = 8
	}

	conn, err := net.ListenPacket("udp", bind)
	if err != nil {
		return errors.Wrapf(err, "runUDPServer█listen(%q) error", bind)
	}
	defer conn.Close()

	log.Infof("runUDPServer█server started on %v", conn.LocalAddr())

	buf := make([]byte, 64*1024)
	if size > len(buf) {
		buf = make([]byte, size)
	}
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Errorf("runUDPServer█ReadFrom() error: %v", err)
			continue
		}

		log.Infof("runUDPServer█got packet from %v, len: %d", addr, n)

		n, err = conn.WriteTo(buf[:size], addr)
		if err != nil {
			log.Errorf("runUDPServer█WriteTo(..., %v) error: %v", addr, err)
			continue
		}
	}
}

// copied from cmd/sockserver/main.go
func monitor() {
	prev := 0
	for {
		now := runtime.NumGoroutine()
		if now != prev {
			log.Debugf("goroutines: %d", now)
		}
		prev = now
		time.Sleep(1 * time.Second)
	}
}

func realMain() int {
	// logging
	defer log.Flush()
	cmd.ConfigLogging()

	// parse cli args
	scriptArg := flag.String("script", "", "scripts to run (TCP)")
	sizeArg := flag.Int("size", 1024, "packet size (UDP)")
	bindArg := flag.String("bind", ":2080", "bind on address")
	flag.Parse()

	script, err := junkchat.ParseScript(*scriptArg)
	if err != nil {
		log.Errorf("parse script error: %v", err)
		return 1
	}

	go monitor()

	// run server
	tcpServerErr := make(chan error, 1)
	udpServerErr := make(chan error, 1)

	go func() {
		tcpServerErr <- runTCPServer(*bindArg, script)
	}()
	go func() {
		udpServerErr <- runUDPServer(*bindArg, *sizeArg)
	}()

	retCode := 0
	for i := 0; i < 2; i++ {
		select {
		case err = <-tcpServerErr:
			if err != nil {
				log.Errorf("tcp server error: %v", err)
				tcpServerErr = nil
				retCode = 2
			}
		case err = <-udpServerErr:
			if err != nil {
				log.Errorf("udp server error: %v", err)
				udpServerErr = nil
				retCode = 3
			}
		}
	}

	return retCode
}

func main() {
	os.Exit(realMain())
}
