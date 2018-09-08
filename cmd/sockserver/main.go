package main

import (
	"os"

	"runtime"
	"time"

	"flag"

	"github.com/account-login/socks_go"
	"github.com/account-login/socks_go/cmd"
	log "github.com/cihub/seelog"
)

func monitor() {
	log.Infof("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
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

	// args
	bindArg := flag.String("bind", ":1080", "bind on address")
	debugArg := flag.String("debug", "127.0.0.1:6061", "http debug server")
	flag.Parse()

	go monitor()
	cmd.StartDebugServer(*debugArg)

	server := socks_go.NewServer(*bindArg, nil)
	err := server.Run()
	if err != nil {
		log.Errorf("failed to start server: %v", err)
		return 1
	} else {
		return 0
	}
}

func main() {
	os.Exit(realMain())
}
