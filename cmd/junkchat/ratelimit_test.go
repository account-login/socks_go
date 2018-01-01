package junkchat

import (
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fastWorker(n int) (int, error) {
	if n >= 30 {
		n = 30
	}
	return n, nil
}

func slowWorker(n int) (int, error) {
	time.Sleep(1 * time.Millisecond)
	return fastWorker(n)
}

func doTimeLimitTest(t *testing.T, limit time.Duration, total int, step int, worker func(int) (int, error)) {
	started := time.Now()
	r := TimeLimit{Limit: limit, Total: total, Step: step}
	err := r.DoWork(worker)
	duration := time.Now().Sub(started)

	require.NoError(t, err)
	assert.Equal(t, r.Total, r.Finished)
	assert.InDelta(t, limit.Seconds(), duration.Seconds(), 0.005)
}

func TestTimeLimit_DoWork(t *testing.T) {
	doTimeLimitTest(t, 100*time.Millisecond, 10000, 100, fastWorker)
}

func TestTimeLimit_slow_worker(t *testing.T) {
	doTimeLimitTest(t, 110*time.Millisecond, 3000, 100, slowWorker)
}

func TestTimeLimit_DoWork_zero_work(t *testing.T) {
	doTimeLimitTest(t, 100*time.Millisecond, 0, 100, fastWorker)
}

func TestTimeLimit_DoWork_zero_limit(t *testing.T) {
	doTimeLimitTest(t, 0*time.Millisecond, 1000, 100, fastWorker)
}

func TestTimeLimit_DoWork_zero_step(t *testing.T) {
	doTimeLimitTest(t, 100*time.Millisecond, 1000, 0, fastWorker)
}

func TestTimeLimit_DoWork_huge_step(t *testing.T) {
	doTimeLimitTest(t, 100*time.Millisecond, 1000, 100000, fastWorker)
}
