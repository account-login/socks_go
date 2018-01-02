package main

import (
	"net"
	"sync"
	"time"

	"sort"

	"flag"
	"os"

	"io"

	"github.com/account-login/socks_go"
	"github.com/account-login/socks_go/cmd"
	"github.com/account-login/socks_go/cmd/junkchat"
	"github.com/account-login/socks_go/util"
	log "github.com/cihub/seelog"
)

type taskSession struct {
	// req
	host   string
	port   uint16
	iofunc func(io.ReadWriter) error
	// timestamp in nano seconds
	reqTime     int64
	connectTime int64
	finishTime  int64
	// result
	err error
}

func worker(proxyAddr string, inq <-chan *taskSession, wg *sync.WaitGroup) {
	for task := range inq {
		func(task *taskSession) {
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

			task.reqTime = time.Now().UnixNano()

			// connnect to proxy
			conn, task.err = net.Dial("tcp", proxyAddr)
			if task.err != nil {
				return
			}

			addr = conn.LocalAddr().(*net.TCPAddr)

			// create tunnel
			client := socks_go.NewClient(conn, nil)
			tunnel, task.err = client.Connect(task.host, task.port)
			if task.err != nil {
				return
			}
			task.connectTime = time.Now().UnixNano()

			// do task
			task.err = task.iofunc(tunnel)
			task.finishTime = time.Now().UnixNano()
		}(task)
	}
}

func nthValue(input []int, pos []float64) (result []int) {
	if len(input) == 0 {
		return make([]int, len(pos))
	}

	sort.Ints(input)
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
	var startTime = int64(^uint64(0) >> 1)
	var stopTime int64 = 0
	success := 0
	connectTimes := make([]int, 0, len(results))
	processTimes := make([]int, 0, len(results))

	for _, r := range results {
		if r.err == nil {
			success++
		} else {
			continue
		}

		if r.reqTime < startTime {
			startTime = r.reqTime
		}
		if r.finishTime > stopTime {
			stopTime = r.finishTime
		}

		connectTimes = append(connectTimes, int(r.connectTime-r.reqTime))
		processTimes = append(processTimes, int(r.finishTime-r.reqTime))
	}

	log.Infof("success rate: %d/%d (%.4f)", success, len(results), float64(success)/float64(len(results)))

	rps := float64(len(results)) / float64(stopTime-startTime) * 1e9
	log.Infof("[duration:%f][reqs:%d][rps:%.1f]", float64(stopTime-startTime)/1e9, len(results), rps)

	distPos := []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1}
	connectDist := nthValue(connectTimes, distPos)
	processDist := nthValue(processTimes, distPos)

	log.Infof("connect dist")
	printDist(connectDist, distPos)
	log.Infof("process dist")
	printDist(processDist, distPos)
}

func printDist(times []int, pos []float64) {
	for i, p := range pos {
		log.Infof("[dist:%02.2f][time:%.3fms]", p, float64(times[i])/1e6)
	}
}

func run(proxyAddr string, workerNum int, works []*taskSession) {
	q := make(chan *taskSession, len(works))
	var wg sync.WaitGroup
	wg.Add(len(works))

	for i := 0; i < workerNum; i++ {
		go worker(proxyAddr, q, &wg)
	}

	for _, task := range works {
		q <- task
	}

	wg.Wait()

	// process results
	printStats(works)
}

func makeSessions(n int, script []junkchat.Action, host string, port uint16) (works []*taskSession) {
	for i := 0; i < n; i++ {
		iofunc := func(transport io.ReadWriter) (err error) {
			//log.Debugf("iofunc begin: %d", i)
			err = junkchat.ExecuteScript(script, transport)
			//log.Debugf("iofunc finish: %d", i)
			return
		}
		works = append(works, &taskSession{host: host, port: port, iofunc: iofunc})
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
	scriptArg := flag.String("script", "", "scripts to run")
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

	// run benchmark
	works := makeSessions(*reqsArg, script, junkHost, junkPort)
	run(*proxyArg, *workerArg, works)

	return 0
}

func main() {
	os.Exit(realMain())
}
