package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/session"
)

var (
	user = syscall.NewLazyDLL("user32.dll")
	kernel = syscall.NewLazyDLL("kernel32.dll")
	gdi = syscall.NewLazyDLL("gdi32.dll")
	dialog = syscall.NewLazyDLL("comdlg32.dll")
	createWindow = user.NewProc("CreateWindowExW")
	defWindow = user.NewProc("DefWindowProcW")
	send = user.NewProc("SendMessageW")
	post = user.NewProc("PostMessageW")
	setText = user.NewProc("SetWindowTextW")
	getText = user.NewProc("GetWindowTextW")
	getLength = user.NewProc("GetWindowTextLengthW")
	destroy = user.NewProc("DestroyWindow")
	instance uintptr
	mainWindow uintptr
	font uintptr
	ui *editor
)

const (
	wmCommand = 0x0111
	wmDone = 0x8001
	wmPreviewDone = 0x8002
	wsChild = 0x40000000
	wsVisible = 0x10000000
	wsBorder = 0x00800000
	wsTab = 0x00010000
	lbAdd = 0x0180
	lbReset = 0x0184
	lbGet = 0x0188
	lbSet = 0x0186
	idNew = 101 + iota
	idOpen
	idSave
	idImport
	idAssets
	idAdd
	idClips
	idApply
	idSplit
	idDelete
	idUndo
	idRedo
	idPlay
	idStop
	idExport
	idStart
	idIn
	idOut
	idCrop
	idAt
	idZoomOut
	idZoomIn
	idFit
	idKeys
	idCropVisual
	idCropReset
	idTransform
	idCanvasW
	idCanvasH
	idScale
	idPositionX
	idPositionY
	idTransformApply
	idTransformReset
	idTransformCancel
	idRotateLeft
	idRotateRight
	idFlipH
	idFlipV
	idRemoveAsset
	idSnap
	idSelectAll
	idRippleDelete
	idRemoveGap
	idRippleTrim
	idCopy
	idCut
	idPaste
	idDuplicate
)

type windowClass struct {
	Size, Style uint32
	Proc uintptr
	ClassExtra, WindowExtra int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name *uint16
	IconSmall uintptr
}

type message struct {
	Window uintptr
	Message uint32
	WParam, LParam uintptr
	Time uint32
	X, Y int32
	Private uint32
}

type openFileName struct {
	Size uint32
	Owner, Instance uintptr
	Filter, CustomFilter *uint16
	MaxCustomFilter, FilterIndex uint32
	File *uint16
	MaxFile uint32
	FileTitle *uint16
	MaxFileTitle uint32
	InitialDir, Title *uint16
	Flags uint32
	FileOffset, FileExtension uint16
	DefaultExt *uint16
	CustomData, Hook uintptr
	TemplateName *uint16
	Reserved uintptr
	ReservedSize, FlagsEx uint32
}

type result struct {
	batch *project.ImportBatch
	placement *placementRequest
	err error
	message string
}

type previewResult struct {
	generation uint64
	err error
}

type editor struct {
	p project.Project
	undo, redo []project.Project
	controls map[int]uintptr
	status, host uintptr
	asset, clip int
	busy bool
	cancel context.CancelFunc
	results chan result
	previewCancel context.CancelFunc
	previewDone chan struct{}
	previewResults chan previewResult
	generation uint64
	dirty bool
	engine media.Engine
	timeline uintptr
	layout []placedControl
	zoom float64
	origin, playhead int64
	drag *timelineDrag
	clock *previewClock
	scrubbing, scrubPlaying bool
	cropSession *cropSession
	transformSession *transformSession
	transformControls []uintptr
	inspectorControls []uintptr
	thumbnails *thumbnailCache
	thumbnailRevision uint64
	waveforms *waveformCache
	waveformRevision uint64
	recovery *session.Store
	recoveryLock syscall.Handle
	recoveryError string
	lastCheckpoint time.Time
	savedView recoveryView
	projectPath string
	snapDisabled bool
	assetDrag *assetDrag
	group map[string]bool
	anchor string
	marquee *marqueeDrag
	rippleTrim bool
	clipboard project.ClipClipboard
}

func u(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func control(class, text string, style uintptr, x, y, w, h, id int) uintptr {
	hwnd, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(u(class))), uintptr(unsafe.Pointer(u(text))), wsChild|wsVisible|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), mainWindow, uintptr(id), instance, 0)
	if hwnd == 0 {
		panic("failed to create " + class)
	}
	send.Call(hwnd, 0x0030, font, 1)
	if id != 0 {
		ui.controls[id] = hwnd
	}
	ui.layout = append(ui.layout, placedControl{hwnd,x,y,w,h,id})
	return hwnd
}

func label(text string, x, y, w int) uintptr {
	return control("STATIC", text, 0, x, y, w, 22, 0)
}

func button(text string, x, y, w, id int) {
	control("BUTTON", text, wsTab|0x0000000b, x, y, w, 28, id)
}

func edit(text string, x, y, w, id int) {
	control("EDIT", text, wsBorder|wsTab|0x80, x, y, w, 25, id)
}

func change(hwnd uintptr, text string) {
	setText.Call(hwnd, uintptr(unsafe.Pointer(u(text))))
}

func read(hwnd uintptr) string {
	n, _, _ := getLength.Call(hwnd)
	buf := make([]uint16, int(n)+1)
	getText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func pick(save bool, filter, ext, title string) (string, error) {
	buf := make([]uint16, 32768)
	f := utf16.Encode([]rune(filter + "\x00"))
	of := openFileName{Owner: mainWindow, Filter: &f[0], FilterIndex: 1, File: &buf[0], MaxFile: uint32(len(buf)), Title: u(title), DefaultExt: u(ext), Flags: 0x00080000|0x00000800|0x00000008|0x00000004}
	proc := "GetOpenFileNameW"
	if save {
		proc = "GetSaveFileNameW"
		of.Flags |= 0x00000800
	} else {
		of.Flags |= 0x00001000
	}
	of.Size = uint32(unsafe.Sizeof(of))
	ok, _, _ := dialog.NewProc(proc).Call(uintptr(unsafe.Pointer(&of)))
	runtime.KeepAlive(f)
	runtime.KeepAlive(buf)
	if ok == 0 {
		code, _, _ := dialog.NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return "", fmt.Errorf("file dialog error 0x%x", code)
		}
		return "", nil
	}
	return syscall.UTF16ToString(buf), nil
}

func (e *editor) tell(s string) { change(e.status, s) }

func (e *editor) confirmDiscard() bool {
	if !e.dirty {
		return true
	}
	r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u("Discard unsaved project changes?"))), uintptr(unsafe.Pointer(u("PixelCut"))), 0x34)
	return r == 6
}

func (e *editor) track() *project.Track { return &e.p.Tracks[0] }

func (e *editor) selected() (project.Clip, bool) {
	t := e.track()
	if e.clip < 0 || e.clip >= len(t.Clips) {
		return project.Clip{}, false
	}
	return t.Clips[e.clip], true
}

func fmtTime(t int64) string { return strconv.FormatFloat(project.Seconds(t), 'f', 6, 64) }

func listAdd(h uintptr, s string) { send.Call(h, lbAdd, 0, uintptr(unsafe.Pointer(u(s)))) }

func (e *editor) refresh() {
	a, c := e.controls[idAssets], e.controls[idClips]
	send.Call(a, lbReset, 0, 0)
	send.Call(c, lbReset, 0, 0)
	for _, item := range e.p.Assets {
		listAdd(a, fmt.Sprintf("%s  [%dx%d]", filepath.Base(item.Path), item.Width, item.Height))
	}
	for _, item := range e.track().Clips {
		asset, _ := e.p.Asset(item.AssetID)
		listAdd(c, fmt.Sprintf("%s  |  %ss - %ss", filepath.Base(asset.Path), fmtTime(item.Start), fmtTime(item.Start+(item.Out-item.In))))
	}
	if e.asset >= len(e.p.Assets) { e.asset = -1 }
	if e.clip >= len(e.track().Clips) { e.clip = -1 }
	e.normalizeGroup()
	send.Call(a, lbSet, uintptr(e.asset), 0)
	send.Call(c, lbSet, uintptr(e.clip), 0)
	e.fields()
	e.invalidateTimeline()
	e.checkpoint()
}

func (e *editor) fields() {
	c, ok := e.selected()
	for _, id := range []int{idStart,idIn,idOut,idCrop,idAt} {
		change(e.controls[id], "")
	}
	if !ok || len(e.group)>1 { return }
	change(e.controls[idStart], fmtTime(c.Start))
	change(e.controls[idIn], fmtTime(c.In))
	change(e.controls[idOut], fmtTime(c.Out))
	change(e.controls[idAt], fmtTime(e.playhead))
	if c.Crop != nil {
		change(e.controls[idCrop], fmt.Sprintf("%d,%d,%d,%d", c.Crop.X,c.Crop.Y,c.Crop.Width,c.Crop.Height))
	}
}

func (e *editor) commit(q project.Project) error {
	if err := q.Validate(); err != nil { return err }
	e.stopPreview()
	e.undo = append(e.undo, e.p.Clone())
	if len(e.undo) > 50 { e.undo = e.undo[1:] }
	e.redo = nil
	e.p = q
	e.dirty = true
	e.refresh()
	return nil
}

func nextID(prefix string) string { return fmt.Sprintf("%s-%d", prefix,time.Now().UnixNano()) }

func (e *editor) startJob(fn func(context.Context) result) {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.busy = true
	for _, h := range e.controls { user.NewProc("EnableWindow").Call(h, 0) }
	go func() {
		e.results <- fn(ctx)
		post.Call(mainWindow, wmDone, 0, 0)
	}()
}

func (e *editor) finished() {
	r := <-e.results
	e.cancel()
	e.cancel = nil
	e.busy = false
	for _, h := range e.controls { user.NewProc("EnableWindow").Call(h, 1) }
	if r.err != nil { e.tell(r.err.Error()); return }
	if r.batch!=nil {e.finishImport(r.batch,r.placement);return}
	e.tell(r.message)
}

func (e *editor) stopPreview() {
	if e.cropSession!=nil {e.endVisualCropUI()}
	if e.transformSession!=nil {e.endTransformUI()}
	e.updatePlaybackClock()
	e.clock=nil
	e.generation++
	if e.previewCancel != nil {
		e.previewCancel()
		<-e.previewDone
		e.previewCancel = nil
		e.previewDone = nil
	}
	if h:=e.controls[idPlay];h!=0 {change(h,"Play  (Space)");user.NewProc("InvalidateRect").Call(h,0,1)}
}

func (e *editor) play() error { return e.startPreview(false) }

func (e *editor) startPreview(paused bool) error {
	c, ok := e.selected()
	if !ok { return fmt.Errorf("select a timeline clip") }
	a, ed, err := e.p.MediaEdit(c)
	if err != nil { return err }
	e.stopPreview()
	ctx, cancel := context.WithCancel(context.Background())
	if e.playhead < c.Start || e.playhead >= c.Start+(c.Out-c.In) {
		e.playhead=c.Start
	}
	delta:=e.playhead-c.Start
	ed.Start=project.Seconds(c.In+delta)
	ed.Duration=project.Seconds(c.Out-c.In-delta)
	bindings,err:=os.CreateTemp("","pixelcut-input-*.conf")
	if err!=nil {cancel();return err}
	name:=bindings.Name()
	_,err=bindings.WriteString("SPACE cycle pause\nESC set pause yes\n")
	closeErr:=bindings.Close()
	if err==nil {err=closeErr}
	if err!=nil {os.Remove(name);cancel();return err}
	pipe:=previewPipeName(name)
	script,err:=writeCropScript()
	if err!=nil {os.Remove(name);cancel();return err}
	cmd, err := e.engine.EmbeddedPreview(ctx,a.Path,e.host,ed.Start,ed.Duration,false,name,pipe,paused,script)
	if err != nil {os.Remove(script);os.Remove(name); cancel(); return err }
	if err = cmd.Start(); err != nil {os.Remove(script);os.Remove(name); cancel(); return err }
	e.previewCancel = cancel
	done := make(chan struct{})
	e.previewDone = done
	generation := e.generation
	clock:=newPreviewClock(c,e.playhead,paused)
	clock.setComposition(a,c.Crop,c.Transform,false)
	e.clock=clock
	processDone:=make(chan struct{})
	clockDone:=make(chan struct{})
	go func(){defer close(clockDone);clock.observe(ctx,pipe,processDone)}()
	go func() {
		err := cmd.Wait()
		close(processDone)
		select {case <-clockDone:case <-time.After(150*time.Millisecond):}
		os.Remove(name)
		os.Remove(script)
		close(done)
		if ctx.Err() == nil {
			select {
			case e.previewResults <- previewResult{generation,err}:
				post.Call(mainWindow,wmPreviewDone,0,0)
			default:
			}
		}
	}()
	e.updatePlayButton(paused)
	user.NewProc("SetFocus").Call(e.timeline)
	e.refreshPlaybackMarker()
	if paused {e.tell("Preview paused.")} else {e.tell("Playing source interval. Space or Esc pauses.")}
	return nil
}

func (e *editor) command(id, notification int) error {
	if e.busy { return fmt.Errorf("media operation in progress") }
	if e.assetDrag!=nil || e.marquee!=nil || e.drag!=nil || e.scrubbing {return fmt.Errorf("finish the active drag first")}
	if len(e.group)>1 {
		switch id {case idApply,idSplit,idCropVisual,idCropReset,idTransform,idExport:return fmt.Errorf("select a single clip for this operation")}
	}
	if e.transformSession!=nil && id!=idTransformApply && id!=idTransformCancel && id!=idTransformReset && id!=idRotateLeft && id!=idRotateRight && id!=idFlipH && id!=idFlipV {return fmt.Errorf("Apply or cancel the active transform first")}
	if e.cropSession!=nil && id!=idCropVisual && id!=idCropReset && id!=idKeys {return fmt.Errorf("Finish or cancel the active crop first")}
	switch id {
	case idNew,idOpen:
		if !e.confirmDiscard() { return nil }
		q := project.New()
		projectPath := ""
		if id == idOpen {
			path, err := pick(false,"PixelCut project (*.pixelcut.json)\x00*.pixelcut.json\x00","json","Open project")
			if err != nil || path == "" { return err }
			q, err = project.Load(path)
			if err != nil { return err }
			projectPath=path
			if len(q.Tracks)!=2 || q.Tracks[0].ID!="V1" || q.Tracks[0].Kind!="video" || q.Tracks[1].ID!="A1" || q.Tracks[1].Kind!="audio" || len(q.Tracks[1].Clips)!=0 {
				return fmt.Errorf("this prototype only opens V1 projects with an empty A1 track")
			}
		}
		q,merged,err:=q.MergeDuplicateAssets()
		if err!=nil {return err}
		e.stopPreview()
		e.group=nil;e.anchor=""
		e.clipboard=project.ClipClipboard{}
		e.p, e.undo, e.redo, e.asset, e.clip, e.dirty = q,nil,nil,-1,-1,len(merged)>0
		e.origin,e.playhead,e.zoom=0,0,0
		e.projectPath=projectPath
		e.refresh()
		if len(merged)>0 {e.tell(fmt.Sprintf("Project ready. Merged %d duplicate asset(s); all clips retained.",len(merged)))} else {e.tell("Project ready.")}
	case idSave:
		path, err := pick(true,"PixelCut project (*.pixelcut.json)\x00*.pixelcut.json\x00","json","Save project as")
		if err != nil || path == "" { return err }
		if !strings.HasSuffix(strings.ToLower(path),".pixelcut.json") {
			path = strings.TrimSuffix(path,filepath.Ext(path))+".pixelcut.json"
		}
		if err = e.p.SaveAs(path); err != nil { return err }
		e.dirty = false
		e.projectPath=path
		e.checkpoint()
		e.tell("Project saved: "+path)
	case idImport:
		paths,err:=pickImportFiles()
		if err!=nil || len(paths)==0 {return err}
		return e.importPaths(paths)
	case idAssets:
		if notification == 1 {
			n,_,_ := send.Call(e.controls[idAssets],lbGet,0,0)
			e.asset = int(n)
		}
	case idClips:
		if notification == 1 {
			n,_,_ := send.Call(e.controls[idClips],lbGet,0,0)
			e.group=nil;e.anchor=""
			e.clip = int(n)
			e.stopPreview()
			e.fields()
			e.invalidateTimeline()
		}
	case idAdd:
		if e.asset < 0 || e.asset >= len(e.p.Assets) { return fmt.Errorf("select a media asset") }
		a := e.p.Assets[e.asset]
		q := e.p.Clone()
		var end int64
		for _, c := range q.Tracks[0].Clips {
			if n := c.Start+(c.Out-c.In); n>end { end=n }
		}
		q.Tracks[0].Clips = append(q.Tracks[0].Clips,project.Clip{ID:nextID("clip"),AssetID:a.ID,Start:end,Out:a.Duration})
		e.group=nil;e.anchor=""
		e.clip = len(q.Tracks[0].Clips)-1
		if err := e.commit(q); err != nil { return err }
		e.tell("Clip added.")
	case idApply:
		c, ok := e.selected()
		if !ok { return fmt.Errorf("select a clip") }
		var err error
		if c.Start,err=project.ParseSeconds(read(e.controls[idStart])); err!=nil { return err }
		if c.In,err=project.ParseSeconds(read(e.controls[idIn])); err!=nil { return err }
		if c.Out,err=project.ParseSeconds(read(e.controls[idOut])); err!=nil { return err }
		c.Crop=nil
		if s:=strings.TrimSpace(read(e.controls[idCrop])); s!="" {
			parts:=strings.Split(s,",")
			if len(parts)!=4 { return fmt.Errorf("crop needs x,y,width,height") }
			var values [4]int
			for i,p:=range parts {
				values[i],err=strconv.Atoi(strings.TrimSpace(p))
				if err!=nil { return fmt.Errorf("invalid crop coordinate") }
			}
			c.Crop=&project.Crop{X:values[0],Y:values[1],Width:values[2],Height:values[3]}
		}
		q:=e.p.Clone()
		q.Tracks[0].Clips[e.clip]=c
		if err=e.commit(q); err!=nil { return err }
		if err:=e.startPreview(true);err!=nil {e.tell("Edits applied; preview unavailable: "+err.Error())} else {e.tell("Clip properties applied.")}
	case idSplit:
		c,ok:=e.selected()
		if !ok { return fmt.Errorf("select a clip") }
		at,err:=project.ParseSeconds(read(e.controls[idAt]))
		if err!=nil { return err }
		q,err:=e.p.Split("V1",c.ID,nextID("clip"),at)
		if err!=nil { return err }
		if err=e.commit(q);err!=nil { return err }
		e.tell("Clip split at timeline position.")
	case idRemoveAsset:
		return e.removeAsset()
	case idSnap:
		e.snapDisabled=!e.snapDisabled
		e.updateSnapButton()
		e.invalidateTimeline()
	case idSelectAll:
		e.selectAllClips()
	case idCopy:
		return e.copySelection(false)
	case idCut:
		return e.copySelection(true)
	case idPaste:
		return e.pasteSelection(false)
	case idDuplicate:
		return e.pasteSelection(true)
	case idDelete:
		return e.deleteSelection()
	case idRippleDelete:
		return e.rippleDeleteSelection()
	case idRemoveGap:
		return e.removeGap()
	case idRippleTrim:
		e.rippleTrim=!e.rippleTrim
		e.updateRippleButton()
	case idUndo,idRedo:
		from,to:=&e.undo,&e.redo
		if id==idRedo { from,to=to,from }
		if len(*from)==0 { return nil }
		e.stopPreview()
		*to=append(*to,e.p.Clone())
		ids:=e.selectionIDs()
		primary:=""
		if c,ok:=e.selected();ok {primary=c.ID}
		e.p=(*from)[len(*from)-1]
		e.setSelection(ids,primary)
		e.anchor=primary
		*from=(*from)[:len(*from)-1]
		e.dirty=true
		e.refresh()
		if _,ok:=e.selected();ok && len(e.group)<=1 {
			if err:=e.startPreview(true);err!=nil {e.tell("History restored; preview unavailable: "+err.Error());return nil}
		}
		e.tell("History restored.")
	case idZoomOut,idZoomIn,idFit:
		e.zoomTimeline(id)
	case idTransform:
		return e.beginTransform()
	case idTransformApply:
		return e.applyTransform()
	case idTransformReset:
		e.resetTransformDraft()
	case idTransformCancel:
		e.cancelTransform()
	case idRotateLeft,idRotateRight,idFlipH,idFlipV:
		return e.changeOrientation(id)
	case idCropVisual:
		return e.beginOrApplyCrop()
	case idCropReset:
		return e.resetVisualCrop()
	case idKeys:
		e.showShortcuts()
	case idPlay:
		return e.togglePreview()
	case idStop:
		return e.pausePreview()
	case idExport:
		c,ok:=e.selected()
		if !ok { return fmt.Errorf("select a clip") }
		a,ed,err:=e.p.MediaEdit(c)
		if err!=nil { return err }
		path,err:=pick(true,"MP4 video (*.mp4)\x00*.mp4\x00","mp4","Export selected clip")
		if err!=nil || path=="" { return err }
		e.stopPreview()
		e.tell("Exporting selected clip with source HDR...")
		e.startJob(func(ctx context.Context) result {
			var err error
			if !ed.Trim && ed.Crop==nil && ed.Transform==nil { err=e.engine.Remux(ctx,a.Path,path) } else { err=e.engine.Render(ctx,a.Path,path,ed) }
			return result{err:err,message:"Export complete: "+path}
		})
	}
	return nil
}

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0005:
		if ui != nil && ui.timeline != 0 { ui.resize() }
		return 0
	case 0x0024:
		if lp==0 { break }
		var m minMaxInfo
		nativeCopy(uintptr(unsafe.Pointer(&m)),lp,unsafe.Sizeof(m))
		m.MinTrack.X,m.MinTrack.Y=1080,710
		nativeCopy(lp,uintptr(unsafe.Pointer(&m)),unsafe.Sizeof(m))
		runtime.KeepAlive(&m)
		return 0
	case 0x002b:
		if drawButton(lp) { return 1 }
	case 0x0133,0x0134,0x0138:
		return colorControl(msg,wp,lp)
	case wmCommand:
		id,notification:=int(wp&0xffff),int((wp>>16)&0xffff)
		if id>=idNew && id<=idDuplicate && !(id>=idCanvasW && id<=idPositionY) && id!=idStart && id!=idIn && id!=idOut && id!=idCrop && id!=idAt {
			if err:=ui.command(id,notification);err!=nil { ui.tell(err.Error()) }
		}
		return 0
	case 0x0113:
		if wp==playbackTimer && ui!=nil {ui.updatePlaybackClock();ui.updateVisualCrop();ui.updateTransformDraft();ui.updateThumbnails();ui.updateWaveforms();ui.checkpointView();return 0}
	case wmDropFiles:
		ui.handleDrop(hwnd,wp)
		return 0
	case wmDone:
		ui.finished()
		return 0
	case wmPreviewDone:
		r:=<-ui.previewResults
		if r.generation==ui.generation {
			ui.stopPreview()
			fg,_,_:=user.NewProc("GetForegroundWindow").Call()
			focus,_,_:=user.NewProc("GetFocus").Call()
			if fg==mainWindow && (focus==0 || focus==ui.host) {user.NewProc("SetFocus").Call(ui.timeline)}
			if r.err!=nil { ui.tell("Preview failed: "+r.err.Error()) } else { ui.tell("Preview finished.") }
		}
		return 0
	case 0x0010:
		ui.cancelAssetDrag()
		ui.endMarquee(true)
		if ui.cropSession!=nil {ui.cancelVisualCrop()}
		if ui.transformSession!=nil {ui.cancelTransform()}
		if ui.busy { ui.tell("Wait for the current media operation to finish."); return 0 }
		if ui.checkpoint() || ui.warnRecoveryClose() { destroy.Call(hwnd) }
		return 0
	case 0x0002:
		user.NewProc("KillTimer").Call(hwnd,playbackTimer)
		ui.stopPreview()
		if ui.thumbnails!=nil {ui.thumbnails.cancel()}
		if ui.waveforms!=nil {ui.waveforms.cancel()}
		if ui.thumbnails!=nil {<-ui.thumbnails.done}
		if ui.waveforms!=nil {<-ui.waveforms.done}
		if ui.cancel!=nil { ui.cancel() }
		user.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r,_,_:=defWindow.Call(hwnd,uintptr(msg),wp,lp)
	return r
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance,_,_=kernel.NewProc("GetModuleHandleW").Call(0)
	initTheme()
	font = modernFont()
	cursor,_,_:=user.NewProc("LoadCursorW").Call(0,32512)
	className:=u("PixelCutEditor")
	wc:=windowClass{Size:uint32(unsafe.Sizeof(windowClass{})),Proc:syscall.NewCallback(wndProc),Instance:instance,Cursor:cursor,Background:theme.background,Name:className}
	if r,_,err:=user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)));r==0 {
		fmt.Fprintln(os.Stderr,err)
		os.Exit(1)
	}
	ui=&editor{p:project.New(),controls:map[int]uintptr{},asset:-1,clip:-1,results:make(chan result,1),previewResults:make(chan previewResult,16),engine:media.Default()}
	if err:=ui.openRecovery();err!=nil {
		fmt.Fprintln(os.Stderr,err)
		user.NewProc("MessageBoxW").Call(0,uintptr(unsafe.Pointer(u(err.Error()))),uintptr(unsafe.Pointer(u("PixelCut recovery"))),0x10)
		freeTheme()
		return
	}
	defer ui.closeRecovery()
	registerTimeline()
	mainWindow,_,_=createWindow.Call(0,uintptr(unsafe.Pointer(className)),uintptr(unsafe.Pointer(u("PixelCut - Editor preview"))),0x00cf0000,100,70,1200,790,0,0,instance,0)
	if mainWindow==0 { fmt.Fprintln(os.Stderr,"unable to create PixelCut window");os.Exit(1) }
	button("New",12,10,70,idNew)
	button("Open",88,10,70,idOpen)
	button("Save As",164,10,80,idSave)
	button("Undo",250,10,65,idUndo)
	button("Redo",321,10,65,idRedo)
	button("Shortcuts",392,10,110,idKeys)
	button("Export clip",970,10,126,idExport)
	applyDarkTitle(mainWindow)
	label("Media bin / Drop files",12,50,245)
	control("LISTBOX","",wsBorder|wsTab|0x00200000|1|0x100,12,75,245,270,idAssets)
	button("Import video",12,350,118,idImport)
	button("Add to V1",136,350,121,idAdd)
	button("Remove asset",12,384,245,idRemoveAsset)
	label("Source preview / SDR",270,50,570)
	ui.host=control("STATIC","",0x02000000|0x04000000|4,270,75,540,303,0)
	button("Play  (Space)",270,384,124,idPlay)
	button("Stop",400,384,80,idStop)
	label("Clip properties (seconds)",825,50,270)
	label("Timeline start",825,80,145)
	edit("",975,77,121,idStart)
	label("Source in",825,112,145)
	edit("",975,109,121,idIn)
	label("Source out",825,144,145)
	edit("",975,141,121,idOut)
	label("Crop: x,y,width,height (blank = full)",825,176,270)
	edit("",825,199,271,idCrop)
	button("Apply edits",825,234,130,idApply)
	button("Transform",961,234,135,idTransform)
	button("Visual crop",825,370,130,idCropVisual)
	button("Reset crop",961,370,135,idCropReset)
	ui.initTransformInspector()
	label("Split at timeline time",825,273,270)
	edit("",825,297,155,idAt)
	button("Split",986,296,110,idSplit)
	button("Remove clip",825,332,130,idDelete)
	label("TIMELINE  /  V1 + linked audio",12,423,310)
	button("Ripple delete",330,419,125,idRippleDelete)
	button("Remove gap",461,419,105,idRemoveGap)
	button("Ripple: off",572,419,120,idRippleTrim)
	button("-",832,419,48,idZoomOut)
	button("+",886,419,48,idZoomIn)
	button("Fit",940,419,70,idFit)
	button("Snap: on",1016,419,80,idSnap)
	control("LISTBOX","",wsBorder|wsTab|0x00200000|1|0x100,12,448,1084,180,idClips)
	user.NewProc("ShowWindow").Call(ui.controls[idClips],0)
	ui.timeline=control("PixelCutTimeline","",wsTab|0x04000000,12,452,1084,176,0)
	ui.status=label("Drop supported HLG videos here, or choose Import video.",12,640,1084)
	ui.thumbnails=newThumbnailCache(ui.engine)
	ui.waveforms=newWaveformCache(ui.engine)
	ui.enableFileDrops()
	ui.refresh()
	ui.resize()
	if timer,_,_:=user.NewProc("SetTimer").Call(mainWindow,playbackTimer,33,0);timer==0 {fmt.Fprintln(os.Stderr,"unable to start playback UI timer")}
	user.NewProc("ShowWindow").Call(mainWindow,5)
	user.NewProc("UpdateWindow").Call(mainWindow)
	ui.finishRecovery()
	var m message
	for {
		r,_,_:=user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)),0,0,0)
		if int32(r)<=0 { break }
		if ui.handleShortcut(m) {continue}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	freeTheme()
}

func (e *editor) removeAsset() error {
	if e.asset<0 || e.asset>=len(e.p.Assets) {return fmt.Errorf("select a media asset")}
	a:=e.p.Assets[e.asset]
	count:=e.p.AssetReferences(a.ID)
	if count>0 {
		message:=fmt.Sprintf("Remove %s and its %d timeline clip(s)? The source file will remain untouched.",filepath.Base(a.Path),count)
		r,_,_:=user.NewProc("MessageBoxW").Call(mainWindow,uintptr(unsafe.Pointer(u(message))),uintptr(unsafe.Pointer(u("Remove media asset"))),0x134)
		if r!=6 {return nil}
	}
	q,err:=e.p.RemoveAsset(a.ID,count>0)
	if err!=nil {return err}
	selectedID:=""
	if c,ok:=e.selected();ok {selectedID=c.ID}
	e.stopPreview()
	e.asset=min(e.asset,len(q.Assets)-1)
	e.clip=-1
	var end int64
	for i,c:=range q.Tracks[0].Clips {
		if c.ID==selectedID {e.clip=i}
		end=max(end,c.Start+(c.Out-c.In))
	}
	e.playhead=min(e.playhead,end)
	if err=e.commit(q);err!=nil {return err}
	e.tell("Asset removed from project; source file unchanged. Ctrl+Z to undo.")
	return nil
}

func (e *editor) updateSnapButton() {
	label:="Snap: on"
	if e.snapDisabled {label="Snap: off"}
	h:=e.controls[idSnap]
	change(h,label)
	user.NewProc("InvalidateRect").Call(h,0,1)
}

func (e *editor) selectExistingAsset(index int) {
	e.asset=index
	send.Call(e.controls[idAssets],lbSet,uintptr(index),0)
	e.checkpoint()
	e.tell("Already imported: "+filepath.Base(e.p.Assets[index].Path))
}
