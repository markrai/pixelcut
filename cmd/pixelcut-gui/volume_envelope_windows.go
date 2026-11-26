package main

import (
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

type volumeDrag struct {
	ref         project.ClipRef
	fingerprint string
	clip        project.Clip
	points      []project.VolumeKeyframe
	index       int
	view        timeline.View
	top, bottom int32
	x, y        int32
}

func volumeSource(c project.Clip, view timeline.View, x int32) int64 {
	return c.In + max(0, min(c.Duration(), view.TimeAt(x)-c.Start))
}

func volumeY(percent int, top, bottom int32) int32 {
	return bottom - int32(math.Round(float64(bottom-top)*float64(percent)/100))
}

func volumePercent(y, top, bottom int32) int {
	return int(math.Round(float64(bottom-max(top, min(bottom, y))) * 100 / float64(bottom-top)))
}

func volumePointX(c project.Clip, v timeline.View, at int64) int32 {
	return v.XAt(c.Start + (at - c.In))
}

func volumeHit(c project.Clip, v timeline.View, points []project.VolumeKeyframe, x, y, top, bottom int32) int {
	best, distance := -1, int64(50)
	for i, p := range points {
		if p.SourceTime < c.In || p.SourceTime > c.Out {
			continue
		}
		px := volumePointX(c, v, p.SourceTime)
		if px < trackHeader || px > v.Width {
			continue
		}
		dx, dy := int64(x-px), int64(y-volumeY(p.Percent, top, bottom))
		d := dx*dx + dy*dy
		if d < distance {
			best, distance = i, d
		}
	}
	return best
}

func insertVolumePoint(c project.Clip, points []project.VolumeKeyframe, at int64, percent int) ([]project.VolumeKeyframe, int, bool) {
	out := slices.Clone(points)
	if len(out) == 0 {
		out = []project.VolumeKeyframe{{SourceTime: c.In, Percent: 100}, {SourceTime: c.Out, Percent: 100}}
	}
	i := sort.Search(len(out), func(i int) bool { return out[i].SourceTime >= at })
	if i < len(out) && out[i].SourceTime == at {
		out[i].Percent = percent
		return out, i, true
	}
	if len(out) >= project.MaxVolumeKeyframes {
		return nil, 0, false
	}
	out = slices.Insert(out, i, project.VolumeKeyframe{SourceTime: at, Percent: percent})
	return out, i, true
}

func moveVolumePoint(c project.Clip, points []project.VolumeKeyframe, index int, at int64, percent int) {
	lo, hi := c.In, c.Out
	if index > 0 {
		lo = max(lo, points[index-1].SourceTime+1)
	}
	if index+1 < len(points) {
		hi = min(hi, points[index+1].SourceTime-1)
	}
	if lo <= hi {
		points[index] = project.VolumeKeyframe{SourceTime: max(lo, min(hi, at)), Percent: max(0, min(100, percent))}
	}
}

func (e *editor) beginEnvelopeDrag(x, y int32, view timeline.View, lane timeline.Lane) bool {
	p := e.model.Project()
	ti := p.TrackIndex(lane.TrackID)
	if ti < 0 {
		return false
	}
	top, bottom := timeline.AudioTop+4, timeline.AudioBottom-5
	if lane.Kind == project.AudioTrack {
		top, bottom = laneTop+22, laneBottom-5
	}
	top, bottom = top+lane.Offset(), bottom+lane.Offset()
	if y < top-4 || y > bottom+4 {
		return false
	}
	for i, c := range p.Tracks[ti].Clips {
		g, visible := view.Clip(c, false)
		if !visible || x < g.Left || x >= g.Right {
			continue
		}
		ref := project.ClipRef{TrackID: lane.TrackID, ClipID: c.ID}
		if !clipAudioAvailable(p, ref) {
			e.tell("Choose an editable clip with audio.")
			return true
		}
		var points []project.VolumeKeyframe
		if c.Envelope != nil {
			points = slices.Clone(c.Envelope.Points)
		}
		index := volumeHit(c, view, points, x, y, top, bottom)
		e.stopPreview()
		e.selectClip(i)
		if keyPressed(0x10) {
			if index >= 0 {
				q, err := p.SetClipEnvelope(ref, slices.Delete(points, index, index+1))
				if err == nil {
					err = e.commit(q)
				}
				if err != nil {
					e.tell(err.Error())
				}
			}
			return true
		}
		if index < 0 {
			var ok bool
			points, index, ok = insertVolumePoint(c, points, volumeSource(c, view, x), volumePercent(y, top, bottom))
			if !ok {
				e.tell("Volume envelope keyframe limit reached.")
				return true
			}
		}
		e.envelopeDrag = &volumeDrag{ref: ref, fingerprint: p.Fingerprint(), clip: c, points: points, index: index, view: view, top: top, bottom: bottom, x: x, y: y}
		user.NewProc("SetCapture").Call(e.timeline)
		if owner, _, _ := user.NewProc("GetCapture").Call(); owner != e.timeline {
			e.envelopeDrag = nil
			e.tell("Could not capture timeline pointer.")
		}
		e.invalidateTimeline()
		return true
	}
	return false
}

func (e *editor) moveEnvelopeDrag(x, y int32) {
	d := e.envelopeDrag
	if d == nil {
		return
	}
	if x == d.x && y == d.y {
		return
	}
	moveVolumePoint(d.clip, d.points, d.index, volumeSource(d.clip, d.view, x), volumePercent(y, d.top, d.bottom))
	d.x, d.y = x, y
	e.tell(fmt.Sprintf("Volume keyframe: %d%% (relative to clip gain)", d.points[d.index].Percent))
	e.invalidateTimeline()
}

func (e *editor) endEnvelopeDrag(cancel bool) {
	d := e.envelopeDrag
	if d == nil {
		return
	}
	e.envelopeDrag = nil
	user.NewProc("ReleaseCapture").Call()
	defer e.invalidateTimeline()
	if cancel {
		e.tell("Volume envelope edit canceled.")
		return
	}
	if e.model.Fingerprint() != d.fingerprint || e.trackID() != d.ref.TrackID || e.model.PrimaryID() != d.ref.ClipID {
		e.tell("Volume envelope edit canceled: selection or project changed.")
		return
	}
	q, err := e.model.Project().SetClipEnvelope(d.ref, d.points)
	if err == nil {
		err = e.commit(q)
	}
	if err != nil {
		e.tell(err.Error())
		return
	}
	e.tell("Volume envelope applied. Use composite playback to hear it.")
}

func (e *editor) paintVolumeEnvelope(dc uintptr, view timeline.View, track string, c project.Clip, span timeline.Span, top, bottom int32) {
	env := c.Envelope
	if d := e.envelopeDrag; d != nil && d.ref.TrackID == track && d.ref.ClipID == c.ID {
		env = &project.VolumeEnvelope{Points: d.points}
	}
	if env == nil || span.Right-span.Left < 2 {
		return
	}
	top, bottom = top+4, bottom-5
	pen, _, _ := gdi.NewProc("CreatePen").Call(0, 1, palette.accent)
	if pen == 0 {
		return
	}
	old, _, _ := gdi.NewProc("SelectObject").Call(dc, pen)
	defer gdi.NewProc("DeleteObject").Call(pen)
	defer gdi.NewProc("SelectObject").Call(dc, old)
	left, right := span.Left, span.Right-1
	yAt := func(at int64) int32 {
		return bottom - int32((int64(bottom-top)*env.Factor(at)+project.EnvelopeScale/2)/project.EnvelopeScale)
	}
	gdi.NewProc("MoveToEx").Call(dc, uintptr(left), uintptr(yAt(volumeSource(c, view, left))), 0)
	for _, k := range env.Points {
		if k.SourceTime < c.In || k.SourceTime > c.Out {
			continue
		}
		x := volumePointX(c, view, k.SourceTime)
		if x <= left || x >= right {
			continue
		}
		gdi.NewProc("LineTo").Call(dc, uintptr(x), uintptr(volumeY(k.Percent, top, bottom)))
	}
	gdi.NewProc("LineTo").Call(dc, uintptr(right), uintptr(yAt(volumeSource(c, view, right))))
	for _, k := range env.Points {
		if k.SourceTime < c.In || k.SourceTime > c.Out {
			continue
		}
		x := volumePointX(c, view, k.SourceTime)
		if x < left || x > right+1 {
			continue
		}
		x = min(right, x)
		y := volumeY(k.Percent, top, bottom)
		fill(dc, rect{max(left, x-3), y - 3, min(right+1, x+4), y + 4}, palette.accent)
	}
}
