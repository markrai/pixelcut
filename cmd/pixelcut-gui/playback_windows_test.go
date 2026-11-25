package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestPlaybackClockMapping(t *testing.T) {
	c := project.Clip{Start: 10000000, In: 2000000, Out: 7000000}
	for _, tc := range []struct {
		source float64
		want   int64
	}{{0, 10000000}, {2, 10000000}, {3.125, 11125000}, {7, 15000000}, {20, 15000000}} {
		if got, ok := sourceToTimeline(c, tc.source); !ok || got != tc.want {
			t.Fatalf("source %v: got %d, want %d", tc.source, got, tc.want)
		}
	}
	if _, ok := sourceToTimeline(c, math.NaN()); ok {
		t.Fatal("accepted NaN")
	}
	for _, tc := range []struct {
		tick int64
		want float64
	}{{0, 2}, {10000000, 2}, {12000000, 4}, {15000000, 6.999999}, {20000000, 6.999999}} {
		if got := timelineToSource(c, tc.tick); math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("timeline %d: got %f, want %f", tc.tick, got, tc.want)
		}
	}
}

func TestPlaybackRequestCoalescing(t *testing.T) {
	c := newPreviewClock(project.Clip{Start: 10000000, In: 2000000, Out: 7000000}, 12000000, true)
	if !c.paused() {
		t.Fatal("initial pause lost")
	}
	c.queue(13000000, true, true)
	c.queue(14000000, true, true)
	c.queue(14000000, false, false)
	r := c.request.Load()
	if r.tick != 14000000 || r.seekSerial != 2 || r.serial != 3 || r.paused {
		t.Fatalf("latest seek lost: %+v", r)
	}
	c.sample.Store(&clockSample{serial: 1, paused: true})
	if c.paused() {
		t.Fatal("stale sample overrode request")
	}
	c.sample.Store(&clockSample{serial: 3, paused: true})
	if !c.paused() {
		t.Fatal("player-side pause ignored")
	}
}

func TestPreviewIPCCommand(t *testing.T) {
	var output bytes.Buffer
	p := previewIPC{file: &output, reader: bufio.NewScanner(strings.NewReader("{\"event\":\"playback-restart\"}\n{\"request_id\":99,\"error\":\"success\"}\n{\"request_id\":1,\"error\":\"success\"}\n{\"request_id\":2,\"error\":\"property unavailable\"}\n"))}
	if _, err := p.call("seek", 4.25, "absolute+exact"); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Command   []any  `json:"command"`
		RequestID uint64 `json:"request_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &sent); err != nil || sent.RequestID != 1 || len(sent.Command) != 3 || sent.Command[0] != "seek" || sent.Command[1] != 4.25 || sent.Command[2] != "absolute+exact" {
		t.Fatalf("invalid seek command %q: %v", output.String(), err)
	}
	if _, err := p.call("get_property", "time-pos"); err == nil {
		t.Fatal("mpv error ignored")
	}
}
