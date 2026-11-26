package playback

import (
	"math"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestSampleClockConfirmedPositionAndEnd(t *testing.T) {
	c, err := NewSampleClock(123, 1000124)
	if err != nil || c.Total() != 48001 || c.Tick() != 123 {
		t.Fatalf("clock initialization: %v", err)
	}
	if !c.Confirm(2400) || c.Tick() != 50123 || c.Confirm(2399) || c.Confirm(c.Total()+1) {
		t.Fatal("clock accepted invalid or regressing observation")
	}
	if !c.Confirm(c.Total()) || c.Tick() != 1000124 || c.Played() != c.Total() {
		t.Fatal("clock failed to clamp to exact end")
	}
	if _, err := NewSampleClock(10, 10); err == nil {
		t.Fatal("empty clock interval accepted")
	}
	c, err = NewSampleClock(0, math.MaxInt64)
	if err != nil || !c.Confirm(c.Total()-1) || c.Tick() >= math.MaxInt64 || !c.Confirm(c.Total()) || c.Tick() != math.MaxInt64 {
		t.Fatal("maximum duration clock overflow")
	}
	if media.PCMFramesForTicks(1) != 1 || media.PCMTicksForFrames(48000) != 1000000 {
		t.Fatal("sample conversion failed")
	}
}
