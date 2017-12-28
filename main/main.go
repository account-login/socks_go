package main

import (
	"os"

	"github.com/account-login/socks_go"
	log "github.com/cihub/seelog"
)

func realMain() int {
	defer log.Flush()

	server := socks_go.NewServer(":1080", nil)
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
