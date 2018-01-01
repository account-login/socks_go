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

func runServer(bind string, script []junkchat.Action) error {
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return errors.Wrapf(err, "runServer█listen(%q) error", bind)
	}

	log.Infof("runServer█server started on %v", listener.Addr())
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("runServerAccept error: %v", err)
			continue
		}

		log.Infof("runServer█client:%v█connected", conn.RemoteAddr())
		go func() {
			err := junkchat.ExecuteScript(script, conn)
			if err != nil {
				log.Errorf("runServer█client:%v█ExecuteScript error: %v", conn.RemoteAddr(), err)
			} else {
				log.Infof("runServer█client:%v█leave", conn.RemoteAddr())
			}
			conn.Close()
		}()
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
	scriptArg := flag.String("script", "", "scripts to run")
	bindArg := flag.String("bind", ":2080", "bind on address")
	flag.Parse()

	script, err := junkchat.ParseScript(*scriptArg)
	if err != nil {
		log.Errorf("parse script error: %v", err)
		return 1
	}

	go monitor()

	// run server
	err = runServer(*bindArg, script)
	if err != nil {
		log.Error(err)
		return 2
	}

	return 0
}

func main() {
	os.Exit(realMain())
}
