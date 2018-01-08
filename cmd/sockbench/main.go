package main

import (
	"net"
	"sync"
	"time"

	"sort"

	"flag"
	"os"

	"io"

	"math"

	"github.com/account-login/socks_go"
	"github.com/account-login/socks_go/cmd"
	"github.com/account-login/socks_go/cmd/junkchat"
	"github.com/account-login/socks_go/util"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type taskSession struct {
	// req
	host   string
	port   uint16
	iofunc func(io.ReadWriter) error
	udp    bool
	// timestamp in nano seconds
	reqTime     time.Time
	connectTime time.Time
	finishTime  time.Time
	// result
	err error
}

// implement io.ReadWriter
type bindedPacketConn struct {
	net.PacketConn
	Addr *net.UDPAddr
}

func (conn *bindedPacketConn) Read(buf []byte) (n int, err error) {
	n, _, err = conn.ReadFrom(buf)
	return
}

func (conn *bindedPacketConn) Write(buf []byte) (n int, err error) {
	n, err = conn.WriteTo(buf, conn.Addr)
	return
}

func makeUDPAddrFromHostPort(host string, port uint16) (*net.UDPAddr, error) {
	ipAddr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return nil, errors.Wrapf(err, "makeUDPAddrFromHostPort: ResolveIPAddr error for %q", host)
	}

	addr := &net.UDPAddr{}
	addr.IP = ipAddr.IP
	addr.Port = int(port)
	return addr, nil
}

func doWork(proxyAddr string, task *taskSession, wg *sync.WaitGroup) {
	var conn net.Conn
	var tunnel io.ReadWriter
	var addr *net.TCPAddr

	defer func() {
		if conn != nil {
			err := conn.Close()
			if err != nil {
				log.Errorf("close conn err: %v", err)
			}
		}
		wg.Done()

		if task.err != nil {
			log.Errorf("client: %v, error: %v", addr, task.err)
		}
	}()

	task.reqTime = time.Now()

	// connnect to proxy
	conn, task.err = net.Dial("tcp", proxyAddr)
	if task.err != nil {
		return
	}

	addr, _ = conn.LocalAddr().(*net.TCPAddr)

	// create tunnel
	client := socks_go.NewClient(conn, nil)
	if task.udp {
		addr, err := makeUDPAddrFromHostPort(task.host, task.port)
		if err != nil {
			task.err = err
			return
		}

		var UDPTunnel socks_go.ClientUDPTunnel
		UDPTunnel, task.err = client.UDPAssociation()
		tunnel = &bindedPacketConn{PacketConn: &UDPTunnel, Addr: addr}
	} else {
		tunnel, task.err = client.Connect(task.host, task.port)
	}

	if task.err != nil {
		return
	}

	task.connectTime = time.Now()

	// do task
	task.err = task.iofunc(tunnel)
	task.finishTime = time.Now()
}

func worker(proxyAddr string, inq <-chan *taskSession, wg *sync.WaitGroup) {
	for task := range inq {
		doWork(proxyAddr, task, wg)
	}
}

func nthValue(input []float64, pos []float64) (result []float64) {
	if len(input) == 0 {
		return make([]float64, len(pos))
	}

	sort.Float64s(input)
	for _, val := range pos {
		idx := util.RoundInt(val*float64(len(input)), 1)
		if idx >= len(input) {
			idx = len(input) - 1
		}
		result = append(result, input[idx])
	}
	return
}

func printStats(results []*taskSession) {
	var startTime = time.Unix(math.MaxInt32, 0) // maximum time
	var stopTime = time.Unix(0, 0)
	success := 0
	connectTimes := make([]float64, 0, len(results))
	processTimes := make([]float64, 0, len(results))

	for _, r := range results {
		if r.err == nil {
			success++
		} else {
			continue
		}

		if r.reqTime.Before(startTime) {
			startTime = r.reqTime
		}
		if r.finishTime.After(stopTime) {
			stopTime = r.finishTime
		}

		connectTimes = append(connectTimes, r.connectTime.Sub(r.reqTime).Seconds())
		processTimes = append(processTimes, r.finishTime.Sub(r.reqTime).Seconds())
	}

	log.Infof("success rate: %d/%d (%.4f)", success, len(results), float64(success)/float64(len(results)))

	rps := float64(len(results)) / stopTime.Sub(startTime).Seconds()
	log.Infof("[duration:%f][reqs:%d][rps:%.1f]", stopTime.Sub(startTime).Seconds(), len(results), rps)

	distPos := []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 0.99, 1}
	connectDist := nthValue(connectTimes, distPos)
	processDist := nthValue(processTimes, distPos)

	log.Infof("connect dist")
	printDist(connectDist, distPos)
	log.Infof("process dist")
	printDist(processDist, distPos)
}

func printDist(times []float64, pos []float64) {
	for i, p := range pos {
		log.Infof("[dist:%02.2f][time:%.3fms]", p, float64(times[i])*1e3)
	}
}

func run(proxyAddr string, workerNum int, works []*taskSession) {
	log.Debugf("worker: %d", workerNum)

	q := make(chan *taskSession, len(works))
	var wg sync.WaitGroup
	wg.Add(len(works))

	for i := 0; i < workerNum; i++ {
		go worker(proxyAddr, q, &wg)
	}

	for _, task := range works {
		q <- task
	}
	close(q)

	wg.Wait()

	// process results
	printStats(works)
}

func makeSessions(n int, script []junkchat.Action, host string, port uint16, udp bool, size int) (works []*taskSession) {
	for i := 0; i < n; i++ {
		iofunc := func(transport io.ReadWriter) (err error) {
			//log.Debugf("iofunc begin: %d", i)
			if udp {
				err = junkchat.ExecutePacketScript(script, transport, size)
			} else {
				err = junkchat.ExecuteScript(script, transport)
			}
			//log.Debugf("iofunc finish: %d", i)
			return
		}
		works = append(works, &taskSession{host: host, port: port, iofunc: iofunc, udp: udp})
	}
	return
}

func realMain() int {
	// logging
	defer log.Flush()
	cmd.ConfigLogging()

	// cli args
	proxyArg := flag.String("proxy", "127.0.0.1:1080", "socks5 proxy server")
	// TODO: multiple junk server
	junkArg := flag.String("junk", "127.0.0.1:2080", "junk server")
	workerArg := flag.Int("worker", 16, "number of workers")
	reqsArg := flag.Int("reqs", 1024, "number of requests")
	udpArg := flag.Bool("udp", false, "run in UDP mode")
	sizeArg := flag.Int("size", 1024, "send packet size in UDP")
	scriptArg := flag.String("script", "", `scripts to run
		In TCP mode, the unit is bytes. In UDP mode, the unit is number of packets`)
	debugArg := flag.String("debug", ":6060", "http debug server")
	flag.Parse()

	junkHost, junkPort, err := util.SplitHostPort(*junkArg)
	if err != nil {
		log.Errorf("can not parse -junk host:port : %v", err)
		return 2
	}

	script, err := junkchat.ParseScript(*scriptArg)
	if err != nil {
		log.Errorf("parse script error: %v", err)
		return 1
	}

	// debug server
	cmd.StartDebugServer(*debugArg)

	// run benchmark
	works := makeSessions(*reqsArg, script, junkHost, junkPort, *udpArg, *sizeArg)
	run(*proxyArg, *workerArg, works)

	return 0
}

func main() {
	os.Exit(realMain())
}
