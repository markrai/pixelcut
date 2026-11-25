package main

import (
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestEditBoundaryNavigation(t *testing.T) {
	clips := []project.Clip{{Start: 8000000, In: 1000000, Out: 3000000}, {Start: 2000000, In: 0, Out: 3000000}}
	for _, tc := range []struct {
		at, want    int64
		forward, ok bool
	}{
		{0, 2000000, true, true}, {2000000, 5000000, true, true}, {5000000, 8000000, true, true},
		{8000000, 10000000, true, true}, {10000000, 0, true, false},
		{10000000, 8000000, false, true}, {8000000, 5000000, false, true}, {2000000, 0, false, true}, {0, 0, false, false},
	} {
		v, ok := editBoundary(clips, tc.at, tc.forward)
		if v != tc.want || ok != tc.ok {
			t.Fatalf("%+v: %d %t", tc, v, ok)
		}
	}
}

func TestPrecisionRuler(t *testing.T) {
	for _, tc := range []struct {
		scale float64
		step  int64
	}{{1000, 100000}, {100, 1000000}, {10, 10000000}, {1, 100000000}} {
		if got := rulerStep(tc.scale); got != tc.step {
			t.Fatalf("scale %f step %d", tc.scale, got)
		}
	}
	if got := precisionStamp(3661123456); got != "01:01:01.123456" {
		t.Fatal(got)
	}
	if got := rulerLabel(12300000, 100000); got != "00:12.300" {
		t.Fatal(got)
	}
}

func TestSpeedRequestPreservesSeek(t *testing.T) {
	c := newPreviewClock(project.Clip{}, 0, true)
	c.queue(123456, true, true)
	c.setSpeed(1.5)
	r := c.request.Load()
	if r.speed != 1.5 || !r.paused || r.tick != 123456 || r.seekSerial != 1 {
		t.Fatalf("speed lost request: %+v", r)
	}
}
