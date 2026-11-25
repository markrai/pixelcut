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

const shortcutsText = "Space          Play / pause\nEsc              Cancel drag / pause\nCtrl+N           New project\nCtrl+O           Open project\nCtrl+S           Save As (new file)\nCtrl+I           Import video\nCtrl+E           Export selected clip\nCtrl+Z           Undo\nCtrl+Y / Ctrl+Shift+Z   Redo\nCtrl+K / S       Split at marker\nDelete           Remove selected clip / media asset\nN                Toggle snapping\nShift+drag       Temporarily bypass snapping\nLeft / Right     Seek by 0.1s\nHome / End       Project start / end\n+ / -            Zoom timeline\nF                Fit timeline\nTab / Shift+Tab  Focus navigation\nEnter            Apply clip properties\nF1               Show shortcuts\n\nText fields retain their normal text-editing shortcuts. Click the timeline for editing shortcuts when the embedded player has focus."

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
	if assetFocus {
		if key==0x2e {return idRemoveAsset}
		return keyNone
	}
	switch key {
	case 'N':return idSnap
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
	if e.playing() {return e.pausePreview()}
	if e.clock!=nil && e.previewActive() {
		c:=e.clock.clip
		if e.playhead>=c.Start+(c.Out-c.In) {
			e.seekTimeline(c.Start,true)
		} else {
			e.clock.queue(e.playhead,false,false)
			e.updatePlayButton(false)
			e.tell("Preview playing.")
		}
		return nil
	}
	return e.play()
}

func (e *editor) setMarker(t int64) {
	e.seekTimeline(t,e.playing())
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
	if e.transformSession!=nil {
		if m.WParam==0x1b {e.cancelTransform();return true}
		if m.WParam==0x0d {if err:=e.applyTransform();err!=nil {e.tell(err.Error())};return true}
		if m.WParam==0x09 {
			prev:=uintptr(0)
			if keyPressed(0x10) {prev=1}
			h,_,_:=user.NewProc("GetNextDlgTabItem").Call(mainWindow,m.Window,prev)
			if h!=0 {user.NewProc("SetFocus").Call(h)}
			return true
		}
		return false
	}
	editing:=false
	for _,id:=range []int{idStart,idIn,idOut,idCrop,idAt} {
		if m.Window==e.controls[id] {editing=true;break}
	}
	action:=shortcutFor(m.WParam,keyPressed(0x11),keyPressed(0x10),keyPressed(0x12),editing,m.Window==e.controls[idAssets])
	if action==keyNone {return false}
	if e.busy {return true}
	if e.cropSession!=nil {
		if action==keyEscape || action==keyFocusTimeline {e.cancelVisualCrop()}
		if m.WParam==0x0d && !keyPressed(0x11) && !keyPressed(0x12) {if err:=e.applyVisualCrop();err!=nil {e.tell(err.Error())}}
		return true
	}
	if m.LParam&(1<<30)!=0 && action!=keyMarkerBack && action!=keyMarkerForward {return true}
	if (e.drag!=nil || e.scrubbing) && action!=keyEscape {return true}
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
		if e.scrubbing {e.scrubbing=false;user.NewProc("ReleaseCapture").Call();e.pausePreview()} else if e.drag!=nil {e.cancelTimelineDrag();e.tell("Timeline edit canceled.")} else if e.previewActive() {e.pausePreview()}
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
