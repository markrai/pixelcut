package main

import (
	"fmt"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func (e *editor) commitRipple(q project.Project, clearSelection bool) error {
	if err := q.Validate(); err != nil {
		return err
	}
	end := q.EndTime()
	playhead, zoom := timelineEditView(e.playhead, end, e.zoom, e.scale(clientRect(e.timeline).Right))
	return e.commitEdit(q, editorstate.SelectionUpdate{ReplaceClips: clearSelection}, &editView{playhead: playhead, zoom: zoom})
}

func (e *editor) rippleDeleteSelection() error {
	ids := e.selectionIDs()
	q, err := e.model.Project().RippleDelete(e.trackID(), ids)
	if err != nil {
		return err
	}
	if err = e.commitRipple(q, true); err != nil {
		return err
	}
	e.tell(fmt.Sprintf("Ripple deleted %d clip(s); source files unchanged.", len(ids)))
	return nil
}

func (e *editor) removeGap() error { return e.removeGapAt(e.playhead) }

func (e *editor) removeGapAt(at int64) error {
	q, duration, err := e.model.Project().RemoveGap(e.trackID(), at)
	if err != nil {
		return err
	}
	if err = e.commitRipple(q, false); err != nil {
		return err
	}
	e.tell(fmt.Sprintf("Removed %.3fs gap.", project.Seconds(duration)))
	return nil
}

func (e *editor) updateRippleButton() {
	label := "Ripple: off"
	if e.rippleTrim {
		label = "Ripple: on"
	}
	h := e.controls[idRippleTrim]
	change(h, label)
	user.NewProc("InvalidateRect").Call(h, 0, 1)
	e.invalidateTimeline()
}
