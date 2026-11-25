package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/session"
)

const projectFilter = "PixelCut project (*.pixelcut.json)\x00*.pixelcut.json\x00"

func projectName(path string) string {
	if path == "" {
		return "Untitled"
	}
	name := filepath.Base(path)
	if strings.HasSuffix(strings.ToLower(name), ".pixelcut.json") {
		name = name[:len(name)-len(".pixelcut.json")]
	}
	if name == "" {
		return "Untitled"
	}
	return name
}

func projectTitle(path string, dirty, recoveryFailed bool) string {
	title := "PixelCut - " + projectName(path)
	if dirty {
		title += " *"
	}
	if recoveryFailed {
		title += " [recovery unavailable]"
	}
	return title
}

func (e *editor) updateDirty() {
	e.updateRecoveryTitle()
}

func (e *editor) restoreSaveBaseline() {
	if e.projectPath == "" {
		return
	}
	p, rev, err := project.LoadRevision(e.projectPath)
	if err == nil && p.Fingerprint() == e.model.Project().Fingerprint() {
		e.projectRevision = rev
		e.model.MarkSaved()
		return
	}
	if err != nil || rev != e.projectRevision {
		e.model.SetSavedBaseline("")
	}
}

func (e *editor) rememberProject(path string) string {
	e.recent = session.TouchRecent(e.recent, path)
	if e.recentDir == "" {
		return " Recent projects are not persisted."
	}
	if err := session.SaveRecent(e.recentDir, e.recent); err != nil {
		return " Recent projects could not be saved: " + err.Error()
	}
	return ""
}

func (e *editor) saveProject(as bool) (bool, error) {
	path := e.projectPath
	if as || path == "" {
		var err error
		path, err = pick(true, projectFilter, "json", "Save project as")
		if err != nil || path == "" {
			return false, err
		}
		if !strings.HasSuffix(strings.ToLower(path), ".pixelcut.json") {
			path = strings.TrimSuffix(path, filepath.Ext(path)) + ".pixelcut.json"
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return false, err
		}
		rev, err := e.model.Project().SaveNew(path)
		if err != nil {
			return false, err
		}
		e.projectRevision = rev
	} else {
		rev, err := e.model.Project().SaveExisting(path, e.projectRevision)
		if err != nil {
			return false, err
		}
		e.projectRevision = rev
	}
	e.projectPath = path
	e.model.MarkSaved()
	e.updateRecoveryTitle()
	recovered := e.checkpoint()
	note := e.rememberProject(path)
	if !recovered {
		note += " Recovery checkpoint unavailable."
	}
	e.tell("Project saved: " + path + note)
	return true, nil
}

func (e *editor) confirmProjectSwitch() (bool, error) {
	if !e.model.Dirty() {
		return true, nil
	}
	msg := "Save changes to " + projectName(e.projectPath) + " before switching projects?\n\nYes: Save\nNo: Discard\nCancel: Keep editing"
	r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(msg))), uintptr(unsafe.Pointer(u("PixelCut"))), 0x223)
	switch r {
	case 6:
		return e.saveProject(false)
	case 7:
		return true, nil
	default:
		return false, nil
	}
}

func loadEditorProject(path string) (project.Project, string, error) {
	q, rev, err := project.LoadRevision(path)
	if err != nil {
		return q, "", err
	}
	if err = q.ValidateEditorLayout(); err != nil {
		return q, "", err
	}
	return q, rev, nil
}

func (e *editor) switchProject(path string, closing bool) error {
	var err error
	q := project.New()
	rev := ""
	if path != "" {
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		q, rev, err = loadEditorProject(path)
		if err != nil {
			return err
		}
	}
	ok, err := e.confirmProjectSwitch()
	if err != nil || !ok {
		return err
	}
	if path != "" {
		q, rev, err = loadEditorProject(path)
		if err != nil {
			return err
		}
	}
	fingerprint := q.Fingerprint()
	q, merged, err := q.MergeDuplicateAssets()
	if err != nil {
		return err
	}
	e.stopPreview()
	e.clipboard = project.ClipClipboard{}
	e.model = editorstate.New(q)
	e.model.SetSavedBaseline(fingerprint)
	e.origin, e.playhead, e.zoom = 0, 0, 0
	e.projectPath, e.projectRevision = path, rev
	e.refresh()
	note := ""
	if path != "" {
		note = e.rememberProject(path)
	}
	switch {
	case closing:
		e.tell("Project closed. Empty workspace ready.")
	case len(merged) > 0:
		e.tell(fmt.Sprintf("Project opened. Merged %d duplicate asset(s); all clips retained.", len(merged)) + note + e.missingNotice())
	case path != "":
		e.tell("Project opened: " + path + note + e.missingNotice())
	default:
		e.tell("New untitled project.")
	}
	return nil
}

const recentMenuBase = 3000
const recentMenuClear = recentMenuBase + session.MaxRecent

func (e *editor) showRecent() error {
	if !e.canOpenMenu() {
		return nil
	}
	paths := append([]string(nil), e.recent...)
	var items []menuItem
	for i, p := range paths {
		label := strings.ReplaceAll(projectName(p), "&", "&&")
		parent := strings.ReplaceAll(filepath.Dir(p), "&", "&&")
		if info, err := os.Stat(p); err != nil || !info.Mode().IsRegular() {
			label += " [missing]"
		}
		items = append(items, item(recentMenuBase+i, label+"  ("+parent+")", true))
	}
	if len(items) == 0 {
		items = append(items, item(recentMenuBase, "No recent projects", false))
	}
	items = append(items, separator(), item(recentMenuClear, "Clear recent projects", len(paths) > 0))
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idRecent], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate Recent button")
	}
	id := e.popup(items, point{r.Left, r.Bottom})
	if id == recentMenuClear {
		if e.recentDir != "" {
			if err := session.SaveRecent(e.recentDir, nil); err != nil {
				return err
			}
		}
		e.recent = nil
		e.tell("Recent projects cleared.")
		return nil
	}
	if id >= recentMenuBase && id < recentMenuBase+len(paths) {
		return e.switchProject(paths[id-recentMenuBase], false)
	}
	return nil
}
