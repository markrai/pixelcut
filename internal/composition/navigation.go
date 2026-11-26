package composition

import (
	"sort"

	"github.com/markrai/pixelcut/internal/media"
)

func (e *Evaluator) EditBoundary(at int64, forward bool) (int64, bool) {
	if forward {
		i := sort.Search(len(e.boundaries), func(i int) bool { return e.boundaries[i] > at })
		if i < len(e.boundaries) {
			return e.boundaries[i], true
		}
	} else {
		i := sort.Search(len(e.boundaries), func(i int) bool { return e.boundaries[i] >= at })
		if i > 0 {
			return e.boundaries[i-1], true
		}
	}
	return 0, false
}

func (e *Evaluator) NextVideoBoundary(at int64) int64 {
	i := sort.Search(len(e.videoBoundaries), func(i int) bool { return e.videoBoundaries[i] > at })
	if i < len(e.videoBoundaries) {
		return e.videoBoundaries[i]
	}
	return e.duration
}

func (e *Evaluator) VideoSampleAt(start, at int64) int64 {
	start = max(0, min(start, e.duration))
	at = max(start, min(at, e.duration))
	if at == e.duration {
		return at
	}
	grid := start + (at-start)/media.CompositeStep*media.CompositeStep
	i := sort.Search(len(e.videoBoundaries), func(i int) bool { return e.videoBoundaries[i] > at })
	if i > 0 {
		grid = max(grid, e.videoBoundaries[i-1])
	}
	return grid
}

func (e *Evaluator) NextVideoSample(start, at int64) int64 {
	start = max(0, min(start, e.duration))
	at = max(start, min(at, e.duration))
	if at == e.duration {
		return at
	}
	delta := media.CompositeStep - (at-start)%media.CompositeStep
	next := e.duration
	if delta < e.duration-at {
		next = at + delta
	}
	return min(next, e.NextVideoBoundary(at))
}

func (f CompositeFrame) CurrentAt(at int64) bool {
	if f.End {
		return at == f.Time
	}
	return at >= f.Time && at < f.ValidUntil && at-f.Time <= 2*media.CompositeStep
}
