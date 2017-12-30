package main

import (
	"os"

	"net"

	"runtime"
	"time"

	"github.com/account-login/socks_go"
	log "github.com/cihub/seelog"
)

const MethodMyExtended = socks_go.MethodPrivateBegin + 1

func extenedAuthHandler(methods []byte, proto *socks_go.ServerProtocol) error {
	extended := false
	for _, method := range methods {
		if method == MethodMyExtended {
			extended = true
			break
		}
	}

	if extended {
		err := proto.AcceptAuthMethod(MethodMyExtended)
		if err != nil {
			return err
		}

		// big endian
		ip := net.IPv4(0, 0, 0, 0)
		if conn, ok := proto.Transport.(net.Conn); ok {
			if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
				if ip4 := tcpAddr.IP.To4(); ip4 != nil {
					ip = ip4
				}
			}
		}

		log.Infof("remote ip: %v", ip)
		_, err = proto.Transport.Write(ip[:4])
		return err
	} else {
		return proto.AcceptAuthMethod(socks_go.MethodNone)
	}
}

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
	defer log.Flush()

	addr := ":1080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	go monitor()

	server := socks_go.NewServer(addr, extenedAuthHandler)
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
