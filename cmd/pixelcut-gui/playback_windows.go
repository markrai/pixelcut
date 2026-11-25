package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	paused, finished bool
	serial uint64
}

type previewRequest struct {
	serial, seekSerial uint64
	tick int64
	paused bool
}

type previewClock struct {
	clip project.Clip
	unavailable atomic.Bool
	sample atomic.Pointer[clockSample]
	request atomic.Pointer[previewRequest]
	cropRequest atomic.Pointer[cropRequest]
	cropSample atomic.Pointer[cropSample]
	cropError atomic.Bool
}

func newPreviewClock(c project.Clip, marker int64, paused bool) *previewClock {
	clock:=&previewClock{clip:c}
	clock.request.Store(&previewRequest{tick:marker,paused:paused})
	return clock
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

func timelineToSource(c project.Clip, tick int64) float64 {
	if tick<c.Start {tick=c.Start}
	end:=c.Start+(c.Out-c.In)
	if tick>=end {tick=end-1}
	return project.Seconds(c.In+(tick-c.Start))
}

func (c *previewClock) paused() bool {
	r:=c.request.Load()
	if s:=c.sample.Load();s!=nil && s.serial==r.serial {return s.paused}
	return r.paused
}

func (c *previewClock) queue(tick int64, seek, paused bool) {
	r:=*c.request.Load()
	if seek && r.seekSerial!=0 && r.tick==tick {
		if s:=c.sample.Load();s==nil || s.serial!=r.serial {seek=false}
	}
	r.serial++
	if seek {r.seekSerial++;r.tick=tick}
	r.paused=paused
	c.request.Store(&r)
}

type ipcReply struct {
	RequestID uint64 `json:"request_id"`
	Error string `json:"error"`
	Data json.RawMessage `json:"data"`
}

type previewIPC struct {
	file io.Writer
	reader *bufio.Scanner
	id uint64
}

func (p *previewIPC) call(command ...any) (json.RawMessage,error) {
	p.id++
	b,err:=json.Marshal(map[string]any{"command":command,"request_id":p.id})
	if err!=nil {return nil,err}
	if _,err=p.file.Write(append(b,'\n'));err!=nil {return nil,err}
	for p.reader.Scan() {
		var r ipcReply
		if json.Unmarshal(p.reader.Bytes(),&r)!=nil || r.RequestID!=p.id {continue}
		if r.Error!="success" {return nil,fmt.Errorf("mpv: %s",r.Error)}
		return r.Data,nil
	}
	if err=p.reader.Err();err!=nil {return nil,err}
	return nil,errors.New("mpv IPC closed")
}

func (p *previewIPC) property(name string, value any) bool {
	b,err:=p.call("get_property",name)
	return err==nil && len(b)>0 && string(b)!="null" && json.Unmarshal(b,value)==nil
}

func (c *previewClock) observe(ctx context.Context, pipe string, processDone <-chan struct{}) {
	deadline:=time.NewTimer(5*time.Second)
	defer deadline.Stop()
	ticker:=time.NewTicker(25*time.Millisecond)
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
	p:=previewIPC{file:f,reader:bufio.NewScanner(f)}
	var applied, appliedSeek, appliedCrop uint64
	var seekUntil time.Time
	var failures int
	for {
		select {
		case <-ctx.Done():return
		case <-processDone:return
		case <-ticker.C:
		}
		var source float64
		if !p.property("time-pos",&source) {
			failures++
			if failures>=80 {c.unavailable.Store(true);return}
			continue
		}
		failures=0
		if req:=c.cropRequest.Load();req!=nil && req.serial!=appliedCrop {
			var ready bool
			if p.property("user-data/pixelcut/crop-ready",&ready) && ready {
				if _,err:=p.call("script-message","pixelcut-crop",req.payload());err!=nil {c.cropError.Store(true)} else {appliedCrop=req.serial}
			}
		}
		if appliedCrop!=0 {
			var state cropSample
			if p.property("user-data/pixelcut/crop",&state) && state.Serial==appliedCrop {c.cropSample.Store(&state)}
		}
		r:=c.request.Load()
		if r.serial!=applied {
			if r.paused {
				if _,err:=p.call("set_property","pause",true);err!=nil {c.unavailable.Store(true);return}
			}
			if r.seekSerial!=appliedSeek {
				if _,err:=p.call("seek",timelineToSource(c.clip,r.tick),"absolute+exact");err!=nil {c.unavailable.Store(true);return}
				appliedSeek=r.seekSerial
				seekUntil=time.Now().Add(2*time.Second)
			}
			if !r.paused {
				if _,err:=p.call("set_property","pause",false);err!=nil {c.unavailable.Store(true);return}
			}
			applied=r.serial
			continue
		}
		var seeking,paused,eof bool
		if !p.property("seeking",&seeking) || seeking {continue}
		if !p.property("pause",&paused) {continue}
		p.property("eof-reached",&eof)
		tick,ok:=sourceToTimeline(c.clip,source)
		if !ok {continue}
		if !seekUntil.IsZero() {
			distance:=math.Abs(float64(tick)-float64(r.tick))
			if distance>250000 && time.Now().Before(seekUntil) {continue}
			seekUntil=time.Time{}
		}
		if eof {
			tick=c.clip.Start+(c.clip.Out-c.clip.In)
			paused=true
		}
		c.sample.Store(&clockSample{tick:tick,paused:paused,finished:eof,serial:applied})
	}
}

func (e *editor) refreshPlaybackMarker() {
	w:=clientRect(e.timeline).Right
	if !e.scrubbing && e.playhead<e.origin {e.origin=e.playhead}
	if !e.scrubbing && e.xAt(e.playhead,w)>=w-12 {
		visible:=math.Max(0,float64(w-trackHeader-24))/e.scale(w)*float64(project.TicksPerSecond)
		if visible<float64(e.playhead) {e.origin=e.playhead-int64(visible)} else {e.origin=0}
	}
	focus,_,_:=user.NewProc("GetFocus").Call()
	if focus!=e.controls[idAt] {change(e.controls[idAt],fmtTime(e.playhead))}
	e.invalidateTimeline()
}

func (e *editor) updatePlayButton(paused bool) {
	label:="Stop  (Space)"
	if paused {label="Play  (Space)"}
	h:=e.controls[idPlay]
	if h!=0 && read(h)!=label {
		change(h,label)
		user.NewProc("InvalidateRect").Call(h,0,1)
	}
}

func (e *editor) updatePlaybackClock() {
	if e.clock==nil {return}
	if e.clock.unavailable.Swap(false) {e.tell("Playback IPC unavailable; reopen the preview to seek.")}
	if e.scrubbing || e.drag!=nil {return}
	if s:=e.clock.sample.Load();s!=nil && s.serial==e.clock.request.Load().serial {
		e.updatePlayButton(s.paused)
		if s.tick!=e.playhead {
			e.playhead=s.tick
			e.refreshPlaybackMarker()
		}
	}
}

func (e *editor) pausePreview() error {
	if e.clock!=nil && e.previewActive() {
		e.clock.queue(e.playhead,false,true)
		e.updatePlayButton(true)
		e.tell("Preview paused.")
	}
	return nil
}

func (e *editor) playing() bool {
	return e.clock!=nil && e.previewActive() && !e.clock.paused()
}

func (e *editor) seekTimeline(t int64, playing bool) {
	end:=e.endTime()
	if t<0 {t=0}
	if t>end {t=end}
	index:=-1
	for i,c:=range e.track().Clips {
		if t>=c.Start && t<c.Start+(c.Out-c.In) {index=i;break}
		if t==end && t==c.Start+(c.Out-c.In) {index=i}
	}
	if index<0 {
		e.stopPreview()
		e.playhead=t
		e.selectClip(-1)
		e.refreshPlaybackMarker()
		e.tell("No video at this timeline position.")
		return
	}
	c:=e.track().Clips[index]
	if t==c.Start+(c.Out-c.In) {playing=false}
	if e.clock!=nil && e.previewActive() && e.clock.clip.ID==c.ID {
		e.playhead=t
		if e.clip!=index {e.selectClip(index)}
		e.clock.queue(t,true,!playing)
		e.updatePlayButton(!playing)
		e.refreshPlaybackMarker()
		return
	}
	e.stopPreview()
	e.playhead=t
	e.selectClip(index)
	if t==c.Start+(c.Out-c.In) {e.playhead=t-1;playing=false}
	if err:=e.startPreview(!playing);err!=nil {e.tell(err.Error())}
}
