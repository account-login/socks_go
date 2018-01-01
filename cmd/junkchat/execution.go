package junkchat

import (
	"io"
	"math/rand"
	"time"

	"github.com/pkg/errors"
)

// from net.Conn
type HasDeadline interface {
	SetDeadline(time.Time) error
}

func ExecuteScript(acts []Action, transport io.ReadWriter) error {
	conn, hasDeadline := transport.(HasDeadline)
	for i, act := range acts {
		if hasDeadline { // hack
			// Read & Write may still block after act.Duration elapsed
			_ = conn.SetDeadline(time.Now().Add(act.Duration + act.Duration/20))
		}

		rerr := doRead(act.Read, act.Duration, transport)
		werr := doWrite(act.Write, act.Duration, transport)
		if err := <-rerr; err != nil {
			return errors.Wrapf(err, "ExecuteScript: reader error on %d-th action %+v", i, act)
		}
		if err := <-werr; err != nil {
			return errors.Wrapf(err, "ExecuteScript: writer error on %d-th action %+v", i, act)
		}
	}
	return nil
}

const (
	minRead  = 1024
	minWrite = 1024
)
const junkPoolSize = 1024 * 1024 // 1M
var junkPool = makeJunkPool(junkPoolSize)

func makeJunkPool(size int) []byte {
	junk := make([]byte, size)
	rand.Read(junk)
	return junk
}

func getJunk(size int) []byte {
	if size > len(junkPool) {
		panic("huge size")
	}
	idx := rand.Intn(len(junkPool) - size)
	return junkPool[idx : idx+size]
}

func doRead(size int, duration time.Duration, reader io.Reader) chan error {
	errch := make(chan error, 1)
	go func() {
		tl := TimeLimit{Limit: duration, Total: size, Step: minRead}
		err := tl.DoWork(func(n int) (done int, err error) {
			return reader.Read(make([]byte, n))
		})
		if err == io.EOF {
			err = nil
		}
		errch <- err
	}()
	return errch
}

func doWrite(size int, duration time.Duration, writer io.Writer) chan error {
	errch := make(chan error, 1)
	go func() {
		tl := TimeLimit{Limit: duration, Total: size, Step: minWrite}
		errch <- tl.DoWork(func(n int) (int, error) {
			return writer.Write(getJunk(n))
		})
	}()
	return errch
}
