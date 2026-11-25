package main

import (
	"context"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/thumbnail"
)

type thumbnailKey struct {
	path,asset string
	tick int64
	width int
}

type thumbnailEntry struct {
	pixels []byte
	used uint64
	retry time.Time
}

type thumbnailCache struct {
	mu sync.Mutex
	entries map[thumbnailKey]thumbnailEntry
	demand []thumbnailKey
	settled time.Time
	wake chan struct{}
	cancel context.CancelFunc
	done chan struct{}
	revision atomic.Uint64
	sequence uint64
}

func newThumbnailCache(engine media.Engine) *thumbnailCache {
	ctx,cancel:=context.WithCancel(context.Background())
	c:=&thumbnailCache{entries:make(map[thumbnailKey]thumbnailEntry),wake:make(chan struct{},1),cancel:cancel,done:make(chan struct{})}
	go c.run(ctx,engine)
	return c
}

func (c *thumbnailCache) lookup(key thumbnailKey) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	v,ok:=c.entries[key]
	if !ok {return nil}
	c.sequence++
	v.used=c.sequence
	c.entries[key]=v
	return v.pixels
}

func (c *thumbnailCache) request(keys []thumbnailKey) {
	c.mu.Lock()
	if slices.Equal(c.demand,keys) {c.mu.Unlock();return}
	c.demand=append(c.demand[:0],keys...)
	c.settled=time.Now().Add(180*time.Millisecond)
	c.mu.Unlock()
	select {case c.wake<-struct{}{}:default:}
}

func (c *thumbnailCache) next() (thumbnailKey,bool,time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.demand)==0 {return thumbnailKey{},false,0}
	if wait:=time.Until(c.settled);wait>0 {return thumbnailKey{},false,wait}
	var retry time.Duration
	for _,k:=range c.demand {
		if v,ok:=c.entries[k];ok {
			if v.pixels!=nil {continue}
			if wait:=time.Until(v.retry);wait>0 {
				if retry==0 || wait<retry {retry=wait}
				continue
			}
		}
		return k,true,0
	}
	return thumbnailKey{},false,retry
}

func (c *thumbnailCache) fallback(key thumbnailKey,start,end int64) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	var best thumbnailKey
	distance:=int64(0)
	found:=false
	for k,v:=range c.entries {
		if k.path!=key.path || k.asset!=key.asset || k.width!=key.width || v.pixels==nil || k.tick<start || k.tick>=end {continue}
		d:=k.tick-key.tick
		if d<0 {d=-d}
		if !found || d<distance || d==distance && k.tick<best.tick {best,distance,found=k,d,true}
	}
	if !found {return nil}
	v:=c.entries[best]
	c.sequence++
	v.used=c.sequence
	c.entries[best]=v
	return v.pixels
}

func (c *thumbnailCache) store(key thumbnailKey,pixels []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _,exists:=c.entries[key];!exists && len(c.entries)>=256 {
		var oldest thumbnailKey
		age:=uint64(0)
		for k,v:=range c.entries {if age==0 || v.used<age {oldest,age=k,v.used}}
		delete(c.entries,oldest)
	}
	c.sequence++
	c.entries[key]=thumbnailEntry{pixels:pixels,used:c.sequence,retry:time.Now().Add(30*time.Second)}
	c.revision.Add(1)
}

func (c *thumbnailCache) run(ctx context.Context,engine media.Engine) {
	defer close(c.done)
	for {
		if ctx.Err()!=nil {return}
		k,ok,wait:=c.next()
		if !ok {
			if wait<=0 {
				select {case <-ctx.Done():return;case <-c.wake:continue}
			}
			timer:=time.NewTimer(wait)
			select {case <-ctx.Done():timer.Stop();return;case <-c.wake:timer.Stop();case <-timer.C:}
			continue
		}
		job,cancel:=context.WithTimeout(ctx,8*time.Second)
		pixels,err:=engine.Thumbnail(job,k.path,project.Seconds(k.tick),k.width,thumbnail.Height)
		cancel()
		if ctx.Err()!=nil {return}
		if err!=nil {pixels=nil}
		c.store(k,pixels)
	}
}

type bitmapInfo struct {
	Size uint32
	Width,Height int32
	Planes,BitCount uint16
	Compression,SizeImage uint32
	XPelsPerMeter,YPelsPerMeter int32
	ClrUsed,ClrImportant uint32
}

func drawThumbnail(dc uintptr,r rect,pixels []byte,width int) {
	if len(pixels)!=width*thumbnail.Height*4 || r.Right<=r.Left {return}
	info:=bitmapInfo{Size:40,Width:int32(width),Height:-thumbnail.Height,Planes:1,BitCount:32}
	gdi.NewProc("StretchDIBits").Call(dc,uintptr(r.Left),uintptr(r.Top),uintptr(r.Right-r.Left),uintptr(r.Bottom-r.Top),0,0,uintptr(width),thumbnail.Height,uintptr(unsafe.Pointer(&pixels[0])),uintptr(unsafe.Pointer(&info)),0,0x00cc0020)
	runtime.KeepAlive(pixels)
	runtime.KeepAlive(&info)
}

func (e *editor) paintThumbnails(dc uintptr,left,right,w int32,c project.Clip,a project.Asset,demand *[]thumbnailKey) {
	if e.thumbnails==nil || right-left<12 {return}
	width:=thumbnail.Width(a.Width,a.Height)
	scale:=e.scale(w)
	saved,_,_:=gdi.NewProc("SaveDC").Call(dc)
	if saved==0 {return}
	defer gdi.NewProc("RestoreDC").Call(dc,saved)
	gdi.NewProc("IntersectClipRect").Call(dc,uintptr(left),uintptr(laneTop+19),uintptr(right),uintptr(laneBottom-1))
	clipLeft:=float64(trackHeader)+project.Seconds(c.Start-e.origin)*scale
	for _,tile:=range thumbnail.Tiles(int(left),int(right),clipLeft,scale,c.In,c.Out,width) {
		key:=thumbnailKey{path:a.Path,asset:a.ID,tick:tile.Source,width:width}
		r:=rect{int32(tile.FullLeft),laneTop+19,int32(tile.FullRight),laneBottom-1}
		if len(*demand)<96 {*demand=append(*demand,key)}
		pixels:=e.thumbnails.lookup(key)
		if pixels==nil {
			pixels=e.thumbnails.fallback(key,tile.Start,tile.End)
			if pixels==nil {
				radius:=max(int64(2000000),min(int64(60000000),tile.End-tile.Start)*2)
				start:=max(c.In,key.tick-radius)
				end:=c.Out
				if radius<c.Out-key.tick {end=key.tick+radius}
				pixels=e.thumbnails.fallback(key,start,end)
			}
		}
		if pixels!=nil {drawThumbnail(dc,r,pixels,width)}
	}
}

func (e *editor) updateThumbnails() {
	if e.thumbnails==nil {return}
	if rev:=e.thumbnails.revision.Load();rev!=e.thumbnailRevision {
		e.thumbnailRevision=rev
		e.invalidateTimeline()
	}
}
