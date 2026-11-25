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
)

const trackHeader int32 = 92
const laneTop int32 = 39
const laneBottom int32 = 91

type paintStruct struct {
	DC              uintptr
	Erase           int32
	Rect            rect
	Restore, Update int32
	Reserved        [32]byte
}

type timelineDrag struct {
	index               int
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

func (e *editor) scale(w int32) float64 {
	if e.zoom > 0 {
		return e.zoom
	}
	return math.Max(1e-9, math.Max(1, float64(w-trackHeader-18))/math.Max(5, project.Seconds(e.endTime())*1.05))
}

func (e *editor) timeAt(x, w int32) int64 {
	t := float64(e.origin) + float64(x-trackHeader)/e.scale(w)*float64(project.TicksPerSecond)
	if t < 0 {
		return 0
	}
	if t >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(math.Round(t))
}

func (e *editor) xAt(t int64, w int32) int32 {
	x := float64(trackHeader) + project.Seconds(t-e.origin)*e.scale(w)
	if x < -100000 {
		return -100000
	}
	if x > 100000 {
		return 100000
	}
	return int32(math.Round(x))
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
		e.zoom = math.Max(.05, math.Min(1000, e.scale(w)*factor))
		e.origin = e.playhead - int64(float64(w-trackHeader)/e.zoom*float64(project.TicksPerSecond)/2)
		if e.origin < 0 {
			e.origin = 0
		}
	}
	e.invalidateTimeline()
}

func stamp(t int64) string {
	s := t / project.TicksPerSecond
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (e *editor) paintTimeline(dc uintptr, r rect) {
	w, h := r.Right, r.Bottom
	fill(dc, r, palette.black)
	fill(dc, rect{0, 0, w, 32}, palette.panel)
	fill(dc, rect{0, 33, trackHeader, h}, palette.panel)
	fill(dc, rect{trackHeader, 33, w, 97}, palette.bg)
	fill(dc, rect{trackHeader, 99, w, 145}, palette.bg)
	textAt(dc, rect{10, 0, trackHeader - 4, 32}, "TIME", palette.muted, 4)
	kind := "Video"
	if e.audioTrack() {
		kind = "Audio"
	}
	textAt(dc, rect{12, laneTop, trackHeader - 4, laneBottom}, e.trackID()+" "+kind, palette.text, 4)
	if !e.audioTrack() {
		textAt(dc, rect{12, 99, trackHeader - 4, 145}, "Linked audio", palette.muted, 4)
	} else {
		textAt(dc, rect{12, 99, trackHeader - 4, 145}, "Independent", palette.muted, 4)
		textAt(dc, rect{trackHeader + 8, 99, w - 8, 145}, "Audio editing only; mixed playback is not yet available.", palette.muted, 4)
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
				fill(dc, rect{x, 28, x + 1, h}, palette.line)
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
	for i, c := range e.track().Clips {
		valid := true
		if d := e.drag; d != nil && d.moved {
			if d.ripple || (len(d.groupIDs) > 1 && e.model.Contains(c.ID)) {
				c = d.groupProject.Tracks[e.trackIndex()].Clips[i]
				valid = d.valid
			} else if len(d.groupIDs) <= 1 && d.index == i {
				c = d.candidate
				valid = d.valid
			}
		}
		left, right := e.xAt(c.Start, w), e.xAt(c.Start+(c.Out-c.In), w)
		if right <= trackHeader || left >= w {
			continue
		}
		if left < trackHeader {
			left = trackHeader
		}
		if right > w {
			right = w
		}
		if right <= left {
			right = left + 1
		}
		col := palette.clip
		if e.audioTrack() {
			col = palette.audio
		}
		if e.clipSelected(i, c) {
			col = palette.selected
		}
		a, _ := e.model.Project().Asset(c.AssetID)
		offline := e.offline(a.Path)
		if offline {
			col = rgb(94, 52, 60)
		}
		if !valid {
			col = rgb(136, 55, 61)
		}
		fill(dc, rect{left, laneTop, right, laneBottom}, col)
		if valid && !offline {
			if e.audioTrack() {
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
			textAt(dc, rect{left + 8, laneTop, right - 7, laneTop + 19}, fmt.Sprintf("%s  %.2fs", filepath.Base(a.Path), project.Seconds(c.Out-c.In)), palette.text, 4)
		}
		if !e.audioTrack() && a.HasAudio {
			audioColor := palette.audio
			if offline {
				audioColor = rgb(74, 44, 49)
			}
			fill(dc, rect{left, 104, right, 140}, audioColor)
			if valid && !offline {
				e.paintWaveform(dc, left, right, w, 104, 140, c, a, &audioDemand)
			}
			if offline && right-left > 48 {
				textAt(dc, rect{left + 6, 104, right - 5, 140}, "Offline audio", rgb(255, 166, 166), 4)
			}
			if e.clipSelected(i, c) && e.model.Count() > 1 {
				fill(dc, rect{left, 104, right, 106}, palette.accent)
			}
		}
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
			fill(dc, rect{x, 28, x + 2, 145}, rgb(112, 220, 241))
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
			fill(dc, rect{x, laneTop, x + 1, laneBottom}, rgb(112, 220, 241))
		}
	}
	e.paintAssetGhost(dc, w)
	e.paintMarquee(dc, w, h)
	x := e.xAt(e.playhead, w)
	if x >= trackHeader && x < w {
		fill(dc, rect{x, 19, x + 2, h}, palette.accent)
		for i := int32(0); i < 8; i++ {
			left, right := x-7+i, x+9-i
			if left < trackHeader {
				left = trackHeader
			}
			fill(dc, rect{left, 12 + i, right, 13 + i}, palette.accent)
		}
	}
	if h > 150 {
		textAt(dc, rect{trackHeader + 8, 150, w - 8, h}, fmt.Sprintf("%s  |  , / .: frame step   Ctrl+arrows: edit boundaries   %.2gx", precisionStamp(e.playhead), e.speed()), palette.muted, 4)
	}
}

func (e *editor) selectClip(i int) {
	e.model.SelectIndex(i)
	e.syncSelectionUI()
}

func (e *editor) beginTimeline(x, y, w int32) {
	if e.menuOpen || e.busy || e.marquee != nil || e.assetDrag != nil || e.cropSession != nil || e.transformSession != nil || x < trackHeader {
		return
	}
	t := e.timeAt(x, w)
	if t > e.endTime() {
		t = e.endTime()
	}
	playing := e.playing()
	if y >= 0 && y < 32 {
		e.scrubbing, e.scrubPlaying = true, playing
		user.NewProc("SetCapture").Call(e.timeline)
		e.seekTimeline(t, false)
		return
	}
	for i, c := range e.track().Clips {
		l, r := e.xAt(c.Start, w), e.xAt(c.Start+(c.Out-c.In), w)
		if mode := videoHitMode(x, y, l, r); mode != "" {
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
			e.drag = &timelineDrag{index: i, mode: mode, x: x, scale: e.scale(w), original: c, candidate: c, valid: true, wasPlaying: playing, clickTime: t, groupIDs: ids, groupProject: e.model.Project(), ripple: e.rippleTrim && mode != "move" && len(ids) == 1, frames: frames, sourceDuration: duration}
			user.NewProc("SetCapture").Call(e.timeline)
			e.setTimelineCursor(x, y, w)
			return
		}
		if !e.audioTrack() && x >= l && x < r && y >= 104 && y < 140 {
			if a, ok := e.model.Project().Asset(c.AssetID); ok && a.HasAudio && (keyPressed(0x11) || keyPressed(0x10)) {
				e.modifySelection(i, keyPressed(0x11), keyPressed(0x10))
				return
			}
			e.seekTimeline(t, playing)
			return
		}
	}
	if y >= 33 && y < 145 {
		e.beginMarquee(x, y, t, playing)
		return
	}
	e.seekTimeline(t, playing)
}

func (e *editor) moveTimeline(x int32) {
	d := e.drag
	if d == nil {
		return
	}
	if !d.moved && math.Abs(float64(x-d.x)) < 4 {
		return
	}
	if !d.moved {
		e.stopPreview()
	}
	d.moved = true
	raw := float64(x-d.x) / d.scale * float64(project.TicksPerSecond)
	raw = math.Max(-1e15, math.Min(1e15, raw))
	delta := int64(math.Round(raw/10000)) * 10000
	tolerance := int64(0)
	if !e.snapDisabled && !keyPressed(0x10) {
		tolerance = int64(math.Min(2000000, math.Round(8/d.scale*float64(project.TicksPerSecond))))
	}
	if d.frames != nil && !keyPressed(0x10) {
		base := d.original.In
		if d.mode == "right" {
			base = d.original.Out
		}
		offset := int64(math.Round(raw))
		wanted := base + max(-base, min(d.sourceDuration-base, offset))
		frameDelta, _, ok := frameTrimDelta(d.frames, d.original, d.sourceDuration, d.mode, wanted, d.ripple)
		d.valid = false
		d.snapped = false
		d.candidate = d.original
		d.groupProject = e.model.Project()
		d.rippleDelta = 0
		if ok {
			if d.ripple {
				r, err := e.model.Project().RippleTrim(e.trackID(), d.original.ID, d.mode, frameDelta, -1, 0)
				d.groupProject = r.Project
				d.rippleDelta = r.Delta
				d.valid = err == nil && r.Delta == frameDelta
			} else {
				c, err := e.model.Project().DragClip(e.trackID(), d.original.ID, d.mode, frameDelta)
				d.candidate = c
				d.valid = err == nil && ((d.mode == "left" && c.In == d.original.In+frameDelta) || (d.mode == "right" && c.Out == d.original.Out+frameDelta))
			}
			if d.valid {
				d.snapped = true
				if d.mode == "right" {
					d.snapTarget = d.original.Start + (d.original.Out - d.original.In) + frameDelta
				} else {
					d.snapTarget = d.original.Start
					if frameDelta >= -d.original.Start && frameDelta <= math.MaxInt64-d.original.Start {
						d.snapTarget += frameDelta
					}
				}
			}
		}
	} else if d.ripple {
		r, err := e.model.Project().RippleTrim(e.trackID(), d.original.ID, d.mode, delta, e.playhead, tolerance)
		d.groupProject = r.Project
		d.rippleDelta = r.Delta
		d.valid = err == nil
		d.snapped = r.Snapped
		d.snapTarget = r.Target
	} else if len(d.groupIDs) > 1 {
		r, err := e.model.Project().MoveClips(e.trackID(), d.groupIDs, delta, e.playhead, tolerance)
		d.groupProject = r.Project
		d.valid = err == nil
		d.snapped = r.Snapped
		d.snapTarget = r.Target
	} else {
		r, err := e.model.Project().SnapDrag(e.trackID(), d.original.ID, d.mode, delta, e.playhead, tolerance)
		d.candidate = r.Clip
		d.valid = err == nil
		d.snapped = r.Snapped
		d.snapTarget = r.Target
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
			if !d.ripple && len(d.groupIDs) <= 1 {
				q = e.model.Project().Clone()
				q.Tracks[e.trackIndex()].Clips[d.index] = d.candidate
			}
			if d.ripple && d.rippleDelta == 0 {
				e.invalidateTimeline()
				return
			}
			if err := q.Validate(); err != nil {
				e.tell(err.Error())
			} else {
				end := q.EndTime()
				playhead, zoom := timelineEditView(e.playhead, end, e.zoom, d.scale)
				if err := e.commitEdit(q, editorstate.SelectionUpdate{}, &editView{playhead: playhead, zoom: zoom}); err != nil {
					e.tell(err.Error())
				} else if d.ripple {
					e.tell("Ripple trim applied.")
				} else {
					e.tell("Timeline edit applied.")
				}
			}
		} else {
			e.tell("Edit rejected: clips cannot overlap or exceed source bounds.")
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
			ui.moveTimeline(x)
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
			ui.scrubbing = false
			user.NewProc("ReleaseCapture").Call()
			ui.seekTimeline(t, playing)
		} else {
			ui.moveTimeline(x)
			ui.finishTimeline()
		}
		ui.setTimelineCursor(x, y, clientRect(hwnd).Right)
		return 0
	case 0x0215:
		ui.drag = nil
		ui.scrubbing = false
		if ui.marquee != nil {
			d := ui.marquee
			ui.marquee = nil
			ui.setSelection(d.before, d.primary)
		}
		ui.invalidateTimeline()
		return 0
	case 0x020a:
		if ui.menuOpen || ui.busy || ui.drag != nil || ui.assetDrag != nil || ui.marquee != nil {
			return 0
		}
		d := int16(wp >> 16)
		if wp&8 != 0 {
			if d > 0 {
				ui.zoomTimeline(idZoomIn)
			} else {
				ui.zoomTimeline(idZoomOut)
			}
		} else {
			w := clientRect(hwnd).Right
			step := int64(math.Min(float64(math.MaxInt64/2), math.Max(0, float64(w-trackHeader)/ui.scale(w)*float64(project.TicksPerSecond)/5)))
			if d > 0 {
				ui.origin -= step
				if ui.origin < 0 {
					ui.origin = 0
				}
			} else if ui.origin <= math.MaxInt64-step {
				ui.origin += step
			}
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
	if x < trackHeader || x < left || x >= right || y < laneTop || y >= laneBottom {
		return ""
	}
	if right-left >= 18 {
		if x-left < 7 {
			return "left"
		}
		if right-x <= 7 {
			return "right"
		}
	}
	return "move"
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
	for _, c := range e.track().Clips {
		l, r := e.xAt(c.Start, w), e.xAt(c.Start+(c.Out-c.In), w)
		if mode := videoHitMode(x, y, l, r); mode != "" {
			if e.model.Count() > 1 && e.model.Contains(c.ID) {
				mode = "move"
			}
			return cursorForMode(mode, false)
		}
		if !e.audioTrack() && x >= l && x < r && y >= 104 && y < 140 {
			if a, ok := e.model.Project().Asset(c.AssetID); ok && a.HasAudio {
				return cursorHand
			}
		}
	}
	if y >= 0 && y < 32 {
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
	if zoom == 0 && dragScale >= .05 && dragScale <= 1000 {
		zoom = dragScale
	}
	return min(marker, end), zoom
}
