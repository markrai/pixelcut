package main

import (
	"math"
	"unsafe"

	"github.com/markrai/pixelcut/internal/project"
)

const (
	keyNone = 0
	keyEscape = 1000+iota
	keyNextFocus
	keyPrevFocus
	keyMarkerBack
	keyMarkerForward
	keyMarkerStart
	keyMarkerEnd
	keyFocusTimeline
)

const shortcutsText = "Space          Play / stop\nEsc              Cancel drag / stop\nCtrl+N           New project\nCtrl+O           Open project\nCtrl+S           Save As (new file)\nCtrl+I           Import video\nCtrl+E           Export selected clip\nCtrl+Z           Undo\nCtrl+Y / Ctrl+Shift+Z   Redo\nCtrl+K / S       Split at marker\nDelete           Remove selected timeline clip\nLeft / Right     Move marker by 0.1s\nHome / End       Project start / end\n+ / -            Zoom timeline\nF                Fit timeline\nTab / Shift+Tab  Focus navigation\nEnter            Apply clip properties\nF1               Show shortcuts\n\nText fields retain their normal text-editing shortcuts. Click the timeline for editing shortcuts when the embedded player has focus."

func (e *editor) showShortcuts() {
	user.NewProc("MessageBoxW").Call(mainWindow,uintptr(unsafe.Pointer(u(shortcutsText))),uintptr(unsafe.Pointer(u("PixelCut keyboard shortcuts"))),0x40)
}

func shortcutFor(key uintptr,ctrl,shift,alt,editing,assetFocus bool) int {
	if alt {return keyNone}
	if key==0x70 && !ctrl && !shift {return idKeys}
	if key==0x09 && !ctrl {
		if shift {return keyPrevFocus}
		return keyNextFocus
	}
	if editing {
		if !ctrl && !shift && key==0x0d {return idApply}
		if !ctrl && !shift && key==0x1b {return keyFocusTimeline}
		return keyNone
	}
	if ctrl {
		switch key {
		case 'Z':
			if shift {return idRedo};return idUndo
		case 'S':
			return idSave
		}
		if shift {return keyNone}
		switch key {
		case 'N':return idNew
		case 'O':return idOpen
		case 'I':return idImport
		case 'E':return idExport
		case 'Y':return idRedo
		case 'K':if !assetFocus {return idSplit}
		}
		return keyNone
	}
	if key==0xbb || key==0x6b {return idZoomIn}
	if key==0xbd || key==0x6d {return idZoomOut}
	if shift {return keyNone}
	switch key {
	case 0x20:return idPlay
	case 0x1b:return keyEscape
	case 'F':return idFit
	}
	if assetFocus {return keyNone}
	switch key {
	case 0x2e:return idDelete
	case 'S':return idSplit
	case 0x25:return keyMarkerBack
	case 0x27:return keyMarkerForward
	case 0x24:return keyMarkerStart
	case 0x23:return keyMarkerEnd
	}
	return keyNone
}

func keyPressed(key uintptr) bool {
	r,_,_:=user.NewProc("GetKeyState").Call(key)
	return r&0x8000!=0
}

func (e *editor) previewActive() bool {
	if e.previewDone==nil {return false}
	select {
	case <-e.previewDone:return false
	default:return true
	}
}

func (e *editor) togglePreview() error {
	if e.previewActive() {
		e.stopPreview()
		e.tell("Preview stopped.")
		return nil
	}
	return e.play()
}

func (e *editor) setMarker(t int64) {
	e.stopPreview()
	end:=e.endTime()
	if t<0 {t=0}
	if t>end {t=end}
	e.playhead=t
	for i,c:=range e.track().Clips {
		if t>=c.Start && t<c.Start+(c.Out-c.In) {
			if e.clip!=i {e.selectClip(i)}
			break
		}
	}
	change(e.controls[idAt],fmtTime(t))
	w:=clientRect(e.timeline).Right
	if t<e.origin {e.origin=t}
	if e.xAt(t,w)>=w {
		visible:=math.Max(0,float64(w-trackHeader-15))/e.scale(w)*float64(project.TicksPerSecond)
		if visible<float64(t) {e.origin=t-int64(visible)} else {e.origin=0}
	}
	e.invalidateTimeline()
}

func (e *editor) cancelTimelineDrag() {
	e.drag=nil
	user.NewProc("ReleaseCapture").Call()
	e.invalidateTimeline()
}

func (e *editor) handleShortcut(m message) bool {
	if m.Message!=0x0100 {return false}
	if m.Window!=mainWindow {
		child,_,_:=user.NewProc("IsChild").Call(mainWindow,m.Window)
		if child==0 {return false}
	}
	editing:=false
	for _,id:=range []int{idStart,idIn,idOut,idCrop,idAt} {
		if m.Window==e.controls[id] {editing=true;break}
	}
	action:=shortcutFor(m.WParam,keyPressed(0x11),keyPressed(0x10),keyPressed(0x12),editing,m.Window==e.controls[idAssets])
	if action==keyNone {return false}
	if e.busy {return true}
	if m.LParam&(1<<30)!=0 && action!=keyMarkerBack && action!=keyMarkerForward {return true}
	if e.drag!=nil && action!=keyEscape {return true}
	switch action {
	case keyNextFocus,keyPrevFocus:
		prev:=uintptr(0)
		if action==keyPrevFocus {prev=1}
		current:=m.Window
		if current==mainWindow {current=0}
		h,_,_:=user.NewProc("GetNextDlgTabItem").Call(mainWindow,current,prev)
		if h!=0 {user.NewProc("SetFocus").Call(h)}
	case keyFocusTimeline:
		user.NewProc("SetFocus").Call(e.timeline)
	case keyEscape:
		if e.drag!=nil {e.cancelTimelineDrag();e.tell("Timeline edit canceled.")} else if e.previewActive() {e.stopPreview();e.tell("Preview stopped.")}
	case keyMarkerBack:
		e.setMarker(e.playhead-project.TicksPerSecond/10)
	case keyMarkerForward:
		t:=e.playhead
		if t>math.MaxInt64-project.TicksPerSecond/10 {t=math.MaxInt64} else {t+=project.TicksPerSecond/10}
		e.setMarker(t)
	case keyMarkerStart:
		e.setMarker(0)
	case keyMarkerEnd:
		e.setMarker(e.endTime())
	default:
		if action==idSplit {change(e.controls[idAt],fmtTime(e.playhead))}
		if err:=e.command(action,0);err!=nil {e.tell(err.Error())}
	}
	return true
}
