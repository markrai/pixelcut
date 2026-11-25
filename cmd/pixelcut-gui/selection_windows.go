package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/markrai/pixelcut/internal/project"
)

type marqueeDrag struct {
	x,y,currentX,currentY int32
	before []string
	primary int
	additive,moved bool
	clickTime int64
	wasPlaying bool
}

func (e *editor) selectionIDs() []string {
	if len(e.group)>1 {
		var ids []string
		for _,c:=range e.track().Clips {if e.group[c.ID] {ids=append(ids,c.ID)}}
		return ids
	}
	if c,ok:=e.selected();ok {return []string{c.ID}}
	return nil
}

func (e *editor) clipSelected(i int,c project.Clip) bool {
	if len(e.group)>1 {return e.group[c.ID]}
	return i==e.clip
}

func (e *editor) setSelection(ids []string,primary string) {
	wanted:=map[string]bool{}
	for _,id:=range ids {wanted[id]=true}
	e.group=nil
	e.clip=-1
	found:=map[string]bool{}
	for i,c:=range e.track().Clips {
		if !wanted[c.ID] {continue}
		found[c.ID]=true
		if e.clip<0 || c.ID==primary {e.clip=i}
	}
	if len(found)>1 {e.group=found}
	e.syncSelectionUI()
}

func (e *editor) syncSelectionUI() {
	send.Call(e.controls[idClips],lbSet,uintptr(e.clip),0)
	e.fields()
	e.invalidateTimeline()
}

func (e *editor) normalizeGroup() {
	if len(e.group)<=1 {e.group=nil;return}
	ids:=e.selectionIDs()
	primary:=""
	if c,ok:=e.selected();ok {primary=c.ID}
	e.setSelection(ids,primary)
}

func (e *editor) selectionKey() string {
	return strings.Join(e.selectionIDs(),"\x00")
}

func (e *editor) selectAllClips() {
	e.stopPreview()
	ids:=make([]string,0,len(e.track().Clips))
	for _,c:=range e.track().Clips {ids=append(ids,c.ID)}
	e.setSelection(ids,"")
	e.anchor=""
	e.checkpoint()
	e.tell(fmt.Sprintf("%d clip(s) selected.",len(ids)))
}

func (e *editor) modifySelection(i int,ctrl,shift bool) {
	c:=e.track().Clips[i]
	e.stopPreview()
	if shift {
		anchor:=e.anchor
		if anchor=="" {if old,ok:=e.selected();ok {anchor=old.ID}}
		e.setSelection(project.RangeClipIDs(e.track().Clips,anchor,c.ID),c.ID)
		if e.anchor=="" {e.anchor=anchor}
		if e.anchor=="" {e.anchor=c.ID}
	} else if ctrl {
		ids:=e.selectionIDs()
		found:=false
		next:=make([]string,0,len(ids)+1)
		for _,id:=range ids {if id==c.ID {found=true} else {next=append(next,id)}}
		if !found {next=append(next,c.ID)}
		e.setSelection(next,c.ID)
		e.anchor=c.ID
	}
	e.checkpoint()
	e.tell(fmt.Sprintf("%d clip(s) selected.",len(e.selectionIDs())))
}

func (e *editor) beginMarquee(x,y int32,t int64,playing bool) {
	e.marquee=&marqueeDrag{x:x,y:y,currentX:x,currentY:y,before:e.selectionIDs(),primary:e.clip,additive:keyPressed(0x11),clickTime:t,wasPlaying:playing}
	user.NewProc("SetCapture").Call(e.timeline)
}

func (e *editor) moveMarquee(x,y,w int32) {
	d:=e.marquee
	if d==nil {return}
	if !d.moved && math.Abs(float64(x-d.x))<4 && math.Abs(float64(y-d.y))<4 {return}
	if !d.moved {e.stopPreview()}
	d.moved=true
	d.currentX,d.currentY=x,y
	left,right:=min(d.x,x),max(d.x,x)
	top,bottom:=min(d.y,y),max(d.y,y)
	ids:=make([]string,0)
	if d.additive {ids=append(ids,d.before...)}
	seen:=map[string]bool{}
	for _,id:=range ids {seen[id]=true}
	for _,c:=range e.track().Clips {
		l,r:=e.xAt(c.Start,w),e.xAt(c.Start+(c.Out-c.In),w)
		if l>right || r<=left {continue}
		a,_:=e.p.Asset(c.AssetID)
		video:=top<laneBottom && bottom>=laneTop
		audio:=!e.audioTrack() && a.HasAudio && top<140 && bottom>=104
		if (video || audio) && !seen[c.ID] {ids=append(ids,c.ID);seen[c.ID]=true}
	}
	e.setSelection(ids,"")
}

func (e *editor) endMarquee(cancel bool) {
	d:=e.marquee
	if d==nil {return}
	e.marquee=nil
	user.NewProc("ReleaseCapture").Call()
	if cancel {
		primary:=""
		if d.primary>=0 && d.primary<len(e.track().Clips) {primary=e.track().Clips[d.primary].ID}
		e.setSelection(d.before,primary)
	} else if !d.moved {
		if !d.additive {e.selectClip(-1);e.seekTimeline(d.clickTime,d.wasPlaying)}
	} else {
		if c,ok:=e.selected();ok {e.anchor=c.ID} else {e.anchor=""}
		e.checkpoint()
		e.tell(fmt.Sprintf("%d clip(s) selected.",len(e.selectionIDs())))
	}
	e.invalidateTimeline()
}

func (e *editor) paintMarquee(dc uintptr,w,h int32) {
	d:=e.marquee
	if d==nil || !d.moved {return}
	l,r:=max(trackHeader,min(d.x,d.currentX)),min(w,max(d.x,d.currentX))
	t,b:=max(int32(33),min(d.y,d.currentY)),min(h,max(d.y,d.currentY))
	if r<=l || b<=t {return}
	for x:=l;x<r;x+=6 {
		fill(dc,rect{x,t,min(x+3,r),t+1},palette.accent)
		fill(dc,rect{x,b-1,min(x+3,r),b},palette.accent)
	}
	for y:=t;y<b;y+=6 {
		fill(dc,rect{l,y,l+1,min(y+3,b)},palette.accent)
		fill(dc,rect{r-1,y,r,min(y+3,b)},palette.accent)
	}
}

func (e *editor) deleteSelection() error {
	ids:=e.selectionIDs()
	q,err:=e.p.DeleteClips(e.trackID(),ids)
	if err!=nil {return err}
	e.stopPreview()
	e.group=nil;e.anchor="";e.clip=-1
	end:=q.EndTime()
	e.playhead=min(e.playhead,end)
	if err=e.commit(q);err!=nil {return err}
	e.tell(fmt.Sprintf("Removed %d clip(s); sources unchanged.",len(ids)))
	return nil
}
