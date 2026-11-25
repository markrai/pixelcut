package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
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
	e.savedFingerprint = e.model.Project().Fingerprint()
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
	e.recovery = store
	if state != nil {
		e.model = editorstate.New(state.Project)
		e.model.Restore(state.ActiveTrack, state.Asset, state.Clip, state.SelectedClips)
		e.playhead, e.origin, e.zoom = state.Playhead, state.Origin, state.Zoom
		e.dirty, e.projectPath = state.Dirty, state.ProjectPath
		e.projectRevision, e.savedFingerprint = state.ProjectRevision, state.SavedFingerprint
		e.restoreSaveBaseline()
		q, merged, err := e.model.Project().MergeDuplicateAssets()
		if err == nil && len(merged) > 0 {
			selectedID := e.model.AssetID()
			if id, ok := merged[selectedID]; ok {
				selectedID = id
			}
			e.model.Adopt(q)
			e.model.SetAssetID(selectedID)
			e.dirty = true
		}
	}
	e.savedView = e.view()
	return nil
}

func (e *editor) closeRecovery() {
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
	s := session.State{Project: e.model.Project(), Asset: e.model.AssetIndex(), Clip: e.model.ClipIndex(), ActiveTrack: e.trackID(), Playhead: e.playhead, Origin: e.origin, Zoom: e.zoom, Dirty: e.dirty, ProjectPath: e.projectPath, ProjectRevision: e.projectRevision, SavedFingerprint: e.savedFingerprint}
	if e.model.Count() > 1 {
		s.SelectedClips = e.selectionIDs()
	}
	if err := e.recovery.Save(s); err != nil {
		if e.recoveryError != err.Error() {
			e.recoveryError = err.Error()
			fmt.Fprintln(os.Stderr, "PixelCut recovery:", err)
			e.updateRecoveryTitle()
		}
		return false
	}
	e.savedView = e.view()
	if e.recoveryError != "" {
		e.recoveryError = ""
		e.updateRecoveryTitle()
	}
	return true
}

func (e *editor) checkpointView() {
	if e.recovery == nil || e.assetDrag != nil || e.marquee != nil || e.drag != nil || e.scrubbing || time.Since(e.lastCheckpoint) < time.Duration(max(1, e.prefs.ViewSeconds))*time.Second {
		return
	}
	if e.savedView != e.view() || e.recoveryError != "" {
		e.checkpoint()
	}
}

func (e *editor) updateRecoveryTitle() {
	title := projectTitle(e.projectPath, e.dirty, e.recoveryError != "")
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
	missing := 0
	for _, a := range e.model.Project().Assets {
		if info, err := os.Stat(a.Path); err != nil || !info.Mode().IsRegular() {
			missing++
		}
	}
	if missing > 0 {
		e.tell(fmt.Sprintf("Restored session; %d source file(s) offline. References retained; use Relink.", missing))
		return
	}
	if c, ok := e.selected(); e.prefs.RestorePreview && !e.audioTrack() && ok && e.playhead >= c.Start && e.playhead < c.Start+(c.Out-c.In) {
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
