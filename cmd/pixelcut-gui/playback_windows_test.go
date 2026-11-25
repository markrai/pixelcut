package main

import (
	"math"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestPlaybackClockMapping(t *testing.T) {
	c:=project.Clip{Start:10000000,In:2000000,Out:7000000}
	for _,tc:=range []struct{source float64;want int64}{{0,10000000},{2,10000000},{3.125,11125000},{7,15000000},{20,15000000}} {
		if got,ok:=sourceToTimeline(c,tc.source);!ok || got!=tc.want {t.Fatalf("source %v: got %d, want %d",tc.source,got,tc.want)}
	}
	if _,ok:=sourceToTimeline(c,math.NaN());ok {t.Fatal("accepted NaN")}
}

func TestPlaybackClockEvents(t *testing.T) {
	c:=newPreviewClock(project.Clip{Start:10000000,In:2000000,Out:7000000},12000000)
	c.accept([]byte("{\"event\":\"property-change\",\"name\":\"time-pos\",\"data\":null}"))
	if c.sample.Load()!=nil {t.Fatal("null clock value accepted")}
	c.accept([]byte("{\"event\":\"property-change\",\"name\":\"time-pos\",\"data\":4.25}"))
	if got:=c.sample.Load();got==nil || got.tick!=12250000 || got.finished {t.Fatal("incorrect playback clock")}
	c.accept([]byte("{\"event\":\"end-file\",\"reason\":\"quit\"}"))
	if got:=c.sample.Load();got.tick!=12250000 || got.finished {t.Fatal("quit jumped to end")}
	c.accept([]byte("{\"event\":\"end-file\",\"reason\":\"eof\"}"))
	c.accept([]byte("{\"event\":\"property-change\",\"name\":\"time-pos\",\"data\":0}"))
	if got:=c.sample.Load();got.tick!=15000000 || !got.finished {t.Fatal("EOF did not preserve end position")}
}
