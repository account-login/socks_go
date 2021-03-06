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

	"strings"

	"github.com/account-login/socks_go"
	"github.com/account-login/socks_go/cmd"
	"github.com/account-login/socks_go/cmd/junkchat"
	"github.com/account-login/socks_go/util"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type taskSession struct {
	// req
	host      string
	port      uint16
	iofunc    func(io.ReadWriter) error
	udp       bool
	localAddr *net.TCPAddr
	// timestamp in nano seconds
	reqTime     time.Time
	connectTime time.Time
	finishTime  time.Time
	// result
	err error
}

type proxyParam struct {
	addr    string
	timeout time.Duration
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

func createTunnel(conn net.Conn, task *taskSession) <-chan io.ReadWriter {
	ch := make(chan io.ReadWriter)
	go func() {
		var tunnel io.ReadWriter
		client := socks_go.NewClientWithParam(conn, nil, socks_go.ClientParam{FixUDPAddr: true})
		if task.udp {
			udpAddr, err := makeUDPAddrFromHostPort(task.host, task.port)
			if err != nil {
				task.err = err
			} else {
				var UDPTunnel socks_go.ClientUDPTunnel
				UDPTunnel, task.err = client.UDPAssociation()
				tunnel = &bindedPacketConn{PacketConn: &UDPTunnel, Addr: udpAddr}
			}
		} else {
			tunnel, task.err = client.Connect(task.host, task.port)
		}

		ch <- tunnel
	}()
	return ch
}

func doWork(proxy proxyParam, task *taskSession, wg *sync.WaitGroup) {
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
	// TODO: move timeout control to Client
	deadline := time.Now().Add(proxy.timeout)
	conn, task.err = (&net.Dialer{
		Timeout:   proxy.timeout,
		LocalAddr: task.localAddr,
	}).Dial("tcp", proxy.addr)
	if task.err != nil {
		return
	}

	addr, _ = conn.LocalAddr().(*net.TCPAddr) // for logging

	// create tunnel
	timeout := deadline.Sub(time.Now())
	select {
	case tunnel = <-createTunnel(conn, task):
	case <-time.After(timeout):
		task.err = errors.Errorf("createTunnel() timeout")
	}

	if task.err != nil {
		return
	}

	task.connectTime = time.Now()

	// do task
	task.err = task.iofunc(tunnel)
	task.finishTime = time.Now()
}

func worker(proxy proxyParam, inq <-chan *taskSession, wg *sync.WaitGroup) {
	for task := range inq {
		doWork(proxy, task, wg)
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

func run(proxy proxyParam, connRate float64, workerNum int, works []*taskSession) {
	log.Debugf("worker: %d", workerNum)

	q := make(chan *taskSession, len(works))
	var wg sync.WaitGroup
	wg.Add(len(works))

	for i := 0; i < workerNum; i++ {
		go worker(proxy, q, &wg)
	}

	// limit connect rate
	tl := junkchat.TimeLimit{
		Total: len(works),
		Step:  1,
		Limit: time.Duration(float64(len(works))/connRate) * time.Second,
	}
	start := 0
	tl.DoWork(func(n int) (int, error) {
		for i := start; i < start+n; i++ {
			q <- works[i]
		}
		start += n
		return n, nil
	})
	close(q)

	wg.Wait()

	// process results
	printStats(works)
}

type hostPortPair struct {
	host string
	port uint16
}

func makeSessions(
	n int,
	script []junkchat.Action,
	junkServers []hostPortPair,
	localAddrs []*net.TCPAddr,
	udp bool,
	size int) (works []*taskSession) {

	for i := 0; i < n; i++ {
		iofunc := func(transport io.ReadWriter) (err error) {
			//log.Debugf("iofunc begin: %d", i)
			if udp {
				err = junkchat.ExecutePacketScript(script, transport, size)
			} else {
				err = junkchat.ExecuteStreamScript(script, transport)
			}
			//log.Debugf("iofunc finish: %d", i)
			return
		}

		pair := junkServers[i%len(junkServers)]
		var addr *net.TCPAddr
		if len(localAddrs) != 0 {
			addr = localAddrs[i%len(localAddrs)]
		}
		works = append(works, &taskSession{
			host: pair.host, port: pair.port, localAddr: addr,
			iofunc: iofunc, udp: udp,
		})
	}
	return
}

func realMain() int {
	// logging
	defer log.Flush()
	cmd.ConfigLogging()

	// cli args
	proxyArg := flag.String("proxy", "127.0.0.1:1080", "socks5 proxy server")
	timeoutArg := flag.Int("timeout", 5000, "timeout in ms for tunnel creation")
	junkArg := flag.String("junk", "127.0.0.1:2080", "junk servers seperated by comma")
	localArg := flag.String("local", "", "local source addresses seperated by comma")
	connectRateArg := flag.Float64("connect-rate", math.MaxFloat64, "maximum number of new connection per second")
	workerArg := flag.Int("worker", 16, "number of workers")
	reqsArg := flag.Int("reqs", 1024, "number of requests")
	udpArg := flag.Bool("udp", false, "run in UDP mode")
	sizeArg := flag.Int("size", 1024, "send packet size in UDP")
	scriptArg := flag.String("script", "", `scripts to run
		In TCP mode, the unit is bytes. In UDP mode, the unit is the number of packets`)
	debugArg := flag.String("debug", "127.0.0.1:6060", "http debug server")
	flag.Parse()

	junkServers := make([]hostPortPair, 0)
	for _, junkSv := range strings.Split(*junkArg, ",") {
		host, port, err := util.SplitHostPort(junkSv)
		if err != nil {
			log.Errorf("can not parse host:port pair %q: %v", junkSv, err)
			return 2
		}
		junkServers = append(junkServers, hostPortPair{host, port})
	}
	if len(junkServers) == 0 {
		log.Errorf("no junk server specified")
		return 2
	}

	localAddrs := make([]*net.TCPAddr, 0)
	if len(*localArg) > 0 {
		for _, piece := range strings.Split(*localArg, ",") {
			addr, err := util.ParseTCPAddr(piece)
			if err != nil {
				log.Errorf("can not parse local address %q: %v", piece, err)
				return 2
			}
			localAddrs = append(localAddrs, addr)
		}
	}

	script, err := junkchat.ParseScript(*scriptArg)
	if err != nil {
		log.Errorf("parse script error: %v", err)
		return 1
	}

	// debug server
	cmd.StartDebugServer(*debugArg)

	// run benchmark
	works := makeSessions(*reqsArg, script, junkServers, localAddrs, *udpArg, *sizeArg)
	run(
		proxyParam{*proxyArg, time.Duration(*timeoutArg) * time.Millisecond},
		*connectRateArg,
		*workerArg, works,
	)

	return 0
}

func main() {
	os.Exit(realMain())
}
