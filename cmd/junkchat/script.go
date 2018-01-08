package junkchat

import (
	"strings"
	"time"
	"unicode"

	"github.com/pkg/errors"
)

type Action struct {
	Duration time.Duration
	Deadline time.Duration
	Read     int // bytes
	Write    int
}

func consumeInt(input string) (n int, remain string, ok bool) {
	if len(input) == 0 {
		ok = false
		return
	}

	for remain = input; len(remain) != 0 && unicode.IsDigit(rune(remain[0])); remain = remain[1:] {
		n *= 10
		n += int(remain[0]) - '0'
	}
	ok = true
	return
}

var sizeUnits = map[string]int{
	"":  1,
	"b": 1,
	"k": 1024,
	"m": 1024 * 1024,
	"g": 1024 * 1024 * 1024,
}

var timeUnits = map[string]int{
	"ms":  1,
	"s":   1000,
	"min": 60 * 1000,
	"h":   60 * 60 * 1000,
}

func ParseScript(input string) (acts []Action, err error) {
	for i, piece := range strings.Split(input, ".") {
		act := Action{}

		subActs := strings.Split(piece, ",")
		for _, subAct := range subActs {
			subAct = strings.ToLower(strings.TrimSpace(subAct))
			if len(subAct) == 0 {
				err = errors.Errorf("empty act at #%d", i)
				return
			}

			op := subAct[0]
			num, remain, ok := consumeInt(subAct[1:])
			if !ok {
				err = errors.Errorf("no number after op at #%d", i)
				return
			}

			switch op {
			case 't', 'd':
				unit, ok := timeUnits[remain]
				if !ok {
					err = errors.Errorf("bad duration unit at #%d", i)
					return
				}
				if op == 't' {
					act.Duration = time.Millisecond * time.Duration(num*unit)
				} else {
					act.Deadline = time.Millisecond * time.Duration(num*unit)
				}
			case 'r', 'w':
				unit, ok := sizeUnits[remain]
				if !ok {
					err = errors.Errorf("bad size unit at #%d", i)
					return
				}
				if op == 'r' {
					act.Read = num * unit
				} else {
					act.Write = num * unit
				}
			default:
				err = errors.Errorf("unknown op: %c", op)
				return
			} // switch op
		} // for subAct

		acts = append(acts, act)
	} // for piece
	return
}
