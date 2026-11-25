package main

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/waveform"
)

type waveformKey struct {
	path,asset string
	duration int64
}

type waveformEntry struct {
	peaks *waveform.Peaks
	used uint64
	retry time.Time
}

type waveformCache struct {
	mu sync.Mutex
	entries map[waveformKey]waveformEntry
	demand []waveformKey
	wake chan struct{}
	cancel context.CancelFunc
	done chan struct{}
	revision atomic.Uint64
	sequence uint64
}

func newWaveformCache(engine media.Engine) *waveformCache {
	ctx,cancel:=context.WithCancel(context.Background())
	c:=&waveformCache{entries:make(map[waveformKey]waveformEntry),wake:make(chan struct{},1),cancel:cancel,done:make(chan struct{})}
	go c.run(ctx,engine)
	return c
}

func (c *waveformCache) lookup(key waveformKey) *waveform.Peaks {
	c.mu.Lock()
	defer c.mu.Unlock()
	v,ok:=c.entries[key]
	if !ok {return nil}
	c.sequence++
	v.used=c.sequence
	c.entries[key]=v
	return v.peaks
}

func (c *waveformCache) request(keys []waveformKey) {
	c.mu.Lock()
	c.demand=append(c.demand[:0],keys...)
	c.mu.Unlock()
	select {case c.wake<-struct{}{}:default:}
}

func (c *waveformCache) next() (waveformKey,bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.demand)>0 {
		k:=c.demand[0]
		c.demand=c.demand[1:]
		if v,ok:=c.entries[k];ok && (v.peaks!=nil || time.Now().Before(v.retry)) {continue}
		return k,true
	}
	return waveformKey{},false
}

func (c *waveformCache) store(key waveformKey,peaks *waveform.Peaks) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _,ok:=c.entries[key];!ok && len(c.entries)>=32 {
		var oldest waveformKey
		age:=uint64(0)
		for k,v:=range c.entries {if age==0 || v.used<age {oldest,age=k,v.used}}
		delete(c.entries,oldest)
	}
	c.sequence++
	c.entries[key]=waveformEntry{peaks:peaks,used:c.sequence,retry:time.Now().Add(time.Minute)}
	c.revision.Add(1)
}

func (c *waveformCache) run(ctx context.Context,engine media.Engine) {
	defer close(c.done)
	for {
		if ctx.Err()!=nil {return}
		k,ok:=c.next()
		if !ok {
			select {case <-ctx.Done():return;case <-c.wake:continue}
		}
		job,cancel:=context.WithTimeout(ctx,90*time.Second)
		peaks,err:=engine.Waveform(job,k.path,k.duration)
		cancel()
		if ctx.Err()!=nil {return}
		if err!=nil {peaks=nil}
		c.store(k,peaks)
	}
}

func waveformInterval(in,out int64,clipLeft,scale float64,x,right int32) (int64,int64) {
	if scale<=0 || out<=in {return in,in}
	duration:=out-in
	offset:=func(px int32) int64 {
		v:=(float64(px)-clipLeft)/scale*float64(project.TicksPerSecond)
		if v<=0 {return 0}
		if v>=float64(duration) {return duration}
		return int64(math.Round(v))
	}
	start,end:=in+offset(x),in+offset(right)
	return start,end
}

func (e *editor) paintWaveform(dc uintptr,left,right,w int32,c project.Clip,a project.Asset,demand *[]waveformKey) {
	if e.waveforms==nil || right<=left {return}
	key:=waveformKey{path:a.Path,asset:a.ID,duration:a.Duration}
	p:=e.waveforms.lookup(key)
	if p==nil {
		if len(*demand)<32 {
			duplicate:=false
			for _,k:=range *demand {if k==key {duplicate=true;break}}
			if !duplicate {*demand=append(*demand,key)}
		}
		if right-left>65 {textAt(dc,rect{left+8,104,right-5,140},"Audio",palette.muted,4)}
		return
	}
	scale:=e.scale(w)
	clipLeft:=float64(trackHeader)+project.Seconds(c.Start-e.origin)*scale
	center:=int32(122)
	color:=rgb(133,201,156)
	fill(dc,rect{left,center,right,center+1},rgb(82,115,92))
	for x:=left;x<right;x+=2 {
		r:=min(x+2,right)
		start,end:=waveformInterval(c.In,c.Out,clipLeft,scale,x,r)
		peak:=p.Peak(start,end)
		if peak==0 {continue}
		h:=int32(math.Max(1,math.Round(float64(peak)*16/32768)))
		fill(dc,rect{x,center-h,x+1,center+h+1},color)
	}
}

func (e *editor) updateWaveforms() {
	if e.waveforms==nil {return}
	if rev:=e.waveforms.revision.Load();rev!=e.waveformRevision {
		e.waveformRevision=rev
		e.invalidateTimeline()
	}
}
