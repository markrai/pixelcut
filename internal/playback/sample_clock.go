package playback

import (
	"errors"
	"sync/atomic"

	"github.com/markrai/pixelcut/internal/media"
)

type SampleClock struct {
	start, end int64
	total      int64
	played     atomic.Int64
}

func NewSampleClock(start, end int64) (*SampleClock, error) {
	if start < 0 || end <= start {
		return nil, errors.New("invalid audio clock interval")
	}
	return &SampleClock{start: start, end: end, total: media.PCMFramesForTicks(end - start)}, nil
}

func (c *SampleClock) Total() int64  { return c.total }
func (c *SampleClock) Played() int64 { return c.played.Load() }

func (c *SampleClock) Confirm(frames int64) bool {
	if frames < 0 || frames > c.total {
		return false
	}
	for {
		previous := c.played.Load()
		if frames < previous {
			return false
		}
		if c.played.CompareAndSwap(previous, frames) {
			return true
		}
	}
}

func (c *SampleClock) Tick() int64 {
	n := c.Played()
	if n == c.total {
		return c.end
	}
	return c.start + media.PCMTicksForFrames(n)
}
