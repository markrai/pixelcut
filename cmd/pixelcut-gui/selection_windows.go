package main

import (
	"fmt"
	"strings"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

type marqueeDrag struct {
	x, y, currentX, currentY int32
	before                   []string
	primary                  string
	additive, moved          bool
	clickTime                int64
	wasPlaying               bool
}

func (e *editor) selectionIDs() []string {
	return e.model.IDs()
}

func (e *editor) clipSelected(_ int, c project.Clip) bool {
	return e.model.Contains(c.ID)
}

func (e *editor) setSelection(ids []string, primary string) {
	e.model.Select(ids, primary)
	e.syncSelectionUI()
}

func (e *editor) syncSelectionUI() {
	send.Call(e.controls[idClips], lbSet, uintptr(e.model.ClipIndex()), 0)
	e.fields()
	e.invalidateTimeline()
}

func (e *editor) selectionKey() string {
	return strings.Join(e.selectionIDs(), "\x00")
}

func (e *editor) selectAllClips() {
	e.stopPreview()
	e.model.SelectAll()
	e.syncSelectionUI()
	e.checkpoint()
	e.tell(fmt.Sprintf("%d clip(s) selected.", e.model.Count()))
}

func (e *editor) modifySelection(i int, ctrl, shift bool) {
	c := e.track().Clips[i]
	e.stopPreview()
	e.model.Modify(c.ID, ctrl, shift)
	e.syncSelectionUI()
	e.checkpoint()
	e.tell(fmt.Sprintf("%d clip(s) selected.", e.model.Count()))
}

func (e *editor) beginMarquee(x, y int32, t int64, playing bool) {
	e.marquee = &marqueeDrag{x: x, y: y, currentX: x, currentY: y, before: e.selectionIDs(), primary: e.model.PrimaryID(), additive: keyPressed(0x11), clickTime: t, wasPlaying: playing}
	user.NewProc("SetCapture").Call(e.timeline)
}

func (e *editor) moveMarquee(x, y, w int32) {
	d := e.marquee
	if d == nil {
		return
	}
	if !d.moved && !timeline.DragStarted(x-d.x, y-d.y) {
		return
	}
	if !d.moved {
		e.stopPreview()
	}
	d.moved = true
	d.currentX, d.currentY = x, y
	left, right := min(d.x, x), max(d.x, x)
	top, bottom := min(d.y, y), max(d.y, y)
	ids := make([]string, 0)
	if d.additive {
		ids = append(ids, d.before...)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	view := e.timelineView(w)
	for _, c := range e.track().Clips {
		geometry, visible := e.clipGeometry(view, c)
		if visible && geometry.Intersects(left, top, right, bottom) && !seen[c.ID] {
			ids = append(ids, c.ID)
			seen[c.ID] = true
		}
	}
	e.setSelection(ids, "")
}

func (e *editor) endMarquee(cancel bool) {
	d := e.marquee
	if d == nil {
		return
	}
	e.marquee = nil
	user.NewProc("ReleaseCapture").Call()
	if cancel {
		e.setSelection(d.before, d.primary)
	} else if !d.moved {
		if !d.additive {
			e.selectClip(-1)
			e.seekTimeline(d.clickTime, d.wasPlaying)
		}
	} else {
		e.model.SetAnchor(e.model.PrimaryID())
		e.checkpoint()
		e.tell(fmt.Sprintf("%d clip(s) selected.", len(e.selectionIDs())))
	}
	e.invalidateTimeline()
}

func (e *editor) paintMarquee(dc uintptr, w, h int32) {
	d := e.marquee
	if d == nil || !d.moved {
		return
	}
	l, r := max(trackHeader, min(d.x, d.currentX)), min(w, max(d.x, d.currentX))
	t, b := max(timeline.TrackTop, min(d.y, d.currentY)), min(h, max(d.y, d.currentY))
	if r <= l || b <= t {
		return
	}
	for x := l; x < r; x += 6 {
		fill(dc, rect{x, t, min(x+3, r), t + 1}, palette.accent)
		fill(dc, rect{x, b - 1, min(x+3, r), b}, palette.accent)
	}
	for y := t; y < b; y += 6 {
		fill(dc, rect{l, y, l + 1, min(y+3, b)}, palette.accent)
		fill(dc, rect{r - 1, y, r, min(y+3, b)}, palette.accent)
	}
}

func (e *editor) deleteSelection() error {
	ids := e.selectionIDs()
	q, err := e.model.Project().DeleteClips(e.trackID(), ids)
	if err != nil {
		return err
	}
	view := editView{playhead: min(e.playhead, q.EndTime()), zoom: e.zoom}
	if err = e.commitEdit(q, editorstate.SelectionUpdate{ReplaceClips: true}, &view); err != nil {
		return err
	}
	e.tell(fmt.Sprintf("Removed %d clip(s); sources unchanged.", len(ids)))
	return nil
}
