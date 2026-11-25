package main

import (
	"fmt"

	"github.com/markrai/pixelcut/internal/project"
)

func (e *editor) copySelection(cut bool) error {
	ids := e.selectionIDs()
	b, err := e.p.CopyClips(e.trackID(), ids)
	if err != nil {
		return err
	}
	if cut {
		q, err := e.p.DeleteClips(e.trackID(), ids)
		if err != nil {
			return err
		}
		if err = e.commitRipple(q, true); err != nil {
			return err
		}
		e.clipboard = b
		e.tell(fmt.Sprintf("Cut %d clip(s). Ctrl+V to paste.", b.Count()))
		return nil
	}
	e.clipboard = b
	e.tell(fmt.Sprintf("Copied %d clip(s). Ctrl+V to paste.", b.Count()))
	return nil
}

func (e *editor) pasteSelection(duplicate bool) error { return e.pasteAt(duplicate, nil) }

func (e *editor) pasteAt(duplicate bool, position *int64) error {
	b := e.clipboard
	if duplicate {
		var err error
		b, err = e.p.CopyClips(e.trackID(), e.selectionIDs())
		if err != nil {
			return err
		}
	}
	if b.Count() == 0 {
		return fmt.Errorf("clip clipboard is empty")
	}
	e.updatePlaybackClock()
	prefix := nextID("clip")
	ids := make([]string, b.Count())
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	var r project.ClipboardPlacement
	var err error
	if duplicate {
		r, err = e.p.DuplicateClips(e.trackID(), b, ids)
	} else {
		at := e.playhead
		if position != nil {
			at = *position
		}
		r, err = e.p.PasteClips(e.trackID(), b, at, ids)
	}
	if err != nil {
		return fmt.Errorf("placement rejected: %w", err)
	}
	if err = r.Project.Validate(); err != nil {
		return err
	}
	e.stopPreview()
	e.group = nil
	if len(ids) > 1 {
		e.group = make(map[string]bool, len(ids))
		for _, id := range ids {
			e.group[id] = true
		}
	}
	e.clip = r.FirstClip
	e.anchor = ids[0]
	_, e.zoom = timelineEditView(e.playhead, e.playhead, e.zoom, e.scale(clientRect(e.timeline).Right))
	if err = e.commit(r.Project); err != nil {
		return err
	}
	user.NewProc("SetFocus").Call(e.timeline)
	verb := "Pasted"
	if duplicate {
		verb = "Duplicated"
	}
	e.tell(fmt.Sprintf("%s %d clip(s) at %.3fs.", verb, len(ids), project.Seconds(r.Start)))
	return nil
}
