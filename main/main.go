package main

import (
	"socks_go"
	"fmt"
	"os"
)

func main() {
	server := socks_go.Server{Addr: ":1080"}
	err := server.Run()
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
}
