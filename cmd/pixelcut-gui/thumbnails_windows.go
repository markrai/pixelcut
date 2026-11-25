package main

import (
	"context"
	"runtime"
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
	c.demand=append(c.demand[:0],keys...)
	c.mu.Unlock()
	select {case c.wake<-struct{}{}:default:}
}

func (c *thumbnailCache) next() (thumbnailKey,bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.demand)>0 {
		k:=c.demand[0]
		c.demand=c.demand[1:]
		if v,ok:=c.entries[k];ok && (v.pixels!=nil || time.Now().Before(v.retry)) {continue}
		return k,true
	}
	return thumbnailKey{},false
}

func (c *thumbnailCache) store(key thumbnailKey,pixels []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries)>=256 {
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
		k,ok:=c.next()
		if !ok {
			select {case <-ctx.Done():return;case <-c.wake:continue}
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
		r:=rect{int32(tile.FullLeft),laneTop+19,int32(tile.FullLeft+width),laneBottom-1}
		if pixels:=e.thumbnails.lookup(key);pixels!=nil {
			drawThumbnail(dc,r,pixels,width)
		} else if len(*demand)<96 {
			*demand=append(*demand,key)
		}
	}
}

func (e *editor) updateThumbnails() {
	if e.thumbnails==nil {return}
	if rev:=e.thumbnails.revision.Load();rev!=e.thumbnailRevision {
		e.thumbnailRevision=rev
		e.invalidateTimeline()
	}
}
