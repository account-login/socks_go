package junkchat

import (
	"time"

	"github.com/account-login/socks_go/util"
)

type TimeLimit struct {
	// params
	Limit time.Duration
	Total int // total amount of work
	Step  int // prefered amount of work to do at once

	// counter
	Finished int

	started time.Time
}

func (r *TimeLimit) checkStatedTime() time.Time {
	now := time.Now()
	if r.started.Equal(time.Time{}) {
		r.started = now
	}
	return now
}

func (r *TimeLimit) Done(n int) time.Duration {
	r.Finished += n

	now := r.checkStatedTime()
	duration := now.Sub(r.started)

	minWork := util.MinNumInt(r.Step, 1)
	next := time.Duration(float64(r.Finished+minWork) / float64(r.Total) * float64(r.Limit))

	if next > duration {
		if next > r.Limit {
			next = r.Limit
		}
		return next - duration
	} else {
		return 0
	}
}

func (r *TimeLimit) DoWork(worker func(int) (int, error)) error {
	// special case
	if r.Total == 0 {
		time.Sleep(r.Limit)
		return nil
	}

	r.checkStatedTime()
	for r.Finished < r.Total {
		todo := util.LimitRangeInt(1, r.Step, r.Total-r.Finished)
		done, err := worker(todo)
		if err != nil {
			return err
		}

		pause := r.Done(done)
		if pause > 0 {
			time.Sleep(pause)
		}
	}
	return nil
}
