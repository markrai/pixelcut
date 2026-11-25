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

const shortcutsText = "Space          Play / pause\nEsc              Cancel drag / pause\nCtrl+N           New project\nCtrl+O           Open project\nCtrl+S           Save project\nCtrl+Shift+S     Save As (new file)\nCtrl+W           Close project\nCtrl+,           Application settings\nCtrl+I           Import video\nCtrl+E           Export selected clip\nCtrl+Z           Undo\nCtrl+Y / Ctrl+Shift+Z   Redo\nCtrl+K / S       Split at marker\nCtrl+A           Select all timeline clips\nCtrl+C           Copy selected clips\nCtrl+X           Cut selected clips\nCtrl+V           Paste at playhead\nCtrl+D           Duplicate selected clips\nCtrl+click       Toggle clip selection\nShift+click      Select clip range\nDrag empty lane  Marquee selection\nDelete           Remove selected clips / media asset\nShift+Delete     Ripple delete selected clips\nG                Remove gap at playhead\nR                Toggle ripple trimming\nN                Toggle snapping\nShift+drag       Temporarily bypass snapping\nLeft / Right     Seek by 0.1s\n, / .            Previous / next decoded frame\nCtrl+Left/Right  Previous / next edit boundary\nI / O            Set source In / Out at playhead\n[ / ]            Slower / faster preview\nB                Toggle source-frame trim snapping\nHome / End       Project start / end\n+ / -            Zoom timeline\nF                Fit timeline\nTab / Shift+Tab  Focus navigation\nEnter            Apply clip properties\nMenu / Shift+F10 Context menu\nF1               Show shortcuts\n\nText fields retain their normal text-editing shortcuts. Click the timeline for editing shortcuts when the embedded player has focus."

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
			if shift {return idSaveAs};return idSave
		}
		if shift {return keyNone}
		switch key {
		case 'A':if !assetFocus {return idSelectAll}
		case 'C':if !assetFocus {return idCopy}
		case 'X':if !assetFocus {return idCut}
		case 'V':if !assetFocus {return idPaste}
		case 'D':if !assetFocus {return idDuplicate}
		case 0xbc:return idPreferences
		case 0x25:if !assetFocus {return idPrevEdit}
		case 0x27:if !assetFocus {return idNextEdit}
		case 'N':return idNew
		case 'O':return idOpen
		case 'W':return idCloseProject
		case 'I':return idImport
		case 'E':return idExport
		case 'Y':return idRedo
		case 'K':if !assetFocus {return idSplit}
		}
		return keyNone
	}
	if key==0xbb || key==0x6b {return idZoomIn}
	if key==0xbd || key==0x6d {return idZoomOut}
	if shift {
		if key==0x2e && !assetFocus {return idRippleDelete}
		return keyNone
	}
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
	case 'G':return idRemoveGap
	case 'R':return idRippleTrim
	case 'B':return idFrameSnap
	case 'I':return idSetIn
	case 'O':return idSetOut
	case 0xbc:return idFrameBack
	case 0xbe:return idFrameForward
	case 0xdb:return idSpeedDown
	case 0xdd:return idSpeedUp
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
	e.updatePlaybackClock()
	index,at,ok:=project.PlaybackTarget(e.track().Clips,e.playhead)
	if !ok {
		marker:=e.playhead
		e.stopPreview()
		e.playhead=marker
		e.refreshPlaybackMarker()
		e.checkpoint()
		e.tell("No footage ahead of the playhead.")
		return nil
	}
	c:=e.track().Clips[index]
	if a,ok:=e.p.Asset(c.AssetID);!ok || !project.SourceAvailable(a.Path) {
		e.stopPreview()
		e.playhead=at
		e.selectClip(index)
		e.refreshPlaybackMarker()
		e.checkpoint()
		e.tell("Source offline; use Relink.")
		return nil
	}
	if e.clock!=nil && e.previewActive() && e.clock.clip.ID==c.ID && at==e.playhead {
		if e.clip!=index || len(e.group)>1 {e.selectClip(index)}
		e.clock.queue(at,false,false)
		e.updatePlayButton(false)
		e.tell("Preview playing.")
		return nil
	}
	e.seekTimeline(at,true)
	return nil
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
	if e.menuOpen {return false}
	if (m.Window==e.timeline || m.Window==e.controls[idAssets]) && (m.WParam==0x5d || (m.WParam==0x79 && keyPressed(0x10))) {
		if m.Window==e.timeline {e.timelineContext(^uintptr(0))} else {e.mediaContext(m.Window,^uintptr(0))}
		return true
	}
	if e.assetDrag!=nil {
		if m.WParam==0x1b {e.cancelAssetDrag()}
		return true
	}
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
	if (e.drag!=nil || e.scrubbing || e.marquee!=nil) && action!=keyEscape {return true}
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
		if e.marquee!=nil {e.endMarquee(true)} else if e.scrubbing {e.scrubbing=false;user.NewProc("ReleaseCapture").Call();e.pausePreview()} else if e.drag!=nil {e.cancelTimelineDrag();e.tell("Timeline edit canceled.")} else if e.previewActive() {e.pausePreview()}
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
