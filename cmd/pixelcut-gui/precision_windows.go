package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

type frameCacheEntry struct {
	index    *media.FrameIndex
	size     int64
	modified time.Time
}

type frameJobResult struct {
	path, clipID string
	entry        frameCacheEntry
	action       int
}

func (e *editor) cachedFrames(a project.Asset) *media.FrameIndex {
	info, err := os.Stat(a.Path)
	if err != nil {
		return nil
	}
	v, ok := e.frameCache[a.Path]
	if !ok || v.size != info.Size() || !v.modified.Equal(info.ModTime()) {
		return nil
	}
	return v.index
}

func (e *editor) loadFrames(a project.Asset, clipID string, action int) error {
	if e.busy {
		return fmt.Errorf("media operation in progress")
	}
	info, err := os.Stat(a.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	e.stopPreview()
	engine := e.engine
	disk := e.mediaDisk
	size, modified := info.Size(), info.ModTime()
	e.tell("Indexing actual video frame timestamps...")
	e.startJob(func(ctx context.Context) result {
		job, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		index, err := engine.CachedFrames(job, disk, a.Path)
		if err != nil {
			return result{err: fmt.Errorf("frame indexing: %w", err)}
		}
		after, err := os.Stat(a.Path)
		if err != nil || after.Size() != size || !after.ModTime().Equal(modified) {
			return result{err: fmt.Errorf("source changed during frame indexing")}
		}
		return result{frames: &frameJobResult{path: a.Path, clipID: clipID, entry: frameCacheEntry{index, size, modified}, action: action}}
	})
	return nil
}

func (e *editor) finishFrames(r *frameJobResult) {
	if e.frameCache == nil {
		e.frameCache = map[string]frameCacheEntry{}
	}
	if len(e.frameCache) >= 4 {
		for path := range e.frameCache {
			if path != r.path {
				delete(e.frameCache, path)
				break
			}
		}
	}
	e.frameCache[r.path] = r.entry
	if r.action == 0 {
		e.tell(fmt.Sprintf("Indexed %d frames. Drag the trim edge again.", r.entry.index.Count()))
		return
	}
	c, ok := e.selected()
	if !ok || c.ID != r.clipID {
		e.tell("Frame index ready.")
		return
	}
	if err := e.precisionAction(r.action); err != nil {
		e.tell(err.Error())
	}
}

func (e *editor) precisionAction(action int) error {
	if e.model.Count() > 1 {
		return fmt.Errorf("select one clip for precision editing")
	}
	c, ok := e.selected()
	if !ok {
		return fmt.Errorf("select a clip")
	}
	e.updatePlaybackClock()
	end := c.Start + (c.Out - c.In)
	if e.playhead < c.Start || e.playhead > end {
		return fmt.Errorf("position the playhead inside the selected clip")
	}
	a, ok := e.model.Project().Asset(c.AssetID)
	if !ok {
		return fmt.Errorf("source asset missing")
	}
	f := e.cachedFrames(a)
	if f == nil {
		return e.loadFrames(a, c.ID, action)
	}
	source := c.In + (e.playhead - c.Start)
	if action == idFrameBack || action == idFrameForward {
		t, ok := f.Step(source, c.In, c.Out, action == idFrameForward)
		if !ok {
			return fmt.Errorf("no further frame inside this clip; use Ctrl+arrow to jump edit boundaries")
		}
		e.seekTimeline(c.Start+(t-c.In), false)
		e.tell(fmt.Sprintf("Frame at %s (source %s).", precisionStamp(c.Start+t-c.In), precisionStamp(t)))
		return nil
	}
	if action != idSetIn && action != idSetOut {
		return fmt.Errorf("unknown precision operation")
	}
	mode := "left"
	if action == idSetOut {
		mode = "right"
	}
	if (mode == "left" && source >= c.Out) || (mode == "right" && source <= c.In) {
		return fmt.Errorf("trim must retain a positive source interval")
	}
	var lo, hi, base int64
	if mode == "left" {
		lo, hi, base = c.In, c.Out-1, c.In
	} else {
		lo, hi, base = c.In+1, c.Out, c.Out
	}
	target, ok := f.Nearest(source, lo, hi)
	if mode == "right" && c.Out == a.Duration && (!ok || c.Out-source < absTicks(target-source)) {
		target, ok = c.Out, true
	}
	if !ok {
		return fmt.Errorf("no valid source frame boundary at playhead")
	}
	delta := target - base
	if delta == 0 {
		return nil
	}
	var q project.Project
	if e.rippleTrim {
		r, err := e.model.Project().RippleTrim(e.trackID(), c.ID, mode, delta, -1, 0)
		if err != nil {
			return err
		}
		if r.Delta != delta {
			return fmt.Errorf("frame boundary exceeds ripple limits")
		}
		q = r.Project
	} else {
		trimmed, err := e.model.Project().DragClip(e.trackID(), c.ID, mode, delta)
		if err != nil {
			return err
		}
		if (mode == "left" && trimmed.In != c.In+delta) || (mode == "right" && trimmed.Out != c.Out+delta) {
			return fmt.Errorf("frame boundary exceeds trim limits")
		}
		q = e.model.Project().Clone()
		q.Tracks[e.trackIndex()].Clips[e.model.ClipIndex()] = trimmed
	}
	if err := e.commitRipple(q, false); err != nil {
		return err
	}
	e.tell("Source " + mode + " boundary set to a decoded frame timestamp.")
	return nil
}

func absTicks(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (e *editor) jumpEdit(forward bool) {
	e.updatePlaybackClock()
	at, ok := editBoundary(e.track().Clips, e.playhead, forward)
	if !ok {
		return
	}
	if at == e.endTime() {
		e.stopPreview()
		e.playhead = at
		e.refreshPlaybackMarker()
		e.checkpoint()
	} else {
		e.seekTimeline(at, false)
	}
}

func editBoundary(clips []project.Clip, at int64, forward bool) (int64, bool) {
	return timeline.EditBoundary(clips, at, forward)
}

var playbackSpeeds = []float64{.25, .5, 1, 1.5, 2}

func (e *editor) speed() float64 {
	if e.playbackSpeed == 0 {
		return 1
	}
	return e.playbackSpeed
}

func (e *editor) changeSpeed(direction int, cycle bool) {
	if e.stillActive {
		e.tell("Composite playback currently uses a fixed 10 fps target at nominal 1x; source preview speed is unchanged.")
		return
	}
	i := 2
	for j, v := range playbackSpeeds {
		if v == e.speed() {
			i = j
			break
		}
	}
	if cycle {
		i = (i + 1) % len(playbackSpeeds)
	} else {
		i = max(0, min(len(playbackSpeeds)-1, i+direction))
	}
	e.playbackSpeed = playbackSpeeds[i]
	if e.clock != nil {
		e.clock.setSpeed(e.playbackSpeed)
	}
	h := e.controls[idSpeed]
	change(h, fmt.Sprintf("Speed: %gx", e.speed()))
	user.NewProc("InvalidateRect").Call(h, 0, 1)
	e.tell(fmt.Sprintf("Preview speed: %gx. Export speed unchanged.", e.speed()) + e.rememberPreferences())
}

func (e *editor) toggleFrameSnap() {
	e.frameSnap = !e.frameSnap
	label := "Frames: off"
	if e.frameSnap {
		label = "Frames: on"
	}
	h := e.controls[idFrameSnap]
	change(h, label)
	user.NewProc("InvalidateRect").Call(h, 0, 1)
	e.tell("Source-frame trim snapping " + label[8:] + ". Shift during drag bypasses snapping." + e.rememberPreferences())
}

func precisionStamp(t int64) string {
	t = max(0, t)
	s := t / project.TicksPerSecond
	return fmt.Sprintf("%02d:%02d:%02d.%06d", s/3600, (s/60)%60, s%60, t%project.TicksPerSecond)
}

func rulerStep(scale float64) int64 { return timeline.RulerStep(scale) }

func rulerLabel(t, step int64) string { return timeline.RulerLabel(t, step) }
