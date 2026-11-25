package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

type verifiedRelink struct {
	asset    project.Asset
	original project.Asset
	stamp    mediacache.Source
	replace  bool
}

type relinkPlan struct {
	changes []verifiedRelink
	issues  []project.RelinkIssue
	folder  string
}

func (e *editor) missingCount() int {
	n := 0
	for _, a := range e.model.Project().Assets {
		if e.offline(a.Path) {
			n++
		}
	}
	return n
}

func (e *editor) offline(path string) bool {
	_, ok := e.sourceVersions[path]
	return e.sourceChecked[path] && !ok
}

func (e *editor) refreshMediaList() {
	h := e.controls[idAssets]
	if h == 0 {
		return
	}
	top, _, _ := send.Call(h, 0x018e, 0, 0)
	send.Call(h, lbReset, 0, 0)
	for _, a := range e.model.Project().Assets {
		prefix := ""
		if e.offline(a.Path) {
			prefix = "[OFFLINE] "
		} else if !e.sourceChecked[a.Path] {
			prefix = "[CHECKING] "
		}
		listAdd(h, fmt.Sprintf("%s%s  [%dx%d]", prefix, filepath.Base(a.Path), a.Width, a.Height))
	}
	send.Call(h, lbSet, uintptr(e.model.AssetIndex()), 0)
	if len(e.model.Project().Assets) > 0 && int(top) >= 0 {
		send.Call(h, 0x0197, uintptr(min(int(top), len(e.model.Project().Assets)-1)), 0)
	}
	label := "Relink"
	if n := e.missingCount(); n > 0 {
		label = fmt.Sprintf("Relink (%d)", n)
	}
	if button := e.controls[idRelinkMenu]; button != 0 {
		change(button, label)
		user.NewProc("InvalidateRect").Call(button, 0, 1)
	}
	e.mediaListDirty = false
}

func (e *editor) missingNotice() string {
	n := e.missingCount()
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" %d offline source(s); use Relink.", n)
}

func (e *editor) selectedAssetID() string { return e.model.AssetID() }

func (e *editor) showRelinkMenu() error {
	if !e.canOpenMenu() {
		return nil
	}
	e.checkSourceVersions(true)
	e.refreshMediaList()
	id := e.selectedAssetID()
	a, selected := e.model.Project().Asset(id)
	items := []menuItem{
		item(idRelinkAsset, "Relink selected offline asset...", selected && e.offline(a.Path)),
		item(idReplaceAsset, "Replace selected source...", selected),
		separator(),
		item(idRelinkFolder, "Search folder for offline assets...", e.missingCount() > 0),
	}
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idRelinkMenu], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate Relink button")
	}
	action := e.popup(items, point{r.Left, r.Bottom})
	if action != 0 {
		return e.command(action, 0)
	}
	return nil
}

func (e *editor) relinkOne(id string, replace bool) error {
	if !e.canOpenMenu() {
		return fmt.Errorf("finish the current operation first")
	}
	a, ok := e.model.Project().Asset(id)
	if !ok {
		return fmt.Errorf("select a media-bin asset")
	}
	if !replace && project.SourceAvailable(a.Path) {
		return fmt.Errorf("source is available; use Replace source instead")
	}
	title := "Locate " + filepath.Base(a.Path)
	if replace {
		title = "Replace " + filepath.Base(a.Path)
	}
	path, err := pick(false, "Video files (*.mp4;*.mov)\x00*.mp4;*.mov\x00All files\x00*.*\x00", "mp4", title)
	if err != nil || path == "" {
		return err
	}
	e.startRelink("", []project.RelinkCandidate{{AssetID: id, Path: path}}, replace)
	return nil
}

func (e *editor) relinkFolder() error {
	if !e.canOpenMenu() {
		return fmt.Errorf("finish the current operation first")
	}
	if e.missingCount() == 0 {
		return fmt.Errorf("no offline assets to relink")
	}
	dir, err := pickRelinkFolder()
	if err != nil || dir == "" {
		return err
	}
	e.startRelink(dir, nil, false)
	return nil
}

func (e *editor) startRelink(folder string, requests []project.RelinkCandidate, replace bool) {
	e.stopPreview()
	snapshot := e.model.Project().Clone()
	engine := e.engine
	e.tell("Checking relink candidates against existing source metadata...")
	e.startJob(func(ctx context.Context) result {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		plan := &relinkPlan{folder: folder}
		if folder != "" {
			var err error
			requests, plan.issues, err = snapshot.FindRelinkCandidates(ctx, folder)
			if err != nil {
				return result{err: err}
			}
		}
		q := snapshot
		for _, request := range requests {
			if err := ctx.Err(); err != nil {
				return result{err: err}
			}
			old, ok := q.Asset(request.AssetID)
			if !ok {
				return result{err: fmt.Errorf("relink asset not found")}
			}
			change, next, err := verifyRelink(ctx, engine, q, old, request.Path, replace)
			if err != nil {
				plan.issues = append(plan.issues, project.RelinkIssue{AssetID: old.ID, Name: filepath.Base(old.Path), Reason: err.Error()})
				continue
			}
			q = next
			plan.changes = append(plan.changes, change)
		}
		if err := ctx.Err(); err != nil {
			return result{err: err}
		}
		return result{relink: plan}
	})
}

func verifyRelink(ctx context.Context, engine media.Engine, p project.Project, old project.Asset, path string, replace bool) (verifiedRelink, project.Project, error) {
	var change verifiedRelink
	stamp, err := mediacache.Identify(path)
	if err != nil {
		return change, p, err
	}
	job, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	report, err := engine.Inspect(job, stamp.Path)
	if err != nil {
		return change, p, err
	}
	if !sourceMatches(stamp.Path, stamp) {
		return change, p, fmt.Errorf("candidate changed during inspection")
	}
	a, err := project.FromReport(old.ID, stamp.Path, report)
	if err != nil {
		return change, p, err
	}
	q, err := p.RelinkAsset(old.ID, a, replace)
	if err != nil {
		return change, p, err
	}
	return verifiedRelink{a, old, stamp, replace}, q, nil
}

func shortRelinkText(s string, limit int) string {
	r := []rune(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	if len(r) > limit {
		return string(r[:limit]) + "..."
	}
	return string(r)
}

func relinkIssueText(issues []project.RelinkIssue) string {
	var b strings.Builder
	for i, v := range issues {
		if i >= 6 {
			fmt.Fprintf(&b, "\n%d more unresolved asset(s).", len(issues)-i)
			break
		}
		fmt.Fprintf(&b, "\n%s: %s", shortRelinkText(v.Name, 120), shortRelinkText(v.Reason, 250))
	}
	return b.String()
}

func (e *editor) finishRelink(plan *relinkPlan) {
	if len(plan.changes) == 0 {
		msg := "No compatible relink candidates found." + relinkIssueText(plan.issues)
		e.tell("No sources changed. Existing references retained.")
		e.menuOpen = true
		user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(msg))), uintptr(unsafe.Pointer(u("PixelCut relink"))), 0x30)
		e.menuOpen = false
		return
	}
	var b strings.Builder
	if plan.folder != "" {
		fmt.Fprintf(&b, "Relink %d compatible offline asset(s) to files in:\n%s\n", len(plan.changes), shortRelinkText(plan.folder, 400))
		for i, v := range plan.changes {
			if i >= 8 {
				fmt.Fprintf(&b, "\n%d more matched asset(s).", len(plan.changes)-i)
				break
			}
			fmt.Fprintf(&b, "\n%s (%d clip(s))", shortRelinkText(filepath.Base(v.asset.Path), 120), e.model.Project().AssetReferences(v.asset.ID))
		}
	} else {
		v := plan.changes[0]
		verb := "Relink"
		if v.replace {
			verb = "Replace"
		}
		fmt.Fprintf(&b, "%s source for %d clip(s)?\n\nFrom: %s\nTo: %s\n\nDuration: %.6fs to %.6fs", verb, e.model.Project().AssetReferences(v.asset.ID), shortRelinkText(v.original.Path, 400), shortRelinkText(v.asset.Path, 400), project.Seconds(v.original.Duration), project.Seconds(v.asset.Duration))
	}
	if len(plan.issues) > 0 {
		fmt.Fprintf(&b, "\n\n%d unresolved asset(s) will remain offline:%s", len(plan.issues), relinkIssueText(plan.issues))
	}
	b.WriteString("\n\nCompatible metadata does not prove identical footage. Confirm these are the intended files. Existing clip edits and source files will be preserved.")
	e.menuOpen = true
	r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(b.String()))), uintptr(unsafe.Pointer(u("Confirm media relink"))), 0x134)
	e.menuOpen = false
	if r != 6 {
		e.tell("Relink canceled; no sources changed.")
		return
	}
	q, err := applyRelinkPlan(e.model.Project(), plan)
	if err != nil {
		e.tell("Relink canceled: " + err.Error())
		return
	}
	if err := e.commit(q); err != nil {
		e.tell(err.Error())
		return
	}
	e.tell(fmt.Sprintf("Updated %d source reference(s); %d unresolved.%s Ctrl+Z to undo.", len(plan.changes), len(plan.issues), e.missingNotice()))
}

func applyRelinkPlan(p project.Project, plan *relinkPlan) (project.Project, error) {
	q := p
	for _, v := range plan.changes {
		old, ok := q.Asset(v.original.ID)
		if !ok || old != v.original || !sourceMatches(v.asset.Path, v.stamp) {
			return p, fmt.Errorf("a source or candidate changed during confirmation")
		}
		next, err := q.RelinkAsset(v.original.ID, v.asset, v.replace)
		if err != nil {
			return p, err
		}
		q = next
	}
	return q, nil
}

type browseInfo struct {
	owner, root        uintptr
	displayName, title *uint16
	flags              uint32
	callback, param    uintptr
	image              int32
}

func pickRelinkFolder() (string, error) {
	ole := syscall.NewLazyDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return "", fmt.Errorf("folder dialog COM initialization failed: 0x%x", uint32(hr))
	}
	defer ole.NewProc("CoUninitialize").Call()
	display := make([]uint16, 32768)
	title := u("Select the folder containing the missing recordings (not recursive)")
	info := browseInfo{owner: mainWindow, displayName: &display[0], title: title, flags: 0x41}
	pidl, _, _ := shell.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(display)
	runtime.KeepAlive(title)
	if pidl == 0 {
		return "", nil
	}
	defer ole.NewProc("CoTaskMemFree").Call(pidl)
	buf := make([]uint16, 32768)
	ok, _, _ := shell.NewProc("SHGetPathFromIDListEx").Call(pidl, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	runtime.KeepAlive(buf)
	if ok == 0 {
		return "", fmt.Errorf("could not resolve selected folder")
	}
	path := syscall.UTF16ToString(buf)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid selected folder")
	}
	return path, nil
}
