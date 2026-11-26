package main

import (
	"fmt"
	"path/filepath"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

type placementRequest struct {
	start, marker, tolerance int64
	trackID                  string
}

type assetDrag struct {
	id, clipID, trackID string
	origin              point
	moved, over, valid  bool
	placement           project.AssetPlacement
}

func (e *editor) placementAt(x, y int32) (*placementRequest, bool) {
	r := clientRect(e.timeline)
	view := e.timelineView(r.Right)
	lane, ok := e.timelineStack().At(y)
	if !ok || !lane.MainContains(y) || x < trackHeader || x >= r.Right {
		return nil, false
	}
	tolerance := timeline.SnapTolerance(view.Scale(), !e.snapDisabled && !keyPressed(0x10))
	return &placementRequest{start: view.TimeAt(x), marker: e.playhead, tolerance: tolerance, trackID: lane.TrackID}, true
}

func (e *editor) beginAssetDrag(hwnd uintptr, lp uintptr) bool {
	if e.menuOpen || e.busy || e.marquee != nil || e.drag != nil || e.envelopeDrag != nil || e.scrubbing || e.cropSession != nil || e.transformSession != nil {
		return false
	}
	r, _, _ := send.Call(hwnd, 0x01a9, 0, lp)
	index := int(r & 0xffff)
	if (r>>16)&0xffff != 0 || index >= len(e.model.Project().Assets) {
		return false
	}
	var item rect
	ok, _, _ := send.Call(hwnd, 0x0198, uintptr(index), uintptr(unsafe.Pointer(&item)))
	y := int32(int16(lp >> 16))
	if int32(ok) < 0 || y < item.Top || y >= item.Bottom {
		return false
	}
	var p point
	if ok, _, _ := user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		return false
	}
	user.NewProc("SetFocus").Call(hwnd)
	send.Call(hwnd, lbSet, uintptr(index), 0)
	e.model.SetAssetIndex(index)
	e.assetDrag = &assetDrag{id: e.model.Project().Assets[index].ID, clipID: nextID("clip"), origin: p}
	user.NewProc("SetCapture").Call(hwnd)
	return true
}

func (e *editor) updateAssetDrag() {
	d := e.assetDrag
	if d == nil {
		return
	}
	var p point
	ok, _, _ := user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	if ok == 0 {
		return
	}
	if !d.moved && !timeline.DragStarted(p.X-d.origin.X, p.Y-d.origin.Y) {
		return
	}
	if !d.moved {
		e.stopPreview()
	}
	d.moved = true
	user.NewProc("ScreenToClient").Call(e.timeline, uintptr(unsafe.Pointer(&p)))
	req, over := e.placementAt(p.X, p.Y)
	d.over, d.valid = over, false
	if over {
		var err error
		d.trackID = req.trackID
		d.placement, err = e.model.Project().PlaceAssets(req.trackID, []string{d.id}, []string{d.clipID}, req.start, req.marker, req.tolerance)
		d.valid = err == nil
	}
	cursor := cursorNo
	if d.valid {
		cursor = cursorSizeAll
	}
	h, _, _ := user.NewProc("LoadCursorW").Call(0, cursor)
	if h != 0 {
		user.NewProc("SetCursor").Call(h)
	}
	e.invalidateTimeline()
}

func (e *editor) cancelAssetDrag() {
	if e.assetDrag == nil {
		return
	}
	e.assetDrag = nil
	user.NewProc("ReleaseCapture").Call()
	e.invalidateTimeline()
}

func (e *editor) finishAssetDrag() {
	e.updateAssetDrag()
	d := e.assetDrag
	if d == nil {
		return
	}
	e.cancelAssetDrag()
	if !d.moved {
		e.checkpoint()
		return
	}
	if !d.over {
		return
	}
	if !d.valid {
		e.tell("Cannot place asset here: check track compatibility and overlapping clips.")
		return
	}
	if err := e.commitPlacementOnTrack(d.trackID, d.placement); err != nil {
		e.tell(err.Error())
		return
	}
	user.NewProc("SetFocus").Call(e.timeline)
	e.tell(placementStatus(*e.track()))
}

func (e *editor) commitPlacement(p project.AssetPlacement, selectedAsset ...string) error {
	return e.commitPlacementOnTrack(e.trackID(), p, selectedAsset...)
}

func (e *editor) commitPlacementOnTrack(trackID string, p project.AssetPlacement, selectedAsset ...string) error {
	ti := p.Project.TrackIndex(trackID)
	if ti < 0 || p.FirstClip < 0 || p.FirstClip >= len(p.Project.Tracks[ti].Clips) {
		return fmt.Errorf("invalid placement selection")
	}
	id := p.Project.Tracks[ti].Clips[p.FirstClip].ID
	update := editorstate.SelectionUpdate{TrackID: trackID, ReplaceClips: true, ClipIDs: []string{id}, Primary: id, Anchor: id}
	if len(selectedAsset) > 0 {
		update.ReplaceAsset, update.AssetID = true, selectedAsset[0]
	}
	return e.commitWithSelection(p.Project, update)
}

func (e *editor) paintAssetGhost(dc uintptr, w int32) {
	d := e.assetDrag
	if d == nil || !d.moved || !d.over {
		return
	}
	stack := e.timelineStack()
	lane, ok := stack.Find(d.trackID)
	if !ok {
		return
	}
	span, visible := e.timelineView(w).Span(d.placement.Start, d.placement.End)
	if !visible {
		return
	}
	left, right := span.Left, span.Right
	a, ok := e.model.Project().Asset(d.id)
	if !ok {
		return
	}
	col := rgb(112, 220, 241)
	if !d.valid {
		col = rgb(242, 105, 112)
	}
	saved := beginLanePaint(dc, lane, w, stack.Bottom)
	if saved == 0 {
		return
	}
	r := rect{left, laneTop, right, laneBottom}
	h, _, _ := gdi.NewProc("CreateHatchBrush").Call(5, col)
	if h != 0 {
		gdi.NewProc("SetBkMode").Call(dc, 1)
		user.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), h)
		gdi.NewProc("DeleteObject").Call(h)
	}
	fill(dc, rect{left, laneTop, right, laneTop + 2}, col)
	fill(dc, rect{left, laneBottom - 2, right, laneBottom}, col)
	fill(dc, rect{left, laneTop, min(left+2, right), laneBottom}, col)
	fill(dc, rect{max(left, right-2), laneTop, right, laneBottom}, col)
	if right-left > 25 {
		name := fmt.Sprintf("%s  %.2fs", filepath.Base(a.Path), project.Seconds(a.Duration))
		if lane.Kind == project.AudioTrack {
			name = "Audio only: " + name
		}
		fill(dc, rect{left + 2, laneTop + 3, right - 2, laneTop + 22}, palette.panel)
		textAt(dc, rect{left + 5, laneTop + 3, right - 5, laneTop + 22}, name, col, 4)
	}
	gdi.NewProc("RestoreDC").Call(dc, saved)
	if d.valid && d.placement.Snapped {
		x := e.xAt(d.placement.Target, w)
		if x >= trackHeader && x < w {
			fill(dc, rect{x, 28, x + 2, stack.Bottom}, col)
		}
	}
}
