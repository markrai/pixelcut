package main

import (
	"math"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/project"
)

const playbackTimer uintptr = 1

func (e *editor) previewTarget(at int64, advance bool) (int, int64, bool, error) {
	fingerprint := e.model.Fingerprint()
	if e.composition == nil || e.compositionFingerprint != fingerprint {
		compiled, err := composition.Compile(e.model.Project())
		if err != nil {
			return -1, at, false, err
		}
		e.composition, e.compositionFingerprint = compiled, fingerprint
	}
	ref, target, ok := e.composition.AuditionTarget(e.trackID(), at, advance)
	if !ok {
		return -1, at, false, nil
	}
	_, index, ok := e.model.Project().ClipIndex(ref)
	if !ok {
		return -1, at, false, project.ErrClipNotFound
	}
	return index, target, true, nil
}

func (e *editor) refreshPlaybackMarker() {
	w := clientRect(e.timeline).Right
	if !e.scrubbing && e.playhead < e.origin {
		e.origin = e.playhead
	}
	if !e.scrubbing && e.xAt(e.playhead, w) >= w-12 {
		visible := math.Max(0, float64(w-trackHeader-24)) / e.scale(w) * float64(project.TicksPerSecond)
		if visible < float64(e.playhead) {
			e.origin = e.playhead - int64(visible)
		} else {
			e.origin = 0
		}
	}
	focus, _, _ := user.NewProc("GetFocus").Call()
	if focus != e.controls[idAt] {
		change(e.controls[idAt], fmtTime(e.playhead))
	}
	e.invalidateTimeline()
}

func (e *editor) updatePlayButton(paused bool) {
	label := "Stop  (Space)"
	if paused {
		label = "Play  (Space)"
	}
	h := e.controls[idPlay]
	if h != 0 && read(h) != label {
		change(h, label)
		user.NewProc("InvalidateRect").Call(h, 0, 1)
	}
}

func (e *editor) updatePlaybackClock() {
	if e.clock == nil {
		return
	}
	if e.clock.unavailable.Swap(false) {
		if e.previewCancel != nil {
			e.previewCancel()
		}
		e.updatePlayButton(true)
		e.tell("Playback IPC unavailable; reopen the preview to seek.")
	}
	if e.scrubbing || e.drag != nil {
		return
	}
	if s := e.clock.transport.Status(); s.Confirmed {
		e.updatePlayButton(s.Paused)
		if s.Tick != e.playhead {
			e.playhead = s.Tick
			e.refreshPlaybackMarker()
		}
	}
}

func (e *editor) pausePreview() error {
	if e.compositeRun != nil {
		e.pauseCompositePlayback()
		return nil
	}
	if e.clock != nil && e.previewActive() && e.clock.transport.Usable() {
		e.clock.queue(e.playhead, false, true)
		e.updatePlayButton(true)
		e.tell("Preview paused.")
	}
	return nil
}

func (e *editor) playing() bool {
	return e.compositeRun != nil || (e.clock != nil && e.previewActive() && !e.clock.paused())
}

func (e *editor) seekTimeline(t int64, playing bool) {
	end := e.endTime()
	if t < 0 {
		t = 0
	}
	if t > end {
		t = end
	}
	if e.stillActive {
		e.playhead = t
		e.refreshPlaybackMarker()
		if err := e.showCompositeStill(); err != nil {
			e.tell(err.Error())
		}
		return
	}
	index := -1
	if t == end {
		for i, c := range e.track().Clips {
			if c.Contains(t) || (t == end && t == c.End()) {
				index = i
				break
			}
		}
	} else {
		var err error
		index, _, _, err = e.previewTarget(t, false)
		if err != nil {
			e.tell(err.Error())
			return
		}
	}
	if index < 0 {
		e.stopPreview()
		e.playhead = t
		for i, c := range e.track().Clips {
			if c.Contains(t) {
				index = i
				break
			}
		}
		e.selectClip(index)
		e.refreshPlaybackMarker()
		e.tell("No enabled video or audio at this position on the active track.")
		return
	}
	c := e.track().Clips[index]
	if a, ok := e.model.Project().Asset(c.AssetID); !ok || !project.SourceAvailable(a.Path) {
		e.stopPreview()
		e.playhead = t
		e.selectClip(index)
		e.refreshPlaybackMarker()
		e.tell("Source offline; use Relink. Timeline edits retained.")
		return
	}
	if t == end {
		e.stopPreview()
		e.playhead = t
		e.selectClip(index)
		e.refreshPlaybackMarker()
		e.tell("End of timeline.")
		return
	}
	if e.clock != nil && e.previewActive() && e.clock.transport.Usable() && e.clock.clip.ID == c.ID {
		e.playhead = t
		if e.model.ClipIndex() != index {
			e.selectClip(index)
		}
		e.clock.queue(t, true, !playing)
		e.updatePlayButton(!playing)
		e.refreshPlaybackMarker()
		return
	}
	e.stopPreview()
	e.playhead = t
	e.selectClip(index)
	if err := e.startPreview(!playing); err != nil {
		e.tell(err.Error())
	}
}
