package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/service"
	"github.com/markrai/pixelcut/internal/session"
)

type recoveryView struct {
	asset, clip      int
	playhead, origin int64
	zoom             float64
	selection        string
	track            string
}

func (e *editor) view() recoveryView {
	return recoveryView{e.model.AssetIndex(), e.model.ClipIndex(), e.playhead, e.origin, e.zoom, e.selectionKey(), e.trackID()}
}

func (e *editor) openRecovery() error {
	dir, err := os.UserCacheDir()
	if err != nil {
		e.recoveryError = err.Error()
		return nil
	}
	dir = filepath.Join(dir, "PixelCut")
	if err = os.MkdirAll(dir, 0700); err != nil {
		e.recoveryError = err.Error()
		return nil
	}
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, "session.lock"))
	if err != nil {
		e.recoveryError = err.Error()
		return nil
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if err == syscall.Errno(32) || err == syscall.Errno(33) {
			return fmt.Errorf("PixelCut is already running. Close the other instance before reopening.")
		}
		e.recoveryError = err.Error()
		return nil
	}
	e.recoveryLock = h
	e.recentDir = dir
	if paths, err := session.LoadRecent(dir); err == nil {
		e.recent = paths
	} else {
		fmt.Fprintln(os.Stderr, "Recent projects:", err)
	}
	store, state, err := session.Open(dir)
	if err != nil {
		e.recoveryError = err.Error()
		return nil
	}
	e.recovery = service.NewLatest(func(_ context.Context, s session.State) (struct{}, error) {
		return struct{}{}, store.Save(s)
	})
	if state != nil {
		e.model = editorstate.New(state.Project)
		e.model.Restore(state.ActiveTrack, state.Asset, state.Clip, state.SelectedClips)
		e.playhead, e.origin, e.zoom = state.Playhead, state.Origin, state.Zoom
		e.projectPath, e.projectRevision = state.ProjectPath, state.ProjectRevision
		if state.SavedFingerprint != "" || state.Dirty {
			e.model.SetSavedBaseline(state.SavedFingerprint)
		}
		e.restoreSaveBaseline()
		q, merged, err := e.model.Project().MergeDuplicateAssets()
		if err == nil && len(merged) > 0 {
			selectedID := e.model.AssetID()
			if id, ok := merged[selectedID]; ok {
				selectedID = id
			}
			baseline := e.model.SavedFingerprint()
			e.model = editorstate.New(q)
			e.model.Restore(state.ActiveTrack, q.AssetIndex(selectedID), state.Clip, state.SelectedClips)
			e.model.SetSavedBaseline(baseline)
		}
	}
	e.savedView = e.view()
	return nil
}

func (e *editor) closeRecovery() {
	if e.recovery != nil {
		e.recovery.Close()
		select {
		case <-e.recovery.Done():
		default:
			return
		}
	}
	if e.recoveryLock != 0 {
		syscall.CloseHandle(e.recoveryLock)
		e.recoveryLock = 0
	}
}

func (e *editor) checkpoint() bool {
	if e.recovery == nil {
		return false
	}
	e.lastCheckpoint = time.Now()
	s := session.State{Project: e.model.Project(), Asset: e.model.AssetIndex(), Clip: e.model.ClipIndex(), ActiveTrack: e.trackID(), Playhead: e.playhead, Origin: e.origin, Zoom: e.zoom, Dirty: e.model.Dirty(), ProjectPath: e.projectPath, ProjectRevision: e.projectRevision, SavedFingerprint: e.model.SavedFingerprint()}
	if e.model.Count() > 1 {
		s.SelectedClips = e.selectionIDs()
	}
	e.recoveryRequest = e.recovery.Submit(s)
	e.savedView = e.view()
	return e.recoveryRequest != 0
}

func (e *editor) pollRecovery() {
	if e.recovery == nil {
		return
	}
	r := e.recovery.Result()
	if r.Sequence == 0 || r.Sequence <= e.recoveryDone {
		return
	}
	e.recoveryDone = r.Sequence
	note := ""
	if r.Err != nil {
		note = r.Err.Error()
	}
	if note != e.recoveryError {
		e.recoveryError = note
		if note != "" {
			fmt.Fprintln(os.Stderr, "PixelCut recovery:", note)
		}
		e.updateRecoveryTitle()
	}
}

func (e *editor) checkpointView() {
	e.pollRecovery()
	if e.recovery == nil || e.assetDrag != nil || e.marquee != nil || e.drag != nil || e.scrubbing || time.Since(e.lastCheckpoint) < time.Duration(max(1, e.prefs.ViewSeconds))*time.Second {
		return
	}
	if e.savedView != e.view() || (e.recoveryError != "" && e.recoveryDone == e.recoveryRequest) {
		e.checkpoint()
	}
}

func (e *editor) beginClose(hwnd uintptr) {
	if !e.checkpoint() {
		if e.warnRecoveryClose() {
			destroy.Call(hwnd)
		}
		return
	}
	if timer, _, _ := user.NewProc("SetTimer").Call(hwnd, playbackTimer, 33, 0); timer == 0 {
		if e.warnRecoveryClose() {
			destroy.Call(hwnd)
		}
		return
	}
	e.closeDeadline = time.Now().Add(5 * time.Second)
	e.busy = true
	e.setControlsEnabled(false)
	e.tell("Saving final recovery checkpoint...")
}

func (e *editor) pollClose(hwnd uintptr) {
	if e.closeDeadline.IsZero() {
		return
	}
	e.pollRecovery()
	if e.recoveryDone < e.recoveryRequest && time.Now().Before(e.closeDeadline) {
		return
	}
	e.closeDeadline = time.Time{}
	if e.recoveryDone == e.recoveryRequest && e.recoveryError == "" {
		destroy.Call(hwnd)
		return
	}
	if e.warnRecoveryClose() {
		destroy.Call(hwnd)
		return
	}
	e.busy = false
	e.setControlsEnabled(true)
}

func (e *editor) updateRecoveryTitle() {
	title := projectTitle(e.projectPath, e.model.Dirty(), e.recoveryError != "")
	if mainWindow != 0 {
		change(mainWindow, title)
	}
}

func (e *editor) finishRecovery() {
	if e.recoveryError != "" {
		e.updateRecoveryTitle()
		e.tell("Automatic recovery unavailable: " + e.recoveryError)
		return
	}
	if len(e.model.Project().Assets) == 0 {
		return
	}
	if !e.sourcesReady() {
		e.restorePending = true
		e.restoreView = e.view()
		e.restoreFingerprint = e.model.Fingerprint()
		return
	}
	missing := e.missingCount()
	if missing > 0 {
		e.tell(fmt.Sprintf("Restored session; %d source file(s) offline. References retained; use Relink.", missing))
		return
	}
	if c, ok := e.selected(); e.prefs.RestorePreview && ok && e.playhead >= c.Start && e.playhead < c.Start+(c.Out-c.In) {
		if err := e.startPreview(true); err != nil {
			e.tell("Restored session; preview unavailable: " + err.Error())
			return
		}
	}
	e.tell("Previous session restored.")
}

func (e *editor) warnRecoveryClose() bool {
	r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u("Automatic recovery failed. Unsaved changes may be lost. Close anyway?"))), uintptr(unsafe.Pointer(u("PixelCut"))), 0x34)
	return r == 6
}
