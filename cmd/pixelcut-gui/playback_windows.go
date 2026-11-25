package main

import (
	"bufio"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/markrai/pixelcut/internal/project"
)

const playbackTimer uintptr = 1

type clockSample struct {
	tick int64
	finished bool
}

type previewClock struct {
	clip project.Clip
	initial int64
	unavailable atomic.Bool
	sample atomic.Pointer[clockSample]
}

type clockEvent struct {
	Event string `json:"event"`
	Name string `json:"name"`
	Data *float64 `json:"data"`
	Reason string `json:"reason"`
}

func newPreviewClock(c project.Clip, marker int64) *previewClock {
	return &previewClock{clip:c,initial:marker}
}

func previewPipeName(configPath string) string {
	return "\\\\.\\pipe\\pixelcut-"+filepath.Base(configPath)
}

func sourceToTimeline(c project.Clip, seconds float64) (int64,bool) {
	if math.IsNaN(seconds) || math.IsInf(seconds,0) {return 0,false}
	if seconds<=project.Seconds(c.In) {return c.Start,true}
	if seconds>=project.Seconds(c.Out) {return c.Start+(c.Out-c.In),true}
	t:=int64(math.Round(seconds*float64(project.TicksPerSecond)))
	if t<c.In {t=c.In}
	if t>c.Out {t=c.Out}
	return c.Start+(t-c.In),true
}

func (c *previewClock) accept(line []byte) {
	var event clockEvent
	if json.Unmarshal(line,&event)!=nil {return}
	switch event.Event {
	case "property-change":
		if event.Name!="time-pos" || event.Data==nil {return}
		if previous:=c.sample.Load();previous!=nil && previous.finished {return}
		t,ok:=sourceToTimeline(c.clip,*event.Data)
		if ok {c.sample.Store(&clockSample{tick:t})}
	case "end-file":
		if event.Reason=="eof" {
			c.sample.Store(&clockSample{tick:c.clip.Start+(c.clip.Out-c.clip.In),finished:true})
		}
	}
}

func (c *previewClock) observe(ctx context.Context, pipe string, processDone <-chan struct{}) {
	deadline:=time.NewTimer(5*time.Second)
	defer deadline.Stop()
	ticker:=time.NewTicker(20*time.Millisecond)
	defer ticker.Stop()
	var f *os.File
	for {
		select {
		case <-ctx.Done():return
		case <-processDone:return
		default:
		}
		var err error
		f,err=os.OpenFile(pipe,os.O_RDWR,0)
		if err==nil {break}
		select {
		case <-ctx.Done():return
		case <-processDone:return
		case <-deadline.C:c.unavailable.Store(true);return
		case <-ticker.C:
		}
	}
	defer f.Close()
	if _,err:=f.WriteString("{\"command\":[\"observe_property\",1,\"time-pos\"]}\n");err!=nil {c.unavailable.Store(true);return}
	scanner:=bufio.NewScanner(f)
	for scanner.Scan() {
		c.accept(scanner.Bytes())
	}
}

func (e *editor) refreshPlaybackMarker() {
	w:=clientRect(e.timeline).Right
	if e.playhead<e.origin {e.origin=e.playhead}
	if e.xAt(e.playhead,w)>=w-12 {
		visible:=math.Max(0,float64(w-trackHeader-24))/e.scale(w)*float64(project.TicksPerSecond)
		if visible<float64(e.playhead) {e.origin=e.playhead-int64(visible)} else {e.origin=0}
	}
	focus,_,_:=user.NewProc("GetFocus").Call()
	if focus!=e.controls[idAt] {change(e.controls[idAt],fmtTime(e.playhead))}
	e.invalidateTimeline()
}

func (e *editor) updatePlaybackClock() {
	if e.clock==nil {return}
	if e.clock.unavailable.Swap(false) {e.tell("Playback clock unavailable; playhead may not follow video.")}
	if s:=e.clock.sample.Load();s!=nil && s.tick!=e.playhead {
		e.playhead=s.tick
		e.refreshPlaybackMarker()
	}
}
