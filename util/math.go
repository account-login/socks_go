package util

// https://stackoverflow.com/a/6878625
const MaxUint = ^uint(0)
const MinUint = 0
const MaxInt = int(MaxUint >> 1)
const MinInt = -MaxInt - 1

// https://stackoverflow.com/a/39544897
func RoundInt(x, unit float64) int {
	return int(float64(int64(x/unit+0.5)) * unit)
}

func MinNumInt(a, b int) int {
	if a < b {
		return b
	} else {
		return a
	}
}

func MaxNumInt(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func LimitRangeInt(lo, num, hi int) int {
	if num < lo {
		return lo
	} else if hi < num {
		return hi
	} else {
		return num
	}
}
