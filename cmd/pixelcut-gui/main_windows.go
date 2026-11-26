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

	"github.com/markrai/pixelcut/internal/composition"
	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/service"
	"github.com/markrai/pixelcut/internal/session"
)

var (
	user         = syscall.NewLazyDLL("user32.dll")
	kernel       = syscall.NewLazyDLL("kernel32.dll")
	gdi          = syscall.NewLazyDLL("gdi32.dll")
	dialog       = syscall.NewLazyDLL("comdlg32.dll")
	createWindow = user.NewProc("CreateWindowExW")
	defWindow    = user.NewProc("DefWindowProcW")
	send         = user.NewProc("SendMessageW")
	post         = user.NewProc("PostMessageW")
	setText      = user.NewProc("SetWindowTextW")
	getText      = user.NewProc("GetWindowTextW")
	getLength    = user.NewProc("GetWindowTextLengthW")
	destroy      = user.NewProc("DestroyWindow")
	instance     uintptr
	mainWindow   uintptr
	font         uintptr
	ui           *editor
)

const (
	wmCommand     = 0x0111
	wmDone        = 0x8001
	wmPreviewDone = 0x8002
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsBorder      = 0x00800000
	wsTab         = 0x00010000
	lbAdd         = 0x0180
	lbReset       = 0x0184
	lbGet         = 0x0188
	lbSet         = 0x0186
	idNew         = 101 + iota
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
	idFrameBack
	idFrameForward
	idSetIn
	idSetOut
	idPrevEdit
	idNextEdit
	idSpeed
	idSpeedDown
	idSpeedUp
	idFrameSnap
	idSaveAs
	idRecent
	idCloseProject
	idMediaCache
	idRelinkMenu
	idRelinkAsset
	idReplaceAsset
	idRelinkFolder
	idPreferences
	idTracks
)

type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	IconSmall                          uintptr
}

type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

type openFileName struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File                         *uint16
	MaxFile                      uint32
	FileTitle                    *uint16
	MaxFileTitle                 uint32
	InitialDir, Title            *uint16
	Flags                        uint32
	FileOffset, FileExtension    uint16
	DefaultExt                   *uint16
	CustomData, Hook             uintptr
	TemplateName                 *uint16
	Reserved                     uintptr
	ReservedSize, FlagsEx        uint32
}

type result struct {
	batch        *project.ImportBatch
	placement    *placementRequest
	frames       *frameJobResult
	cacheCleared bool
	relink       *relinkPlan
	preferences  *session.Preferences
	err          error
	message      string
}

type previewResult struct {
	generation uint64
	err        error
}

type editor struct {
	model                   editorstate.State
	controls                map[int]uintptr
	status, host            uintptr
	layerSurface            uintptr
	layerPlan               *composition.LayerPlan
	stillWorker             *service.Latest[stillPreviewRequest, stillPreviewResult]
	stillCancel             context.CancelFunc
	stillSequence           uint64
	stillActive             bool
	stillFrame              *media.StillFrame
	stillSources            map[string]mediacache.Source
	stillFingerprint        string
	stillTime               int64
	stillError              string
	busy                    bool
	jobs                    service.Jobs[result]
	jobFingerprint          string
	projectEpoch            uint64
	previewCancel           context.CancelFunc
	previewDone             chan struct{}
	retiringPreview         <-chan struct{}
	previewResults          chan previewResult
	generation              uint64
	engine                  media.Engine
	timeline                uintptr
	laneScroll              int32
	visibleTrack            string
	layout                  []placedControl
	zoom                    float64
	origin, playhead        int64
	drag                    *timelineDrag
	clock                   *previewClock
	scrubbing, scrubPlaying bool
	cropSession             *cropSession
	transformSession        *transformSession
	transformControls       []uintptr
	inspectorControls       []uintptr
	thumbnails              *thumbnailCache
	thumbnailRevision       uint64
	waveforms               *waveformCache
	waveformRevision        uint64
	recovery                *service.Latest[session.State, struct{}]
	recoveryRequest         uint64
	recoveryDone            uint64
	closeDeadline           time.Time
	restorePending          bool
	restoreView             recoveryView
	restoreFingerprint      string
	recoveryLock            syscall.Handle
	recoveryError           string
	lastCheckpoint          time.Time
	savedView               recoveryView
	projectPath             string
	snapDisabled            bool
	assetDrag               *assetDrag
	marquee                 *marqueeDrag
	rippleTrim              bool
	clipboard               project.ClipClipboard
	menuOpen                bool
	frameCache              map[string]frameCacheEntry
	frameSnap               bool
	playbackSpeed           float64
	projectRevision         string
	recentDir               string
	recent                  []string
	mediaDisk               *mediacache.Cache
	sourceVersions          map[string]mediacache.Source
	sourceChecked           map[string]bool
	sourcePaths             []string
	sourceFingerprint       string
	sourceScanner           *service.Latest[service.SourceScan, service.SourceResult]
	sourceEpoch             uint64
	sourceRequest           uint64
	sourceDone              uint64
	retiredWorkers          []<-chan struct{}
	mediaWorkersPending     bool
	lastSourceCheck         time.Time
	mediaListDirty          bool
	prefs                   session.Preferences
	preferencesWarning      string
	previewLabel            uintptr
	timelineLabel           uintptr
	composition             *composition.Evaluator
	compositionFingerprint  string
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
	ui.layout = append(ui.layout, placedControl{hwnd, x, y, w, h, id})
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
	of := openFileName{Owner: mainWindow, Filter: &f[0], FilterIndex: 1, File: &buf[0], MaxFile: uint32(len(buf)), Title: u(title), DefaultExt: u(ext), Flags: 0x00080000 | 0x00000800 | 0x00000008 | 0x00000004}
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

func (e *editor) track() *project.Track { return &e.model.Project().Tracks[e.trackIndex()] }

func (e *editor) selected() (project.Clip, bool) {
	return e.model.Selected()
}

func fmtTime(t int64) string { return strconv.FormatFloat(project.Seconds(t), 'f', 6, 64) }

func listAdd(h uintptr, s string) { send.Call(h, lbAdd, 0, uintptr(unsafe.Pointer(u(s)))) }

func (e *editor) refresh() {
	e.syncTrackUI()
	e.checkSourceVersions(true)
	a, c := e.controls[idAssets], e.controls[idClips]
	e.refreshMediaList()
	send.Call(c, lbReset, 0, 0)
	for _, item := range e.track().Clips {
		asset, _ := e.model.Project().Asset(item.AssetID)
		listAdd(c, fmt.Sprintf("%s  |  %ss - %ss", filepath.Base(asset.Path), fmtTime(item.Start), fmtTime(item.Start+(item.Out-item.In))))
	}
	e.updateDirty()
	send.Call(a, lbSet, uintptr(e.model.AssetIndex()), 0)
	send.Call(c, lbSet, uintptr(e.model.ClipIndex()), 0)
	e.fields()
	e.invalidateTimeline()
	e.checkpoint()
}

func (e *editor) fields() {
	e.syncPreviewLabel()
	c, ok := e.selected()
	for _, id := range []int{idStart, idIn, idOut, idCrop, idAt} {
		change(e.controls[id], "")
	}
	if !ok || e.model.Count() > 1 {
		return
	}
	change(e.controls[idStart], fmtTime(c.Start))
	change(e.controls[idIn], fmtTime(c.In))
	change(e.controls[idOut], fmtTime(c.Out))
	change(e.controls[idAt], fmtTime(e.playhead))
	if c.Crop != nil {
		change(e.controls[idCrop], fmt.Sprintf("%d,%d,%d,%d", c.Crop.X, c.Crop.Y, c.Crop.Width, c.Crop.Height))
	}
}

func (e *editor) commit(q project.Project) error {
	return e.commitWithSelection(q, editorstate.SelectionUpdate{})
}

type editView struct {
	playhead int64
	zoom     float64
}

func (e *editor) commitWithSelection(q project.Project, update editorstate.SelectionUpdate) error {
	return e.commitEdit(q, update, nil)
}

func (e *editor) commitEdit(q project.Project, update editorstate.SelectionUpdate, view *editView) error {
	track, primary := e.trackID(), e.model.PrimaryID()
	changed, err := e.model.Commit(q, update)
	if err != nil {
		return err
	}
	if changed || track != e.trackID() || primary != e.model.PrimaryID() {
		e.stopPreview()
	}
	if view != nil {
		e.playhead, e.zoom = view.playhead, view.zoom
	}
	e.refresh()
	return nil
}

func nextID(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

func (e *editor) setControlsEnabled(enabled bool) {
	var value uintptr
	if enabled {
		value = 1
	}
	for _, h := range e.controls {
		user.NewProc("EnableWindow").Call(h, value)
	}
}

func (e *editor) startJob(fn func(context.Context) result) {
	hwnd := mainWindow
	if err := e.jobs.Start(e.projectEpoch, fn, func() { post.Call(hwnd, wmDone, 0, 0) }); err != nil {
		e.tell(err.Error())
		return
	}
	e.jobFingerprint = e.model.Fingerprint()
	e.busy = true
	e.setControlsEnabled(false)
}

func (e *editor) finished() {
	r, ready, current := e.jobs.Take(e.projectEpoch)
	if !ready {
		return
	}
	e.busy = false
	e.setControlsEnabled(true)
	if !current || e.jobFingerprint != e.model.Fingerprint() {
		e.tell("Discarded background result for an outdated project.")
		return
	}
	if r.cacheCleared {
		e.restartMediaWorkers()
	}
	if r.err != nil {
		e.tell(r.err.Error())
		return
	}
	if r.batch != nil {
		e.finishImport(r.batch, r.placement)
		return
	}
	if r.frames != nil {
		e.finishFrames(r.frames)
		return
	}
	if r.relink != nil {
		e.finishRelink(r.relink)
		return
	}
	if r.preferences != nil {
		if err := e.applyPreferences(*r.preferences); err != nil {
			e.tell(err.Error())
		}
		return
	}
	e.tell(r.message)
}

func (e *editor) stopPreview() {
	e.hideLayerLayout()
	if e.cropSession != nil {
		e.endVisualCropUI()
	}
	if e.transformSession != nil {
		e.endTransformUI()
	}
	e.updatePlaybackClock()
	if e.clock != nil {
		e.clock.transport.Stop()
	}
	e.clock = nil
	e.generation++
	if e.previewCancel != nil {
		e.previewCancel()
		e.retiringPreview = e.previewDone
		e.previewCancel = nil
		e.previewDone = nil
	}
	if h := e.controls[idPlay]; h != 0 {
		change(h, "Play  (Space)")
		user.NewProc("InvalidateRect").Call(h, 0, 1)
	}
}

func (e *editor) play() error { return e.startPreview(false) }

func (e *editor) startPreview(paused bool) error {
	c, ok := e.selected()
	if !ok {
		return fmt.Errorf("select a timeline clip")
	}
	a, ok := e.model.Project().Asset(c.AssetID)
	if !ok || (e.audioTrack() && !a.HasAudio) {
		return project.ErrInvalidClip
	}
	policy := e.model.Project().TrackPolicies()[e.trackID()]
	audible := policy.Audio && a.HasAudio && !c.AudioDetached
	if !policy.Video && !audible {
		e.stopPreview()
		return fmt.Errorf("active track has no enabled video or audio for this clip")
	}
	if !project.SourceAvailable(a.Path) {
		e.stopPreview()
		return fmt.Errorf("source offline: %s; use Relink", filepath.Base(a.Path))
	}
	e.stopPreview()
	if e.retiringPreview != nil {
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-e.retiringPreview:
			e.retiringPreview = nil
		case <-timer.C:
			return fmt.Errorf("previous preview is still closing; retry Play or seek")
		}
	}
	if !c.Contains(e.playhead) {
		e.playhead = c.Start
	}
	generation := e.generation
	preview, err := launchNativePreview(e.engine, e.host, a, c, e.playhead, e.speed(), e.prefs.HDR, paused, !policy.Video, !audible, func(err error) {
		select {
		case e.previewResults <- previewResult{generation, err}:
			post.Call(mainWindow, wmPreviewDone, 0, 0)
		default:
		}
	})
	if err != nil {
		return err
	}
	e.clock, e.previewCancel, e.previewDone = preview.clock, preview.cancel, preview.done
	e.updatePlayButton(paused)
	user.NewProc("SetFocus").Call(e.timeline)
	e.refreshPlaybackMarker()
	if paused {
		e.tell("Preview paused.")
	} else {
		e.tell("Playing source interval. Space or Esc pauses.")
	}
	return nil
}

func (e *editor) command(id, notification int) error {
	if e.menuOpen {
		return fmt.Errorf("close the context menu first")
	}
	if e.busy {
		return fmt.Errorf("media operation in progress")
	}
	if e.assetDrag != nil || e.marquee != nil || e.drag != nil || e.scrubbing {
		return fmt.Errorf("finish the active drag first")
	}
	if e.transformSession != nil && id != idTransformApply && id != idTransformCancel && id != idTransformReset && id != idRotateLeft && id != idRotateRight && id != idFlipH && id != idFlipV {
		return fmt.Errorf("Apply or cancel the active transform first")
	}
	if e.cropSession != nil && id != idCropVisual && id != idCropReset && id != idKeys {
		return fmt.Errorf("Finish or cancel the active crop first")
	}
	if id != idUndo && id != idRedo {
		a := e.availability()
		if err := trackCommandAllowed(id, a); err != nil {
			return err
		}
		if err := a.Check(commandRequirement(id)); err != nil {
			return err
		}
	}
	switch id {
	case idNew, idCloseProject:
		return e.switchProject("", id == idCloseProject)
	case idOpen:
		path, err := pick(false, projectFilter, "json", "Open project")
		if err != nil || path == "" {
			return err
		}
		return e.switchProject(path, false)
	case idSave, idSaveAs:
		_, err := e.saveProject(id == idSaveAs)
		return err
	case idRecent:
		return e.showRecent()
	case idMediaCache:
		return e.showMediaCache()
	case idRelinkMenu:
		return e.showRelinkMenu()
	case idRelinkAsset, idReplaceAsset:
		return e.relinkOne(e.selectedAssetID(), id == idReplaceAsset)
	case idRelinkFolder:
		return e.relinkFolder()
	case idPreferences:
		return e.showPreferences()
	case idTracks:
		return e.showTracks()
	case idImport:
		paths, err := pickImportFiles()
		if err != nil || len(paths) == 0 {
			return err
		}
		return e.importPaths(paths)
	case idAssets:
		if notification == 1 {
			n, _, _ := send.Call(e.controls[idAssets], lbGet, 0, 0)
			e.model.SetAssetIndex(int(n))
		}
	case idClips:
		if notification == 1 {
			n, _, _ := send.Call(e.controls[idClips], lbGet, 0, 0)
			e.stopPreview()
			e.model.SelectIndex(int(n))
			e.fields()
			e.invalidateTimeline()
		}
	case idAdd:
		if e.model.AssetIndex() < 0 || e.model.AssetIndex() >= len(e.model.Project().Assets) {
			return fmt.Errorf("select a media asset")
		}
		a := e.model.Project().Assets[e.model.AssetIndex()]
		var end int64
		for _, c := range e.track().Clips {
			end = max(end, c.Start+(c.Out-c.In))
		}
		r, err := e.model.Project().PlaceAssets(e.trackID(), []string{a.ID}, []string{nextID("clip")}, end, -1, 0)
		if err != nil {
			return err
		}
		if r.Start != end {
			return fmt.Errorf("clip exceeds timeline bounds")
		}
		if err = e.commitPlacement(r); err != nil {
			return err
		}
		e.tell(placementStatus(*e.track()))
	case idApply:
		c, ok := e.selected()
		if !ok {
			return fmt.Errorf("select a clip")
		}
		var err error
		if c.Start, err = project.ParseSeconds(read(e.controls[idStart])); err != nil {
			return err
		}
		if c.In, err = project.ParseSeconds(read(e.controls[idIn])); err != nil {
			return err
		}
		if c.Out, err = project.ParseSeconds(read(e.controls[idOut])); err != nil {
			return err
		}
		c.Crop = nil
		if s := strings.TrimSpace(read(e.controls[idCrop])); s != "" {
			parts := strings.Split(s, ",")
			if len(parts) != 4 {
				return fmt.Errorf("crop needs x,y,width,height")
			}
			var values [4]int
			for i, p := range parts {
				values[i], err = strconv.Atoi(strings.TrimSpace(p))
				if err != nil {
					return fmt.Errorf("invalid crop coordinate")
				}
			}
			c.Crop = &project.Crop{X: values[0], Y: values[1], Width: values[2], Height: values[3]}
		}
		q := e.model.Project().Clone()
		q.Tracks[e.trackIndex()].Clips[e.model.ClipIndex()] = c
		if err = e.commit(q); err != nil {
			return err
		}
		if e.audioTrack() {
			e.tell("Audio clip properties applied.")
		} else if err := e.startPreview(true); err != nil {
			e.tell("Edits applied; preview unavailable: " + err.Error())
		} else {
			e.tell("Clip properties applied.")
		}
	case menuDetachAudio, menuReattachAudio:
		return e.editClipAudio(id == menuDetachAudio)
	case idSplit:
		c, ok := e.selected()
		if !ok {
			return fmt.Errorf("select a clip")
		}
		at, err := project.ParseSeconds(read(e.controls[idAt]))
		if err != nil {
			return err
		}
		q, err := e.model.Project().Split(e.trackID(), c.ID, nextID("clip"), at)
		if err != nil {
			return err
		}
		if err = e.commit(q); err != nil {
			return err
		}
		e.tell("Clip split at timeline position.")
	case idRemoveAsset:
		return e.removeAsset()
	case idSnap:
		e.snapDisabled = !e.snapDisabled
		e.updateSnapButton()
		e.invalidateTimeline()
		e.tell("Magnetic snapping updated." + e.rememberPreferences())
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
	case idFrameBack, idFrameForward, idSetIn, idSetOut:
		return e.precisionAction(id)
	case idPrevEdit, idNextEdit:
		e.jumpEdit(id == idNextEdit)
	case idSpeed, idSpeedDown, idSpeedUp:
		direction := 1
		if id == idSpeedDown {
			direction = -1
		}
		e.changeSpeed(direction, id == idSpeed)
	case idFrameSnap:
		e.toggleFrameSnap()
	case idDelete:
		return e.deleteSelection()
	case idRippleDelete:
		return e.rippleDeleteSelection()
	case idRemoveGap:
		return e.removeGap()
	case idRippleTrim:
		e.rippleTrim = !e.rippleTrim
		e.updateRippleButton()
		e.tell("Ripple trimming updated." + e.rememberPreferences())
	case idUndo, idRedo:
		if (id == idUndo && !e.model.CanUndo()) || (id == idRedo && !e.model.CanRedo()) {
			return nil
		}
		e.stopPreview()
		if id == idUndo {
			e.model.Undo()
		} else {
			e.model.Redo()
		}
		e.playhead = min(e.playhead, e.endTime())
		e.refresh()
		if _, ok := e.selected(); ok && e.model.Count() <= 1 {
			if err := e.startPreview(true); err != nil {
				e.tell("History restored; preview unavailable: " + err.Error())
				return nil
			}
		}
		e.tell("History restored.")
	case idZoomOut, idZoomIn, idFit:
		e.zoomTimeline(id)
	case idTransform:
		return e.beginTransform()
	case idTransformApply:
		return e.applyTransform()
	case idTransformReset:
		e.resetTransformDraft()
	case idTransformCancel:
		e.cancelTransform()
	case idRotateLeft, idRotateRight, idFlipH, idFlipV:
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
		c, ok := e.selected()
		if !ok {
			return fmt.Errorf("select a clip")
		}
		a, ed, err := e.model.Project().MediaEdit(c)
		if err != nil {
			return err
		}
		ed.Silent = ed.Silent || e.track().Muted
		if !project.SourceAvailable(a.Path) {
			return fmt.Errorf("source offline: %s; use Relink", filepath.Base(a.Path))
		}
		path, err := pick(true, "MP4 video (*.mp4)\x00*.mp4\x00", "mp4", "Export selected clip")
		if err != nil || path == "" {
			return err
		}
		e.stopPreview()
		e.tell("Exporting selected clip with source HDR...")
		e.startJob(func(ctx context.Context) result {
			var err error
			if !ed.Trim && ed.Crop == nil && ed.Transform == nil {
				if ed.Silent {
					err = e.engine.RemuxVideoOnly(ctx, a.Path, path)
				} else {
					err = e.engine.Remux(ctx, a.Path, path)
				}
			} else {
				err = e.engine.Render(ctx, a.Path, path, ed)
			}
			return result{err: err, message: "Export complete: " + path}
		})
	}
	return nil
}

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0005:
		if ui != nil && ui.timeline != 0 {
			ui.resize()
		}
		return 0
	case 0x0024:
		if lp == 0 {
			break
		}
		var m minMaxInfo
		nativeCopy(uintptr(unsafe.Pointer(&m)), lp, unsafe.Sizeof(m))
		m.MinTrack.X, m.MinTrack.Y = 1080, 710
		nativeCopy(lp, uintptr(unsafe.Pointer(&m)), unsafe.Sizeof(m))
		runtime.KeepAlive(&m)
		return 0
	case 0x002b:
		if ui != nil && ui.drawLayerLayout(lp) {
			return 1
		}
		if drawButton(lp) {
			return 1
		}
	case 0x0133, 0x0134, 0x0138:
		return colorControl(msg, wp, lp)
	case wmCommand:
		id, notification := int(wp&0xffff), int((wp>>16)&0xffff)
		if id >= idNew && id <= idTracks && !(id >= idCanvasW && id <= idPositionY) && id != idStart && id != idIn && id != idOut && id != idCrop && id != idAt {
			if err := ui.command(id, notification); err != nil {
				ui.tell(err.Error())
			}
		}
		return 0
	case 0x0113:
		if wp == playbackTimer && ui != nil {
			if !ui.closeDeadline.IsZero() {
				ui.pollClose(hwnd)
				return 0
			}
			ui.finished()
			ui.updatePlaybackClock()
			ui.updateVisualCrop()
			ui.updateTransformDraft()
			ui.updateThumbnails()
			ui.updateWaveforms()
			ui.checkSourceVersions(false)
			ui.updateCompositeStill()
			ui.checkpointView()
			return 0
		}
	case wmDropFiles:
		ui.handleDrop(hwnd, wp)
		return 0
	case wmDone:
		ui.finished()
		return 0
	case wmPreviewDone:
		var r previewResult
		select {
		case r = <-ui.previewResults:
		default:
			return 0
		}
		if r.generation == ui.generation {
			ui.stopPreview()
			fg, _, _ := user.NewProc("GetForegroundWindow").Call()
			focus, _, _ := user.NewProc("GetFocus").Call()
			if fg == mainWindow && (focus == 0 || focus == ui.host) {
				user.NewProc("SetFocus").Call(ui.timeline)
			}
			if r.err != nil {
				ui.tell("Preview failed: " + r.err.Error())
			} else {
				ui.tell("Preview finished.")
			}
		}
		return 0
	case 0x0010:
		if ui.menuOpen {
			user.NewProc("EndMenu").Call()
			return 0
		}
		ui.cancelAssetDrag()
		ui.endMarquee(true)
		if ui.drag != nil {
			ui.cancelTimelineDrag()
		}
		if ui.scrubbing {
			ui.scrubbing = false
			user.NewProc("ReleaseCapture").Call()
		}
		if ui.cropSession != nil {
			ui.cancelVisualCrop()
		}
		if ui.transformSession != nil {
			ui.cancelTransform()
		}
		if ui.busy {
			ui.tell("Wait for the current media operation to finish.")
			return 0
		}
		ui.beginClose(hwnd)
		return 0
	case 0x0002:
		user.NewProc("KillTimer").Call(hwnd, playbackTimer)
		ui.stopPreview()
		if ui.stillWorker != nil {
			ui.stillWorker.Close()
		}
		ui.retireMediaWorkers()
		ui.jobs.Cancel()
		if ui.sourceScanner != nil {
			ui.sourceScanner.Close()
		}
		if ui.recovery != nil {
			ui.recovery.Close()
		}
		user.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ = kernel.NewProc("GetModuleHandleW").Call(0)
	initTheme()
	font = modernFont()
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	className := u("PixelCutEditor")
	wc := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: syscall.NewCallback(wndProc), Instance: instance, Cursor: cursor, Background: theme.background, Name: className}
	if r, _, err := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ui = &editor{model: editorstate.New(project.New()), controls: map[int]uintptr{}, previewResults: make(chan previewResult, 16), engine: media.Default()}
	if err := ui.openRecovery(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(u(err.Error()))), uintptr(unsafe.Pointer(u("PixelCut recovery"))), 0x10)
		freeTheme()
		return
	}
	defer ui.closeRecovery()
	ui.loadPreferences()
	registerTimeline()
	mainWindow, _, _ = createWindow.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(u("PixelCut - Editor preview"))), 0x00cf0000, 100, 70, 1200, 790, 0, 0, instance, 0)
	if mainWindow == 0 {
		fmt.Fprintln(os.Stderr, "unable to create PixelCut window")
		os.Exit(1)
	}
	button("New", 12, 10, 70, idNew)
	button("Open", 88, 10, 70, idOpen)
	button("Save", 164, 10, 70, idSave)
	button("Save As", 240, 10, 80, idSaveAs)
	button("Recent", 326, 10, 78, idRecent)
	button("Close", 410, 10, 70, idCloseProject)
	button("Undo", 486, 10, 65, idUndo)
	button("Redo", 557, 10, 65, idRedo)
	button("Shortcuts", 628, 10, 110, idKeys)
	button("Cache", 744, 10, 75, idMediaCache)
	button("Relink", 825, 10, 132, idRelinkMenu)
	button("Export clip", 970, 10, 126, idExport)
	applyDarkTitle(mainWindow)
	label("Media bin / Drop files", 12, 50, 245)
	control("LISTBOX", "", wsBorder|wsTab|0x00200000|1|0x100, 12, 75, 245, 270, idAssets)
	button("Import video", 12, 350, 118, idImport)
	button("Add to V1", 136, 350, 121, idAdd)
	button("Remove asset", 12, 384, 245, idRemoveAsset)
	ui.previewLabel = label("Source preview / SDR", 270, 50, 570)
	ui.host = control("STATIC", "", 0x02000000|0x04000000|4, 270, 75, 540, 303, 0)
	ui.layerSurface = control("STATIC", "", 0x0000000d, 270, 75, 540, 303, layerSurfaceID)
	user.NewProc("ShowWindow").Call(ui.layerSurface, 0)
	button("Play  (Space)", 270, 384, 124, idPlay)
	button("Stop", 400, 384, 80, idStop)
	button("< Frame", 486, 384, 70, idFrameBack)
	button("Frame >", 562, 384, 70, idFrameForward)
	button("Speed: 1x", 638, 384, 85, idSpeed)
	button("Frames: off", 729, 384, 81, idFrameSnap)
	label("Clip properties (seconds)", 825, 50, 270)
	label("Timeline start", 825, 80, 145)
	edit("", 975, 77, 121, idStart)
	label("Source in", 825, 112, 145)
	edit("", 975, 109, 121, idIn)
	label("Source out", 825, 144, 145)
	edit("", 975, 141, 121, idOut)
	label("Crop: x,y,width,height (blank = full)", 825, 176, 270)
	edit("", 825, 199, 271, idCrop)
	button("Apply edits", 825, 234, 130, idApply)
	button("Transform", 961, 234, 135, idTransform)
	button("Visual crop", 825, 370, 130, idCropVisual)
	button("Reset crop", 961, 370, 135, idCropReset)
	ui.initTransformInspector()
	label("Split at timeline time", 825, 273, 270)
	edit("", 825, 297, 155, idAt)
	button("Split", 986, 296, 110, idSplit)
	button("Remove clip", 825, 332, 130, idDelete)
	ui.timelineLabel = label("TIMELINE / V1 / active track", 12, 423, 180)
	button("Track: V1 (2)", 196, 419, 128, idTracks)
	button("Ripple delete", 330, 419, 125, idRippleDelete)
	button("Remove gap", 461, 419, 105, idRemoveGap)
	button("Ripple: off", 572, 419, 120, idRippleTrim)
	button("Settings", 698, 419, 84, idPreferences)
	button("-", 832, 419, 48, idZoomOut)
	button("+", 886, 419, 48, idZoomIn)
	button("Fit", 940, 419, 70, idFit)
	button("Snap: on", 1016, 419, 80, idSnap)
	control("LISTBOX", "", wsBorder|wsTab|0x00200000|1|0x100, 12, 448, 1084, 180, idClips)
	user.NewProc("ShowWindow").Call(ui.controls[idClips], 0)
	ui.timeline = control("PixelCutTimeline", "", wsTab|0x04000000, 12, 452, 1084, 176, 0)
	ui.status = label("Drop supported HLG videos here, or choose Import video.", 12, 640, 1084)
	ui.openMediaCache()
	ui.restartMediaWorkers()
	ui.enableFileDrops()
	ui.refresh()
	ui.syncPreferencesUI()
	ui.resize()
	if timer, _, _ := user.NewProc("SetTimer").Call(mainWindow, playbackTimer, 33, 0); timer == 0 {
		fmt.Fprintln(os.Stderr, "unable to start playback UI timer")
	}
	user.NewProc("ShowWindow").Call(mainWindow, 5)
	user.NewProc("UpdateWindow").Call(mainWindow)
	ui.finishRecovery()
	if ui.preferencesWarning != "" {
		ui.tell(ui.preferencesWarning)
	}
	var m message
	for {
		r, _, _ := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if ui.handleShortcut(m) {
			continue
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	freeTheme()
}

func (e *editor) removeAsset() error {
	if e.model.AssetIndex() < 0 || e.model.AssetIndex() >= len(e.model.Project().Assets) {
		return fmt.Errorf("select a media asset")
	}
	a := e.model.Project().Assets[e.model.AssetIndex()]
	count := e.model.Project().AssetReferences(a.ID)
	if count > 0 {
		message := fmt.Sprintf("Remove %s and its %d timeline clip(s)? The source file will remain untouched.", filepath.Base(a.Path), count)
		r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(message))), uintptr(unsafe.Pointer(u("Remove media asset"))), 0x134)
		if r != 6 {
			return nil
		}
	}
	q, err := e.model.Project().RemoveAsset(a.ID, count > 0)
	if err != nil {
		return err
	}
	assetIndex := min(e.model.AssetIndex(), len(q.Assets)-1)
	view := editView{playhead: min(e.playhead, q.EndTime()), zoom: e.zoom}
	assetID := ""
	if assetIndex >= 0 {
		assetID = q.Assets[assetIndex].ID
	}
	if err = e.commitEdit(q, editorstate.SelectionUpdate{ReplaceAsset: true, AssetID: assetID}, &view); err != nil {
		return err
	}
	e.tell("Asset removed from project; source file unchanged. Ctrl+Z to undo.")
	return nil
}

func (e *editor) updateSnapButton() {
	label := "Snap: on"
	if e.snapDisabled {
		label = "Snap: off"
	}
	h := e.controls[idSnap]
	change(h, label)
	user.NewProc("InvalidateRect").Call(h, 0, 1)
}

func (e *editor) selectExistingAsset(index int) {
	e.model.SetAssetIndex(index)
	send.Call(e.controls[idAssets], lbSet, uintptr(index), 0)
	e.checkpoint()
	e.tell("Already imported: " + filepath.Base(e.model.Project().Assets[index].Path))
}
