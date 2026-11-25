package timeline

import (
	"fmt"
	"math"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func FrameTrimDelta(f *media.FrameIndex, c project.Clip, duration int64, mode string, wanted int64, ripple bool) (int64, int64, bool) {
	var base, lo, hi int64
	switch mode {
	case "left":
		base = c.In
		lo = 0
		if !ripple {
			lo = max(0, c.In-c.Start)
		}
		hi = c.Out - 1
	case "right":
		base = c.Out
		lo = c.In + 1
		hi = duration
	default:
		return 0, 0, false
	}
	if lo > hi {
		return 0, 0, false
	}
	wanted = max(lo, min(hi, wanted))
	t, ok := f.Nearest(wanted, lo, hi)
	if mode == "right" && duration >= lo && (!ok || duration-wanted < absTicks(t-wanted)) {
		t, ok = duration, true
	}
	if !ok {
		return 0, 0, false
	}
	return t - base, t, true
}

func absTicks(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func EditBoundary(clips []project.Clip, at int64, forward bool) (int64, bool) {
	best := int64(0)
	found := false
	for _, c := range clips {
		for _, v := range []int64{c.Start, c.End()} {
			if forward && v > at && (!found || v < best) {
				best, found = v, true
			}
			if !forward && v < at && (!found || v > best) {
				best, found = v, true
			}
		}
	}
	if !forward && at > 0 && !found {
		return 0, true
	}
	return best, found
}

func RulerStep(scale float64) int64 {
	for power := int64(1); power <= math.MaxInt64/10; power *= 10 {
		for _, m := range []int64{1, 2, 5} {
			step := power * m
			if float64(step)/float64(project.TicksPerSecond)*scale >= 82 {
				return step
			}
		}
	}
	return math.MaxInt64 / 2
}

func RulerLabel(t, step int64) string {
	s := t / project.TicksPerSecond
	base := fmt.Sprintf("%02d:%02d", s/60, s%60)
	if step < project.TicksPerSecond {
		return fmt.Sprintf("%s.%03d", base, (t/1000)%1000)
	}
	return base
}
