package main

import (
	"fmt"
	"math"
	"path/filepath"
	"syscall"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

const trackHeader = timeline.Header
const laneTop = timeline.LaneTop
const laneBottom = timeline.LaneBottom

type paintStruct struct {
	DC              uintptr
	Erase           int32
	Rect            rect
	Restore, Update int32
	Reserved        [32]byte
}

type timelineDrag struct {
	index               int
	trackID, targetID   string
	y                   int32
	mode                string
	x                   int32
	scale               float64
	original, candidate project.Clip
	moved, valid        bool
	wasPlaying          bool
	clickTime           int64
	snapped             bool
	snapTarget          int64
	groupIDs            []string
	groupProject        project.Project
	ripple              bool
	rippleDelta         int64
	frames              *media.FrameIndex
	sourceDuration      int64
}

func registerTimeline() {
	name := u("PixelCutTimeline")
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	wc := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: syscall.NewCallback(timelineProc), Instance: instance, Cursor: cursor, Name: name}
	if n, _, err := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); n == 0 {
		panic(err)
	}
}

func (e *editor) invalidateTimeline() {
	if e.timeline != 0 {
		user.NewProc("InvalidateRect").Call(e.timeline, 0, 0)
	}
}

func (e *editor) endTime() int64 { return e.model.Project().EndTime() }

func (e *editor) timelineView(w int32) timeline.View {
	return timeline.View{Width: w, Origin: e.origin, End: e.endTime(), Zoom: e.zoom}
}

func (e *editor) scale(w int32) float64 { return e.timelineView(w).Scale() }

func (e *editor) timeAt(x, w int32) int64 { return e.timelineView(w).TimeAt(x) }

func (e *editor) xAt(t int64, w int32) int32 { return e.timelineView(w).XAt(t) }

func (e *editor) timelineStack() timeline.Stack {
	return timeline.LayoutStack(e.model.Project(), clientRect(e.timeline).Bottom, e.laneScroll)
}

func (e *editor) clipGeometry(v timeline.View, c project.Clip) (timeline.ClipGeometry, bool) {
	l, ok := e.timelineStack().Find(e.trackID())
	if !ok {
		return timeline.ClipGeometry{}, false
	}
	return e.laneClipGeometry(v, c, l)
}

func (e *editor) laneClipGeometry(v timeline.View, c project.Clip, l timeline.Lane) (timeline.ClipGeometry, bool) {
	a, ok := e.model.Project().Asset(c.AssetID)
	g, visible := v.Clip(c, ok && a.HasAudio && !c.AudioDetached && l.Kind == project.VideoTrack)
	g.OffsetY = l.Offset()
	return g, visible
}

func (e *editor) selectTimelineLane(id string) {
	if id != e.trackID() {
		e.visibleTrack = id
		e.switchTrack(id)
	}
}

func beginLanePaint(dc uintptr, l timeline.Lane, w, bottom int32) uintptr {
	top, end := max(timeline.TrackTop, l.Top), min(bottom, l.Bottom)
	if end <= top {
		return 0
	}
	saved, _, _ := gdi.NewProc("SaveDC").Call(dc)
	if saved != 0 {
		gdi.NewProc("IntersectClipRect").Call(dc, 0, uintptr(top), uintptr(w), uintptr(end))
		gdi.NewProc("SetViewportOrgEx").Call(dc, 0, uintptr(l.Offset()), 0)
	}
	return saved
}

func (e *editor) zoomTimeline(id int) {
	w := clientRect(e.timeline).Right
	if id == idFit {
		e.zoom, e.origin = 0, 0
	} else {
		factor := 1.5
		if id == idZoomOut {
			factor = 1 / factor
		}
		e.zoom, e.origin = e.timelineView(w).ZoomAt(e.playhead, factor)
	}
	e.invalidateTimeline()
}

func stamp(t int64) string {
	s := t / project.TicksPerSecond
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (e *editor) paintTimeline(dc uintptr, r rect) {
	w, h := r.Right, r.Bottom
	view := e.timelineView(w)
	fill(dc, r, palette.black)
	fill(dc, rect{0, 0, w, 32}, palette.panel)
	current := e.model.Project()
	policies := current.TrackPolicies()
	stack := timeline.LayoutStack(current, h, e.laneScroll)
	e.laneScroll = stack.Scroll
	textAt(dc, rect{10, 0, trackHeader - 4, 32}, "TIME", palette.muted, 4)
	for _, l := range stack.Lanes {
		saved := beginLanePaint(dc, l, w, stack.Bottom)
		if saved == 0 {
			continue
		}
		header := palette.panel
		if l.TrackID == e.trackID() {
			header = palette.selected
		}
		end := timeline.TrackBottom
		kind := "Audio"
		if l.Kind == project.VideoTrack {
			kind, end = "Video", timeline.ContentBottom
		}
		fill(dc, rect{0, timeline.TrackTop, trackHeader, end}, header)
		fill(dc, rect{trackHeader, timeline.TrackTop, w, end}, palette.bg)
		textAt(dc, rect{12, laneTop, trackHeader - 4, laneTop + 22}, l.TrackID+" "+kind, palette.text, 4)
		if i := current.TrackIndex(l.TrackID); i >= 0 {
			status := trackStatus(current.Tracks[i])
			if status != "" {
				textAt(dc, rect{12, laneTop + 22, trackHeader - 4, laneBottom}, status, palette.accent, 4)
			}
		}
		if l.Kind == project.VideoTrack {
			textAt(dc, rect{12, timeline.AudioTop, trackHeader - 4, timeline.AudioBottom}, "Linked audio", palette.muted, 4)
		}
		gdi.NewProc("RestoreDC").Call(dc, saved)
	}
	step := rulerStep(e.scale(w))
	minor := max(int64(1), step/5)
	start := e.origin / minor * minor
	for t := start; t >= 0; t += minor {
		x := e.xAt(t, w)
		if x >= w {
			break
		}
		if x >= trackHeader {
			if t%step == 0 {
				fill(dc, rect{x, 28, x + 1, stack.Bottom}, palette.line)
				textAt(dc, rect{x + 4, 0, x + 82, 28}, rulerLabel(t, step), palette.muted, 4)
			} else {
				fill(dc, rect{x, 24, x + 1, 32}, palette.muted)
			}
		}
		if t > math.MaxInt64-minor {
			break
		}
	}
	var demand []thumbnailKey
	var audioDemand []waveformKey
	p := e.model.Project()
	if d := e.drag; d != nil && d.moved {
		p = d.groupProject
	}
	for _, l := range stack.Lanes {
		ti := p.TrackIndex(l.TrackID)
		if ti < 0 {
			continue
		}
		saved := beginLanePaint(dc, l, w, stack.Bottom)
		if saved == 0 {
			continue
		}
		audioTrack := l.Kind == project.AudioTrack
		for i, c := range p.Tracks[ti].Clips {
			valid := true
			if d := e.drag; d != nil && d.moved && !d.valid {
				valid = !e.model.Contains(c.ID) && !(d.ripple && l.TrackID == d.trackID)
			}
			a, _ := p.Asset(c.AssetID)
			geometry, visible := view.Clip(c, a.HasAudio && !c.AudioDetached && !audioTrack)
			if !visible {
				continue
			}
			left, right := geometry.Left, geometry.Right
			col := palette.clip
			if audioTrack {
				col = palette.audio
			}
			if e.clipSelected(i, c) {
				col = palette.selected
			}
			offline := e.offline(a.Path)
			if offline {
				col = rgb(94, 52, 60)
			}
			policy := policies[l.TrackID]
			if (audioTrack && (!policy.Audio || c.EffectiveGainPercent() == 0)) || (!audioTrack && !policy.Video) {
				col = rgb(53, 56, 62)
			}
			if !valid {
				col = rgb(136, 55, 61)
			}
			fill(dc, rect{left, laneTop, right, laneBottom}, col)
			if valid && !offline {
				if audioTrack {
					e.paintWaveform(dc, left, right, w, laneTop+19, laneBottom, c, a, &audioDemand)
				} else {
					e.paintThumbnails(dc, left, right, w, c, a, &demand)
				}
			}
			if offline && right-left > 48 {
				textAt(dc, rect{left + 6, laneTop + 19, right - 5, laneBottom}, "MEDIA OFFLINE", rgb(255, 166, 166), 4)
			}
			if e.clipSelected(i, c) && right-left > 12 {
				fill(dc, rect{left, laneTop, left + 3, laneBottom}, palette.accent)
				fill(dc, rect{right - 3, laneTop, right, laneBottom}, palette.accent)
			}
			if right-left > 20 {
				textAt(dc, rect{left + 8, laneTop, right - 7, laneTop + 19}, fmt.Sprintf("%s  %.2fs%s", filepath.Base(a.Path), project.Seconds(c.Out-c.In), audioClipLabel(c)+layerClipLabel(c)), palette.text, 4)
			}
			if geometry.LinkedAudio {
				audioColor := palette.audio
				if !policy.Audio || c.EffectiveGainPercent() == 0 {
					audioColor = rgb(53, 56, 62)
				}
				if offline {
					audioColor = rgb(74, 44, 49)
				}
				fill(dc, rect{left, timeline.AudioTop, right, timeline.AudioBottom}, audioColor)
				if valid && !offline {
					e.paintWaveform(dc, left, right, w, timeline.AudioTop, timeline.AudioBottom, c, a, &audioDemand)
				}
				if offline && right-left > 48 {
					textAt(dc, rect{left + 6, timeline.AudioTop, right - 5, timeline.AudioBottom}, "Offline audio", rgb(255, 166, 166), 4)
				}
				if e.clipSelected(i, c) && e.model.Count() > 1 {
					fill(dc, rect{left, timeline.AudioTop, right, timeline.AudioTop + 2}, palette.accent)
				}
			}
		}
		gdi.NewProc("RestoreDC").Call(dc, saved)
	}
	if e.thumbnails != nil {
		if e.drag != nil || e.assetDrag != nil || e.marquee != nil {
			e.thumbnails.request(nil)
		} else {
			e.thumbnails.request(demand)
		}
	}
	if e.waveforms != nil {
		e.waveforms.request(audioDemand)
	}
	if d := e.drag; d != nil && d.snapped && d.valid {
		x := e.xAt(d.snapTarget, w)
		if x >= trackHeader && x < w {
			fill(dc, rect{x, 28, x + 2, stack.Bottom}, rgb(112, 220, 241))
			textAt(dc, rect{max(trackHeader, x-40), 0, min(w, x+80), 27}, func() string {
				if d.frames != nil && !keyPressed(0x10) {
					return "FRAME"
				}
				return "SNAP"
			}(), rgb(112, 220, 241), 4)
		}
	}
	if d := e.drag; d != nil && d.ripple && d.moved && d.mode == "left" && d.valid {
		at := d.original.Start
		if d.rippleDelta >= -at {
			at += d.rippleDelta
		} else {
			at = 0
		}
		x := e.xAt(at, w)
		if x >= trackHeader && x < w {
			if l, ok := stack.Find(d.trackID); ok {
				fill(dc, rect{x, max(timeline.TrackTop, laneTop+l.Offset()), x + 1, min(stack.Bottom, laneBottom+l.Offset())}, rgb(112, 220, 241))
			}
		}
	}
	e.paintAssetGhost(dc, w)
	e.paintMarquee(dc, w, h)
	x := e.xAt(e.playhead, w)
	if x >= trackHeader && x < w {
		fill(dc, rect{x, 19, x + 2, stack.Bottom}, palette.accent)
		for i := int32(0); i < 8; i++ {
			left, right := x-7+i, x+9-i
			if left < trackHeader {
				left = trackHeader
			}
			fill(dc, rect{left, 12 + i, right, 13 + i}, palette.accent)
		}
	}
	if h > stack.Bottom {
		textAt(dc, rect{trackHeader + 8, stack.Bottom, w - 8, h}, fmt.Sprintf("%s  |  Shift+wheel: tracks   Ctrl+wheel: zoom   %.2gx", precisionStamp(e.playhead), e.speed()), palette.muted, 4)
	}
}

func (e *editor) selectClip(i int) {
	e.model.SelectIndex(i)
	e.syncSelectionUI()
}

func (e *editor) beginTimeline(x, y, w int32) {
	if e.menuOpen || e.busy || e.marquee != nil || e.assetDrag != nil || e.cropSession != nil || e.transformSession != nil {
		return
	}
	if x < trackHeader && y < timeline.TrackTop {
		return
	}
	t := e.timeAt(x, w)
	if t > e.endTime() {
		t = e.endTime()
	}
	playing := e.playing()
	if y >= 0 && y < timeline.RulerBottom {
		e.scrubbing, e.scrubPlaying = true, playing
		user.NewProc("SetCapture").Call(e.timeline)
		e.seekTimeline(t, false)
		return
	}
	l, ok := e.timelineStack().At(y)
	if !ok {
		return
	}
	if l.TrackID != e.trackID() {
		e.selectTimelineLane(l.TrackID)
		playing = e.playing()
	}
	if x < trackHeader {
		return
	}
	view := e.timelineView(w)
	for i, c := range e.track().Clips {
		geometry, visible := e.laneClipGeometry(view, c, l)
		if !visible {
			continue
		}
		if mode := geometry.MainHit(x, y); mode != "" {
			e.updatePlaybackClock()
			if keyPressed(0x11) || keyPressed(0x10) {
				e.modifySelection(i, keyPressed(0x11), keyPressed(0x10))
				return
			}
			if !e.clipSelected(i, c) || e.model.Count() <= 1 {
				e.selectClip(i)
			} else {
				e.model.SetPrimary(c.ID)
				e.syncSelectionUI()
				mode = "move"
			}
			if e.track().Locked {
				if e.stillActive {
					e.seekTimeline(t, false)
					e.checkpoint()
					return
				}
				e.stopPreview()
				e.playhead = t
				e.refreshPlaybackMarker()
				e.checkpoint()
				e.tell("Track locked; clip selected for inspection.")
				return
			}
			ids := e.selectionIDs()
			var frames *media.FrameIndex
			var duration int64
			if !e.audioTrack() && e.frameSnap && mode != "move" && len(ids) == 1 {
				a, ok := e.model.Project().Asset(c.AssetID)
				if !ok {
					return
				}
				duration = a.Duration
				frames = e.cachedFrames(a)
				if frames == nil {
					if err := e.loadFrames(a, c.ID, 0); err != nil {
						e.tell(err.Error())
					}
					return
				}
			}
			if e.stillActive && playing {
				e.freezeCompositePlayback()
			}
			e.drag = &timelineDrag{index: i, trackID: e.trackID(), targetID: e.trackID(), y: y, mode: mode, x: x, scale: e.scale(w), original: c, candidate: c, valid: true, wasPlaying: playing, clickTime: t, groupIDs: ids, groupProject: e.model.Project(), ripple: e.rippleTrim && mode != "move" && len(ids) == 1, frames: frames, sourceDuration: duration}
			user.NewProc("SetCapture").Call(e.timeline)
			e.setTimelineCursor(x, y, w)
			return
		}
		if geometry.AudioHit(x, y) {
			if keyPressed(0x11) || keyPressed(0x10) {
				e.modifySelection(i, keyPressed(0x11), keyPressed(0x10))
				return
			}
			e.seekTimeline(t, playing)
			return
		}
	}
	if y >= l.Top && y < l.Bottom {
		e.beginMarquee(x, y, t, playing)
		return
	}
	e.seekTimeline(t, playing)
}

func (e *editor) moveTimeline(x, y int32) {
	d := e.drag
	if d == nil {
		return
	}
	dy := int32(0)
	if d.mode == "move" {
		dy = y - d.y
	}
	if !d.moved && !timeline.DragStarted(x-d.x, dy) {
		return
	}
	if !d.moved {
		e.stopPreview()
	}
	d.moved = true
	d.targetID = d.trackID
	if d.mode == "move" {
		l, ok := e.timelineStack().At(y)
		if !ok || !l.MainContains(y) {
			d.valid, d.snapped = false, false
			d.groupProject = e.model.Project()
			e.invalidateTimeline()
			return
		}
		d.targetID = l.TrackID
	}
	r := timeline.EvaluateDrag(e.model.Project(), timeline.DragRequest{
		Ref: project.ClipRef{TrackID: d.trackID, ClipID: d.original.ID}, TargetTrack: d.targetID,
		Mode: d.mode, GroupIDs: d.groupIDs, Pixels: x - d.x, Scale: d.scale,
		Marker: e.playhead, Magnetic: !e.snapDisabled, Bypass: keyPressed(0x10),
		Ripple: d.ripple, Frames: d.frames, SourceDuration: d.sourceDuration,
	})
	d.candidate, d.groupProject = r.Clip, r.Project
	d.valid, d.snapped, d.snapTarget, d.rippleDelta = r.Valid, r.Snapped, r.Target, r.RippleDelta
	if d.targetID == d.trackID && !d.ripple && len(d.groupIDs) <= 1 {
		d.groupProject = e.model.Project().Clone()
		if ti, ci, ok := d.groupProject.ClipIndex(project.ClipRef{TrackID: d.trackID, ClipID: d.original.ID}); ok && r.Clip.ID == d.original.ID {
			d.groupProject.Tracks[ti].Clips[ci] = r.Clip
		}
	}
	e.invalidateTimeline()
}

func (e *editor) finishTimeline() {
	d := e.drag
	e.drag = nil
	user.NewProc("ReleaseCapture").Call()
	if d != nil && d.moved {
		if d.valid {
			q := d.groupProject
			if d.ripple && d.rippleDelta == 0 {
				e.invalidateTimeline()
				return
			}
			if err := q.Validate(); err != nil {
				e.tell(err.Error())
			} else {
				end := q.EndTime()
				playhead, zoom := timelineEditView(e.playhead, end, e.zoom, d.scale)
				update := editorstate.SelectionUpdate{}
				if d.targetID != d.trackID {
					update = editorstate.SelectionUpdate{TrackID: d.targetID, ReplaceClips: true, ClipIDs: d.groupIDs, Primary: d.original.ID, Anchor: d.original.ID}
				}
				if err := e.commitEdit(q, update, &editView{playhead: playhead, zoom: zoom}); err != nil {
					e.tell(err.Error())
				} else if d.ripple {
					e.tell("Ripple trim applied.")
				} else {
					e.tell("Timeline edit applied.")
				}
			}
		} else {
			e.tell("Edit rejected: choose a compatible track, avoid overlaps and stay within source bounds.")
		}
	} else if d != nil {
		e.selectClip(d.index)
		e.seekTimeline(d.clickTime, d.wasPlaying)
	}
	e.invalidateTimeline()
}

func timelineProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if ui == nil {
		r, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	x, y := int32(int16(lp&0xffff)), int32(int16((lp>>16)&0xffff))
	switch msg {
	case wmContextMenu:
		ui.timelineContext(lp)
		return 0
	case 0x0204:
		return 0
	case 0x0205:
		p := point{x, y}
		user.NewProc("ClientToScreen").Call(hwnd, uintptr(unsafe.Pointer(&p)))
		ui.timelineContext(uintptr(uint16(p.X)) | uintptr(uint16(p.Y))<<16)
		return 0
	case wmDropFiles:
		ui.handleDrop(hwnd, wp)
		return 0
	case 0x000f:
		var ps paintStruct
		dc, _, _ := user.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		r := clientRect(hwnd)
		if dc != 0 {
			mem, _, _ := gdi.NewProc("CreateCompatibleDC").Call(dc)
			bmp, _, _ := gdi.NewProc("CreateCompatibleBitmap").Call(dc, uintptr(r.Right), uintptr(r.Bottom))
			if mem != 0 && bmp != 0 {
				old, _, _ := gdi.NewProc("SelectObject").Call(mem, bmp)
				ui.paintTimeline(mem, r)
				gdi.NewProc("BitBlt").Call(dc, 0, 0, uintptr(r.Right), uintptr(r.Bottom), mem, 0, 0, 0x00cc0020)
				gdi.NewProc("SelectObject").Call(mem, old)
			} else {
				ui.paintTimeline(dc, r)
			}
			if bmp != 0 {
				gdi.NewProc("DeleteObject").Call(bmp)
			}
			if mem != 0 {
				gdi.NewProc("DeleteDC").Call(mem)
			}
		}
		user.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0014:
		return 1
	case 0x0020:
		if lp&0xffff == 1 {
			var p point
			ok, _, _ := user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
			if ok != 0 {
				user.NewProc("ScreenToClient").Call(hwnd, uintptr(unsafe.Pointer(&p)))
				ui.setTimelineCursor(p.X, p.Y, clientRect(hwnd).Right)
				return 1
			}
		}
	case 0x0201:
		user.NewProc("SetFocus").Call(hwnd)
		ui.beginTimeline(x, y, clientRect(hwnd).Right)
		return 0
	case 0x0200:
		if ui.marquee != nil {
			ui.moveMarquee(x, y, clientRect(hwnd).Right)
		} else if ui.scrubbing {
			ui.seekTimeline(ui.timeAt(x, clientRect(hwnd).Right), false)
		} else {
			ui.moveTimeline(x, y)
		}
		ui.setTimelineCursor(x, y, clientRect(hwnd).Right)
		return 0
	case 0x0202:
		if ui.marquee != nil {
			ui.moveMarquee(x, y, clientRect(hwnd).Right)
			ui.endMarquee(false)
		} else if ui.scrubbing {
			t := ui.timeAt(x, clientRect(hwnd).Right)
			playing := ui.scrubPlaying
			ui.scrubbing, ui.scrubPlaying = false, false
			user.NewProc("ReleaseCapture").Call()
			ui.seekTimeline(t, playing)
		} else {
			ui.moveTimeline(x, y)
			ui.finishTimeline()
		}
		ui.setTimelineCursor(x, y, clientRect(hwnd).Right)
		return 0
	case 0x0215:
		ui.drag = nil
		ui.scrubbing, ui.scrubPlaying = false, false
		if ui.marquee != nil {
			d := ui.marquee
			ui.marquee = nil
			ui.setSelection(d.before, d.primary)
		}
		ui.invalidateTimeline()
		return 0
	case 0x020a:
		if ui.menuOpen || ui.busy || ui.assetDrag != nil || ui.marquee != nil || ui.scrubbing || (ui.drag != nil && (wp&4 == 0 || wp&8 != 0 || ui.drag.mode != "move")) {
			return 0
		}
		d := int16(wp >> 16)
		if wp&8 != 0 {
			if d > 0 {
				ui.zoomTimeline(idZoomIn)
			} else {
				ui.zoomTimeline(idZoomOut)
			}
		} else if wp&4 != 0 {
			stack := ui.timelineStack()
			step := int32(48)
			if d > 0 {
				step = -step
			}
			ui.laneScroll = max(0, min(stack.MaxScroll, stack.Scroll+step))
			if ui.drag != nil {
				p := point{x, y}
				user.NewProc("ScreenToClient").Call(hwnd, uintptr(unsafe.Pointer(&p)))
				ui.moveTimeline(p.X, p.Y)
				ui.setTimelineCursor(p.X, p.Y, clientRect(hwnd).Right)
			}
			ui.invalidateTimeline()
		} else {
			w := clientRect(hwnd).Right
			ui.origin = ui.timelineView(w).Scroll(d < 0)
			ui.invalidateTimeline()
		}
		return 0
	}
	r, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

const (
	cursorArrow   uintptr = 32512
	cursorCross   uintptr = 32515
	cursorSizeWE  uintptr = 32644
	cursorSizeAll uintptr = 32646
	cursorNo      uintptr = 32648
	cursorHand    uintptr = 32649
)

func videoHitMode(x, y, left, right int32) string {
	g := timeline.ClipGeometry{Span: timeline.Span{Left: max(trackHeader, left), Right: right}, RawLeft: left, RawRight: right}
	return g.MainHit(x, y)
}

func cursorForMode(mode string, invalid bool) uintptr {
	if invalid {
		return cursorNo
	}
	switch mode {
	case "move":
		return cursorSizeAll
	case "left", "right":
		return cursorSizeWE
	default:
		return cursorArrow
	}
}

func (e *editor) timelineCursor(x, y, w int32) uintptr {
	if e.busy {
		return cursorArrow
	}
	if e.scrubbing || e.marquee != nil {
		return cursorCross
	}
	if e.drag != nil {
		return cursorForMode(e.drag.mode, e.drag.moved && !e.drag.valid)
	}
	if x < trackHeader || x >= w {
		return cursorArrow
	}
	l, ok := e.timelineStack().At(y)
	if ok {
		view := e.timelineView(w)
		ti := e.model.Project().TrackIndex(l.TrackID)
		for _, c := range e.model.Project().Tracks[ti].Clips {
			geometry, visible := e.laneClipGeometry(view, c, l)
			if !visible {
				continue
			}
			if mode := geometry.MainHit(x, y); mode != "" {
				if e.model.Project().Tracks[ti].Locked {
					return cursorArrow
				}
				if e.model.Count() > 1 && e.model.Contains(c.ID) {
					mode = "move"
				}
				return cursorForMode(mode, false)
			}
			if geometry.AudioHit(x, y) {
				return cursorHand
			}
		}
	}
	if y >= 0 && y < timeline.RulerBottom {
		return cursorCross
	}
	return cursorArrow
}

func (e *editor) setTimelineCursor(x, y, w int32) {
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, e.timelineCursor(x, y, w))
	if cursor != 0 {
		user.NewProc("SetCursor").Call(cursor)
	}
}

func timelineEditView(marker, end int64, zoom, dragScale float64) (int64, float64) {
	return timeline.EditView(marker, end, zoom, dragScale)
}
