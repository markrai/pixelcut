package main

import (
	"fmt"
	"math"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/markrai/pixelcut/internal/project"
)

const trackHeader int32=92
const laneTop int32=39
const laneBottom int32=91

type paintStruct struct {
	DC uintptr
	Erase int32
	Rect rect
	Restore,Update int32
	Reserved [32]byte
}

type timelineDrag struct {
	index int
	mode string
	x int32
	scale float64
	original,candidate project.Clip
	moved,valid bool
}

func registerTimeline() {
	name:=u("PixelCutTimeline")
	cursor,_,_:=user.NewProc("LoadCursorW").Call(0,32512)
	wc:=windowClass{Size:uint32(unsafe.Sizeof(windowClass{})),Proc:syscall.NewCallback(timelineProc),Instance:instance,Cursor:cursor,Name:name}
	if n,_,err:=user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)));n==0 { panic(err) }
}

func (e *editor) invalidateTimeline() {
	if e.timeline!=0 { user.NewProc("InvalidateRect").Call(e.timeline,0,0) }
}

func (e *editor) endTime() int64 {
	var end int64
	for _,c:=range e.track().Clips {
		if n:=c.Start+(c.Out-c.In);n>end {end=n}
	}
	return end
}

func (e *editor) scale(w int32) float64 {
	if e.zoom>0 { return e.zoom }
	return math.Max(1e-9,math.Max(1,float64(w-trackHeader-18))/math.Max(5,project.Seconds(e.endTime())*1.05))
}

func (e *editor) timeAt(x,w int32) int64 {
	t:=float64(e.origin)+float64(x-trackHeader)/e.scale(w)*float64(project.TicksPerSecond)
	if t<0 { return 0 };if t>=float64(math.MaxInt64) {return math.MaxInt64}
	return int64(math.Round(t))
}

func (e *editor) xAt(t int64,w int32) int32 {
	x:=float64(trackHeader)+project.Seconds(t-e.origin)*e.scale(w)
	if x < -100000 { return -100000 };if x>100000 {return 100000}
	return int32(math.Round(x))
}

func (e *editor) zoomTimeline(id int) {
	w:=clientRect(e.timeline).Right
	if id==idFit { e.zoom,e.origin=0,0 } else {
		factor:=1.5
		if id==idZoomOut { factor=1/factor }
		e.zoom=math.Max(.05,math.Min(1000,e.scale(w)*factor))
		e.origin=e.playhead-int64(float64(w-trackHeader)/e.zoom*float64(project.TicksPerSecond)/2)
		if e.origin<0 {e.origin=0}
	}
	e.invalidateTimeline()
}

func stamp(t int64) string {
	s:=t/project.TicksPerSecond
	return fmt.Sprintf("%02d:%02d",s/60,s%60)
}

func (e *editor) paintTimeline(dc uintptr,r rect) {
	w,h:=r.Right,r.Bottom
	fill(dc,r,palette.black)
	fill(dc,rect{0,0,w,32},palette.panel)
	fill(dc,rect{0,33,trackHeader,h},palette.panel)
	fill(dc,rect{trackHeader,33,w,97},palette.bg)
	fill(dc,rect{trackHeader,99,w,145},palette.bg)
	textAt(dc,rect{10,0,trackHeader-4,32},"TIME",palette.muted,4)
	textAt(dc,rect{12,laneTop,trackHeader-4,laneBottom},"V1  Video",palette.text,4)
	textAt(dc,rect{12,99,trackHeader-4,145},"A1  Linked",palette.muted,4)
	step:=int64(project.TicksPerSecond)
	for float64(step)/float64(project.TicksPerSecond)*e.scale(w)<72 && step<math.MaxInt64/2 {step*=2}
	start:=e.origin/step*step
	for t:=start;t>=0;t+=step {
		x:=e.xAt(t,w)
		if x>=w {break}
		if x>=trackHeader {
			fill(dc,rect{x,28,x+1,h},palette.line)
			textAt(dc,rect{x+4,0,x+70,28},stamp(t),palette.muted,4)
		}
		if t>math.MaxInt64-step {break}
	}
	for i,c:=range e.track().Clips {
		valid:=true
		if e.drag!=nil && e.drag.index==i && e.drag.moved {c=e.drag.candidate;valid=e.drag.valid}
		left,right:=e.xAt(c.Start,w),e.xAt(c.Start+(c.Out-c.In),w)
		if right<=trackHeader || left>=w {continue}
		if left<trackHeader {left=trackHeader};if right>w {right=w}
		if right<=left {right=left+1}
		col:=palette.clip
		if i==e.clip {col=palette.selected}
		if !valid {col=rgb(136,55,61)}
		fill(dc,rect{left,laneTop,right,laneBottom},col)
		if i==e.clip && right-left>12 {
			fill(dc,rect{left,laneTop,left+3,laneBottom},palette.accent)
			fill(dc,rect{right-3,laneTop,right,laneBottom},palette.accent)
		}
		a,_:=e.p.Asset(c.AssetID)
		if right-left>20 {
			textAt(dc,rect{left+8,laneTop+3,right-7,laneTop+28},filepath.Base(a.Path),palette.text,4)
			textAt(dc,rect{left+8,laneTop+27,right-7,laneBottom},fmt.Sprintf("%.2fs",project.Seconds(c.Out-c.In)),palette.text,4)
		}
		if a.HasAudio {
			fill(dc,rect{left,104,right,140},palette.audio)
			if right-left>35 {textAt(dc,rect{left+8,104,right-5,140},"Audio",palette.muted,4)}
		}
	}
	x:=e.xAt(e.playhead,w)
	if x>=trackHeader && x<w {
		fill(dc,rect{x,25,x+2,h},palette.accent)
		fill(dc,rect{x-4,22,x+6,28},palette.accent)
	}
	if h>150 {textAt(dc,rect{trackHeader+8,150,w-8,h},fmt.Sprintf("%s  |  Wheel: scroll   Ctrl+wheel: zoom   Drag clip or edges",stamp(e.playhead)),palette.muted,4)}
}

func (e *editor) selectClip(i int) {
	e.clip=i
	send.Call(e.controls[idClips],lbSet,uintptr(i),0)
	e.fields()
	e.invalidateTimeline()
}

func (e *editor) beginTimeline(x,y,w int32) {
	if e.busy || x<trackHeader {return}
	e.stopPreview()
	e.playhead=e.timeAt(x,w)
	for i,c:=range e.track().Clips {
		l,r:=e.xAt(c.Start,w),e.xAt(c.Start+(c.Out-c.In),w)
		if mode:=videoHitMode(x,y,l,r);mode!="" {
			e.selectClip(i)
			e.drag=&timelineDrag{index:i,mode:mode,x:x,scale:e.scale(w),original:c,candidate:c,valid:true}
			user.NewProc("SetCapture").Call(e.timeline)
			e.setTimelineCursor(x,y,w)
			return
		}
		if x>=l && x<r && y>=104 && y<140 {
			e.selectClip(i);return
		}
	}
	change(e.controls[idAt],fmtTime(e.playhead))
	e.invalidateTimeline()
}

func (e *editor) moveTimeline(x int32) {
	d:=e.drag
	if d==nil {return}
	if !d.moved && math.Abs(float64(x-d.x))<4 {return}
	d.moved=true
	raw:=float64(x-d.x)/d.scale*float64(project.TicksPerSecond)
	raw=math.Max(-1e15,math.Min(1e15,raw))
	delta:=int64(math.Round(raw/10000))*10000
	c,err:=e.p.DragClip("V1",d.original.ID,d.mode,delta)
	d.candidate=c;d.valid=err==nil
	e.invalidateTimeline()
}

func (e *editor) finishTimeline() {
	d:=e.drag;e.drag=nil
	user.NewProc("ReleaseCapture").Call()
	if d!=nil && d.moved {
		if d.valid {
			q:=e.p.Clone();q.Tracks[0].Clips[d.index]=d.candidate
			e.playhead=d.candidate.Start
			if err:=e.commit(q);err!=nil {e.tell(err.Error())} else {e.tell("Timeline edit applied.")}
		} else {e.tell("Edit rejected: clips cannot overlap or exceed source bounds.")}
	}
	e.invalidateTimeline()
}

func timelineProc(hwnd uintptr,msg uint32,wp,lp uintptr) uintptr {
	if ui==nil {r,_,_:=defWindow.Call(hwnd,uintptr(msg),wp,lp);return r}
	x,y:=int32(int16(lp&0xffff)),int32(int16((lp>>16)&0xffff))
	switch msg {
	case 0x000f:
		var ps paintStruct
		dc,_,_:=user.NewProc("BeginPaint").Call(hwnd,uintptr(unsafe.Pointer(&ps)))
		r:=clientRect(hwnd)
		if dc!=0 {
			mem,_,_:=gdi.NewProc("CreateCompatibleDC").Call(dc)
			bmp,_,_:=gdi.NewProc("CreateCompatibleBitmap").Call(dc,uintptr(r.Right),uintptr(r.Bottom))
			if mem!=0 && bmp!=0 {
				old,_,_:=gdi.NewProc("SelectObject").Call(mem,bmp)
				ui.paintTimeline(mem,r)
				gdi.NewProc("BitBlt").Call(dc,0,0,uintptr(r.Right),uintptr(r.Bottom),mem,0,0,0x00cc0020)
				gdi.NewProc("SelectObject").Call(mem,old)
			} else {ui.paintTimeline(dc,r)}
			if bmp!=0 {gdi.NewProc("DeleteObject").Call(bmp)}
			if mem!=0 {gdi.NewProc("DeleteDC").Call(mem)}
		}
		user.NewProc("EndPaint").Call(hwnd,uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0014:return 1
	case 0x0020:
		if lp&0xffff==1 {
			var p point
			ok,_,_:=user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
			if ok!=0 {
				user.NewProc("ScreenToClient").Call(hwnd,uintptr(unsafe.Pointer(&p)))
				ui.setTimelineCursor(p.X,p.Y,clientRect(hwnd).Right)
				return 1
			}
		}
	case 0x0201:
		user.NewProc("SetFocus").Call(hwnd)
		ui.beginTimeline(x,y,clientRect(hwnd).Right);return 0
	case 0x0200:
		ui.moveTimeline(x)
		ui.setTimelineCursor(x,y,clientRect(hwnd).Right)
		return 0
	case 0x0202:
		ui.moveTimeline(x);ui.finishTimeline()
		ui.setTimelineCursor(x,y,clientRect(hwnd).Right)
		return 0
	case 0x0215:
		ui.drag=nil;ui.invalidateTimeline();return 0
	case 0x020a:
		if ui.busy || ui.drag!=nil {return 0}
		d:=int16(wp>>16)
		if wp&8!=0 {
			if d>0 {ui.zoomTimeline(idZoomIn)} else {ui.zoomTimeline(idZoomOut)}
		} else {
			w:=clientRect(hwnd).Right
			step:=int64(math.Min(float64(math.MaxInt64/2),math.Max(0,float64(w-trackHeader)/ui.scale(w)*float64(project.TicksPerSecond)/5)))
			if d>0 {ui.origin-=step;if ui.origin<0 {ui.origin=0}} else if ui.origin<=math.MaxInt64-step {ui.origin+=step}
			ui.invalidateTimeline()
		}
		return 0
	}
	r,_,_:=defWindow.Call(hwnd,uintptr(msg),wp,lp);return r
}

const (
	cursorArrow uintptr=32512
	cursorCross uintptr=32515
	cursorSizeWE uintptr=32644
	cursorSizeAll uintptr=32646
	cursorNo uintptr=32648
	cursorHand uintptr=32649
)

func videoHitMode(x,y,left,right int32) string {
	if x<trackHeader || x<left || x>=right || y<laneTop || y>=laneBottom {return ""}
	if right-left>=18 {
		if x-left<7 {return "left"}
		if right-x<=7 {return "right"}
	}
	return "move"
}

func cursorForMode(mode string,invalid bool) uintptr {
	if invalid {return cursorNo}
	switch mode {
	case "move":return cursorSizeAll
	case "left","right":return cursorSizeWE
	default:return cursorArrow
	}
}

func (e *editor) timelineCursor(x,y,w int32) uintptr {
	if e.busy {return cursorArrow}
	if e.drag!=nil {return cursorForMode(e.drag.mode,e.drag.moved && !e.drag.valid)}
	if x<trackHeader || x>=w {return cursorArrow}
	for _,c:=range e.track().Clips {
		l,r:=e.xAt(c.Start,w),e.xAt(c.Start+(c.Out-c.In),w)
		if mode:=videoHitMode(x,y,l,r);mode!="" {return cursorForMode(mode,false)}
		if x>=l && x<r && y>=104 && y<140 {
			if a,ok:=e.p.Asset(c.AssetID);ok && a.HasAudio {return cursorHand}
		}
	}
	if y>=0 && y<32 {return cursorCross}
	return cursorArrow
}

func (e *editor) setTimelineCursor(x,y,w int32) {
	cursor,_,_:=user.NewProc("LoadCursorW").Call(0,e.timelineCursor(x,y,w))
	if cursor!=0 {user.NewProc("SetCursor").Call(cursor)}
}
