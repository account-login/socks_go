package main

import (
	"os"

	"github.com/account-login/socks_go"
	log "github.com/cihub/seelog"
)

func realMain() int {
	defer log.Flush()

	addr := ":1080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	server := socks_go.NewServer(addr, nil)
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
