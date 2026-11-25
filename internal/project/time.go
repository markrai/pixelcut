package project

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

const TicksPerSecond int64 = 1000000

type Interval struct {
	Start int64
	End   int64
}

func (i Interval) Valid() bool {
	return i.Start >= 0 && i.End > i.Start
}

func (i Interval) Contains(at int64) bool {
	return i.Valid() && at >= i.Start && at < i.End
}

func (i Interval) Overlaps(other Interval) bool {
	return i.Valid() && other.Valid() && i.Start < other.End && other.Start < i.End
}

func (c Clip) SourceInterval() Interval {
	return Interval{Start: c.In, End: c.Out}
}

func (c Clip) Duration() int64 {
	if !c.SourceInterval().Valid() {
		return 0
	}
	return c.Out - c.In
}

func (c Clip) TimelineInterval() (Interval, bool) {
	duration := c.Duration()
	if c.Start < 0 || duration == 0 || c.Start > math.MaxInt64-duration {
		return Interval{}, false
	}
	return Interval{Start: c.Start, End: c.Start + duration}, true
}

func (c Clip) End() int64 {
	i, ok := c.TimelineInterval()
	if !ok {
		return 0
	}
	return i.End
}

func (c Clip) Contains(at int64) bool {
	i, ok := c.TimelineInterval()
	return ok && i.Contains(at)
}

func (c Clip) SourceAt(at int64) (int64, bool) {
	if !c.Contains(at) {
		return 0, false
	}
	return c.In + (at - c.Start), true
}

func (t Track) EndTime() int64 {
	var end int64
	for _, c := range t.Clips {
		end = max(end, c.End())
	}
	return end
}

func ParseSeconds(s string) (int64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, errors.New("invalid nonnegative seconds")
	}
	ticks := math.Round(n * float64(TicksPerSecond))
	if ticks >= float64(math.MaxInt64) {
		return 0, errors.New("seconds exceed timeline bounds")
	}
	return int64(ticks), nil
}

func Seconds(t int64) float64 { return float64(t) / float64(TicksPerSecond) }
