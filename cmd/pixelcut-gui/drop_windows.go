package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

const wmDropFiles = 0x0233

var shell = syscall.NewLazyDLL("shell32.dll")
var acceptFiles = shell.NewProc("DragAcceptFiles")
var queryFile = shell.NewProc("DragQueryFileW")
var finishDrop = shell.NewProc("DragFinish")
var commonControls = syscall.NewLazyDLL("comctl32.dll")
var subclassWindow = commonControls.NewProc("SetWindowSubclass")
var removeSubclass = commonControls.NewProc("RemoveWindowSubclass")
var defSubclass = commonControls.NewProc("DefSubclassProc")
var mediaDropCallback uintptr

func readDroppedFiles(drop uintptr) ([]string, error) {
	defer finishDrop.Call(drop)
	count, _, _ := queryFile.Call(drop, 0xffffffff, 0, 0)
	if count == 0 || count > project.MaxImportFiles {
		return nil, fmt.Errorf("drop between 1 and %d files", project.MaxImportFiles)
	}
	paths := make([]string, 0, count)
	for i := uintptr(0); i < count; i++ {
		n, _, _ := queryFile.Call(drop, i, 0, 0)
		if n == 0 || n >= 32768 {
			return nil, fmt.Errorf("invalid dropped file path")
		}
		buf := make([]uint16, n+1)
		got, _, _ := queryFile.Call(drop, i, uintptr(unsafe.Pointer(&buf[0])), n+1)
		runtime.KeepAlive(buf)
		if got != n {
			return nil, fmt.Errorf("could not read dropped file path")
		}
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	return paths, nil
}

func (e *editor) handleDrop(hwnd, drop uintptr) {
	var req *placementRequest
	var p point
	if hwnd == e.timeline || hwnd == mainWindow {
		ok, _, _ := shell.NewProc("DragQueryPoint").Call(drop, uintptr(unsafe.Pointer(&p)))
		if ok != 0 {
			if hwnd != e.timeline {
				user.NewProc("MapWindowPoints").Call(hwnd, e.timeline, uintptr(unsafe.Pointer(&p)), 1)
			}
			req, _ = e.placementAt(p.X, p.Y)
		}
	}
	paths, err := readDroppedFiles(drop)
	if err == nil {
		err = e.importAt(paths, req)
	}
	if err != nil {
		e.tell(err.Error())
	}
}

func mediaDropProc(hwnd uintptr, msg uint32, wp, lp, id, data uintptr) uintptr {
	if msg == wmDropFiles && ui != nil {
		ui.handleDrop(hwnd, wp)
		return 0
	}
	if ui != nil {
		switch msg {
		case wmContextMenu:
			ui.mediaContext(hwnd, lp)
			return 0
		case 0x0204:
			return 0
		case 0x0205:
			p := point{int32(int16(lp & 0xffff)), int32(int16(lp >> 16))}
			user.NewProc("ClientToScreen").Call(hwnd, uintptr(unsafe.Pointer(&p)))
			ui.mediaContext(hwnd, uintptr(uint16(p.X))|uintptr(uint16(p.Y))<<16)
			return 0
		case 0x0201:
			if ui.beginAssetDrag(hwnd, lp) {
				return 0
			}
		case 0x0200:
			if ui.assetDrag != nil {
				ui.updateAssetDrag()
				return 0
			}
		case 0x0202:
			if ui.assetDrag != nil {
				ui.finishAssetDrag()
				return 0
			}
		case 0x001f:
			if ui.assetDrag != nil {
				ui.cancelAssetDrag()
			}
		case 0x0215:
			if ui.assetDrag != nil {
				ui.assetDrag = nil
				ui.invalidateTimeline()
			}
		}
	}
	if msg == 0x0082 {
		acceptFiles.Call(hwnd, 0)
		removeSubclass.Call(hwnd, mediaDropCallback, id)
	}
	r, _, _ := defSubclass.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func (e *editor) enableFileDrops() {
	acceptFiles.Call(mainWindow, 1)
	acceptFiles.Call(e.timeline, 1)
	h := e.controls[idAssets]
	if subclassWindow.Find() == nil && removeSubclass.Find() == nil && defSubclass.Find() == nil {
		if mediaDropCallback == 0 {
			mediaDropCallback = syscall.NewCallback(mediaDropProc)
		}
		if ok, _, _ := subclassWindow.Call(h, mediaDropCallback, 1, 0); ok != 0 {
			acceptFiles.Call(h, 1)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "Media-bin drop registration failed; use the timeline or Import video.")
}

func (e *editor) importPaths(paths []string) error { return e.importAt(paths, nil) }

func (e *editor) importAt(paths []string, placement *placementRequest) error {
	if e.menuOpen || e.assetDrag != nil || e.marquee != nil || e.busy || e.drag != nil || e.envelopeDrag != nil || e.scrubbing || e.cropSession != nil || e.transformSession != nil {
		return fmt.Errorf("finish the current operation before importing files")
	}
	if len(paths) == 0 || len(paths) > project.MaxImportFiles {
		return fmt.Errorf("select between 1 and %d files", project.MaxImportFiles)
	}
	snapshot := e.model.Project().Clone()
	engine := e.engine
	disk := e.mediaDisk
	e.tell(fmt.Sprintf("Inspecting %d file(s)...", len(paths)))
	e.startJob(func(ctx context.Context) result {
		batch, err := snapshot.ImportPaths(ctx, paths, func(ctx context.Context, path string) (project.Asset, error) {
			info, err := os.Stat(path)
			if err != nil {
				return project.Asset{}, err
			}
			if !info.Mode().IsRegular() {
				return project.Asset{}, fmt.Errorf("not a regular media file")
			}
			job, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			report, err := engine.CachedImportReport(job, disk, path)
			if err != nil {
				return project.Asset{}, err
			}
			return project.FromReport(nextID("asset"), path, report)
		})
		if err != nil {
			return result{err: err}
		}
		return result{batch: &batch, placement: placement}
	})
	return nil
}

func (e *editor) finishImport(b *project.ImportBatch, req *placementRequest) {
	placed := 0
	placementError := ""
	if req != nil && len(b.AssetIDs) > 0 {
		ids := make([]string, len(b.AssetIDs))
		prefix := nextID("clip")
		for i := range ids {
			ids[i] = fmt.Sprintf("%s-%d", prefix, i)
		}
		p, err := b.Project.PlaceAssets(req.trackID, b.AssetIDs, ids, req.start, req.marker, req.tolerance)
		if err == nil {
			if err = e.commitPlacementOnTrack(req.trackID, p, b.Project.Assets[b.Selected].ID); err != nil {
				e.tell(err.Error())
				return
			}
			placed = len(ids)
			user.NewProc("SetFocus").Call(e.timeline)
		} else {
			placementError = " Placement rejected; imported assets remain in media bin."
		}
	}
	if placed == 0 {
		if b.Added > 0 {
			assetID := ""
			if b.Selected >= 0 && b.Selected < len(b.Project.Assets) {
				assetID = b.Project.Assets[b.Selected].ID
			}
			if err := e.commitWithSelection(b.Project, editorstate.SelectionUpdate{ReplaceAsset: true, AssetID: assetID}); err != nil {
				e.tell(err.Error())
				return
			}
		} else if b.Selected >= 0 {
			e.model.SetAssetIndex(b.Selected)
			send.Call(e.controls[idAssets], lbSet, uintptr(e.model.AssetIndex()), 0)
			e.checkpoint()
		}
	}
	summary := fmt.Sprintf("Imported %d; already in media bin %d; failed %d; placed %d.", b.Added, b.Duplicates, len(b.Failures), placed) + placementError
	e.tell(summary)
	if len(b.Failures) > 0 {
		var details strings.Builder
		details.WriteString(summary)
		for i, f := range b.Failures {
			if i >= 8 {
				fmt.Fprintf(&details, "\n\n%d more failed file(s).", len(b.Failures)-i)
				break
			}
			reason := f.Error
			if len([]rune(reason)) > 300 {
				reason = string([]rune(reason)[:300]) + "..."
			}
			fmt.Fprintf(&details, "\n\n%s\n%s", filepath.Base(f.Path), reason)
		}
		user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(details.String()))), uintptr(unsafe.Pointer(u("PixelCut import results"))), 0x30)
	}
}

func parseImportSelection(buf []uint16) ([]string, error) {
	var parts []string
	start := 0
	for i, v := range buf {
		if v != 0 {
			continue
		}
		if i == start {
			break
		}
		parts = append(parts, syscall.UTF16ToString(buf[start:i]))
		start = i + 1
		if len(parts) > project.MaxImportFiles+1 {
			return nil, fmt.Errorf("select at most %d files", project.MaxImportFiles)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	if len(parts) == 1 {
		if !filepath.IsAbs(parts[0]) {
			return nil, fmt.Errorf("invalid selected file path")
		}
		return parts, nil
	}
	if !filepath.IsAbs(parts[0]) {
		return nil, fmt.Errorf("invalid selected directory")
	}
	paths := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		if name == "." || name == ".." || filepath.Base(name) != name || filepath.VolumeName(name) != "" || filepath.IsAbs(name) {
			return nil, fmt.Errorf("invalid selected filename")
		}
		paths = append(paths, filepath.Join(parts[0], name))
	}
	return paths, nil
}

func pickImportFiles() ([]string, error) {
	buf := make([]uint16, 65536)
	filter := utf16.Encode([]rune("Video files (*.mp4;*.mov)\x00*.mp4;*.mov\x00All files\x00*.*\x00\x00"))
	of := openFileName{Owner: mainWindow, Filter: &filter[0], FilterIndex: 1, File: &buf[0], MaxFile: uint32(len(buf)), Title: u("Import videos"), DefaultExt: u("mp4"), Flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000200 | 0x00000008 | 0x00000004}
	of.Size = uint32(unsafe.Sizeof(of))
	ok, _, _ := dialog.NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&of)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(buf)
	if ok == 0 {
		code, _, _ := dialog.NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return nil, fmt.Errorf("file dialog error 0x%x", code)
		}
		return nil, nil
	}
	return parseImportSelection(buf)
}
