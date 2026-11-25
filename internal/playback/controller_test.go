package playback

import (
	"math"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestPlaybackClockMapping(t *testing.T) {
	c := project.Clip{Start: 10000000, In: 2000000, Out: 7000000}
	for _, tc := range []struct {
		source float64
		want   int64
	}{{0, 10000000}, {2, 10000000}, {3.125, 11125000}, {7, 15000000}, {20, 15000000}} {
		if got, ok := SourceToTimeline(c, tc.source); !ok || got != tc.want {
			t.Fatalf("source %v: got %d, want %d", tc.source, got, tc.want)
		}
	}
	if _, ok := SourceToTimeline(c, math.NaN()); ok {
		t.Fatal("accepted NaN")
	}
	for _, tc := range []struct {
		tick int64
		want float64
	}{{0, 2}, {10000000, 2}, {12000000, 4}, {15000000, 6.999999}, {20000000, 6.999999}} {
		if got := TimelineToSource(c, tc.tick); math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("timeline %d: got %f, want %f", tc.tick, got, tc.want)
		}
	}
}

func TestTransportRequestsAndObservations(t *testing.T) {
	c := New(project.Interval{Start: 10000000, End: 15000000}, 12000000, true)
	if s := c.Status(); s.State != Loading || !s.Paused || s.Confirmed {
		t.Fatalf("initial state: %+v", s)
	}
	if !c.Queue(13000000, true, true) || !c.Queue(14000000, true, true) || !c.Queue(14000000, false, false) {
		t.Fatal("request rejected")
	}
	r := c.Request()
	if r.Tick != 14000000 || r.SeekSerial != 2 || r.Serial != 3 || r.Paused {
		t.Fatalf("latest seek lost: %+v", r)
	}
	if c.Report(Sample{Serial: 1, Tick: 13000000, Paused: true}) {
		t.Fatal("stale sample accepted")
	}
	if s := c.Status(); s.State != Seeking || s.Paused || s.Tick != 14000000 || s.Confirmed {
		t.Fatalf("stale sample changed state: %+v", s)
	}
	if !c.Report(Sample{Serial: 3, Tick: 14000000, Paused: true}) || c.Status().State != Paused {
		t.Fatal("backend pause ignored")
	}
	if !c.Queue(14000000, false, false) || !c.Report(Sample{Serial: 4, Tick: 14500000}) || c.Status().State != Playing {
		t.Fatal("resume failed")
	}
	if !c.Report(Sample{Serial: 4, Tick: 15000000, Finished: true}) || c.Status().State != Finished || !c.Status().Paused {
		t.Fatal("end not preserved")
	}
	if c.Report(Sample{Serial: 4, Tick: 14999999}) || c.Queue(15000000, false, false) {
		t.Fatal("finished transport resumed without seek")
	}
	if c.Queue(15000000, true, true) || !c.Queue(12000000, true, true) || c.Status().State != Seeking {
		t.Fatal("exclusive-end seek policy")
	}
	if !c.SetSpeed(1.5) {
		t.Fatal("speed request failed")
	}
	r = c.Request()
	if r.Tick != 12000000 || r.Speed != 1.5 || !r.Paused || r.SeekSerial != 3 {
		t.Fatalf("speed lost seek: %+v", r)
	}
}

func TestFailureAndSessionIsolation(t *testing.T) {
	span := project.Interval{Start: 10, End: 20}
	old := New(span, 10, false)
	old.Stop()
	next := New(span, 12, false)
	if old.Report(Sample{Tick: 15}) || old.Queue(12, true, true) || old.Usable() {
		t.Fatal("stopped session accepted event")
	}
	if next.Status().Tick != 12 || next.Status().State != Loading {
		t.Fatal("old session changed new session")
	}
	if next.Report(Sample{Tick: 21}) || next.Report(Sample{Tick: 9}) {
		t.Fatal("invalid observation accepted")
	}
	next.Fail()
	if next.Usable() || !next.Status().Paused || next.Status().State != Failed || next.Report(Sample{Tick: 13}) {
		t.Fatal("failed session accepted event")
	}
	next.Stop()
	next.Fail()
	if next.Status().State != Stopped {
		t.Fatal("late failure changed stopped state")
	}
	if New(span, 20, false).Usable() {
		t.Fatal("invalid initial position accepted")
	}
}

func TestPendingSeekCoalescing(t *testing.T) {
	c := New(project.Interval{Start: 0, End: 100}, 0, true)
	c.Queue(50, true, true)
	c.Queue(50, true, true)
	if c.Request().SeekSerial != 1 {
		t.Fatal("duplicate pending seek not coalesced")
	}
	if !c.Report(Sample{Serial: c.Request().Serial, Tick: 50, Paused: true}) {
		t.Fatal("latest seek rejected")
	}
	c.Queue(50, true, true)
	if c.Request().SeekSerial != 2 {
		t.Fatal("confirmed seek incorrectly coalesced")
	}
}
