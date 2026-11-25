package main

import (
	"fmt"
	"math"
	"path/filepath"
	"unsafe"

	"github.com/markrai/pixelcut/internal/project"
)

type placementRequest struct {
	start,marker,tolerance int64
}

type assetDrag struct {
	id,clipID string
	origin point
	moved,over,valid bool
	placement project.AssetPlacement
}

func (e *editor) placementAt(x,y int32) (*placementRequest,bool) {
	r:=clientRect(e.timeline)
	if x<trackHeader || x>=r.Right || y<33 || y>=97 {return nil,false}
	scale:=e.scale(r.Right)
	tolerance:=int64(0)
	if !e.snapDisabled && !keyPressed(0x10) {
		tolerance=int64(math.Min(2000000,math.Round(8/scale*float64(project.TicksPerSecond))))
	}
	return &placementRequest{e.timeAt(x,r.Right),e.playhead,tolerance},true
}

func (e *editor) beginAssetDrag(hwnd uintptr,lp uintptr) bool {
	if e.busy || e.drag!=nil || e.scrubbing || e.cropSession!=nil || e.transformSession!=nil {return false}
	r,_,_:=send.Call(hwnd,0x01a9,0,lp)
	index:=int(r&0xffff)
	if (r>>16)&0xffff!=0 || index>=len(e.p.Assets) {return false}
	var item rect
	ok,_,_:=send.Call(hwnd,0x0198,uintptr(index),uintptr(unsafe.Pointer(&item)))
	y:=int32(int16(lp>>16))
	if int32(ok)<0 || y<item.Top || y>=item.Bottom {return false}
	var p point
	if ok,_,_:=user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)));ok==0 {return false}
	user.NewProc("SetFocus").Call(hwnd)
	send.Call(hwnd,lbSet,uintptr(index),0)
	e.asset=index
	e.assetDrag=&assetDrag{id:e.p.Assets[index].ID,clipID:nextID("clip"),origin:p}
	user.NewProc("SetCapture").Call(hwnd)
	return true
}

func (e *editor) updateAssetDrag() {
	d:=e.assetDrag
	if d==nil {return}
	var p point
	ok,_,_:=user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	if ok==0 {return}
	if !d.moved && math.Abs(float64(p.X-d.origin.X))<4 && math.Abs(float64(p.Y-d.origin.Y))<4 {return}
	if !d.moved {e.stopPreview()}
	d.moved=true
	user.NewProc("ScreenToClient").Call(e.timeline,uintptr(unsafe.Pointer(&p)))
	req,over:=e.placementAt(p.X,p.Y)
	d.over,d.valid=over,false
	if over {
		var err error
		d.placement,err=e.p.PlaceAssets("V1",[]string{d.id},[]string{d.clipID},req.start,req.marker,req.tolerance)
		d.valid=err==nil
	}
	cursor:=cursorNo
	if d.valid {cursor=cursorSizeAll}
	h,_,_:=user.NewProc("LoadCursorW").Call(0,cursor)
	if h!=0 {user.NewProc("SetCursor").Call(h)}
	e.invalidateTimeline()
}

func (e *editor) cancelAssetDrag() {
	if e.assetDrag==nil {return}
	e.assetDrag=nil
	user.NewProc("ReleaseCapture").Call()
	e.invalidateTimeline()
}

func (e *editor) finishAssetDrag() {
	e.updateAssetDrag()
	d:=e.assetDrag
	if d==nil {return}
	e.cancelAssetDrag()
	if !d.moved {e.checkpoint();return}
	if !d.over {return}
	if !d.valid {e.tell("Cannot place asset here: clips cannot overlap.");return}
	if err:=e.commitPlacement(d.placement);err!=nil {e.tell(err.Error());return}
	user.NewProc("SetFocus").Call(e.timeline)
	e.tell("Asset placed on timeline.")
}

func (e *editor) commitPlacement(p project.AssetPlacement) error {
	if err:=p.Project.Validate();err!=nil {return err}
	e.clip=p.FirstClip
	return e.commit(p.Project)
}

func (e *editor) paintAssetGhost(dc uintptr,w int32) {
	d:=e.assetDrag
	if d==nil || !d.moved || !d.over {return}
	left:=max(trackHeader,e.xAt(d.placement.Start,w))
	right:=min(w,e.xAt(d.placement.End,w))
	if right<=left {return}
	a,ok:=e.p.Asset(d.id)
	if !ok {return}
	col:=rgb(112,220,241)
	if !d.valid {col=rgb(242,105,112)}
	r:=rect{left,laneTop,right,laneBottom}
	h,_,_:=gdi.NewProc("CreateHatchBrush").Call(5,col)
	if h!=0 {
		gdi.NewProc("SetBkMode").Call(dc,1)
		user.NewProc("FillRect").Call(dc,uintptr(unsafe.Pointer(&r)),h)
		gdi.NewProc("DeleteObject").Call(h)
	}
	fill(dc,rect{left,laneTop,right,laneTop+2},col)
	fill(dc,rect{left,laneBottom-2,right,laneBottom},col)
	fill(dc,rect{left,laneTop,min(left+2,right),laneBottom},col)
	fill(dc,rect{max(left,right-2),laneTop,right,laneBottom},col)
	if right-left>25 {
		name:=fmt.Sprintf("%s  %.2fs",filepath.Base(a.Path),project.Seconds(a.Duration))
		fill(dc,rect{left+2,laneTop+3,right-2,laneTop+22},palette.panel)
		textAt(dc,rect{left+5,laneTop+3,right-5,laneTop+22},name,col,4)
	}
	if d.valid && d.placement.Snapped {
		x:=e.xAt(d.placement.Target,w)
		if x>=trackHeader && x<w {fill(dc,rect{x,28,x+2,145},col)}
	}
}
