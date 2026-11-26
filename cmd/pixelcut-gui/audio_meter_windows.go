package main

import (
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
)

type audioMeterView struct {
	label string
	peak  [media.PCMChannels]uint32
	hold  [media.PCMChannels]uint32
	clips int64
}

func registerAudioMeter() {
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	wc := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: syscall.NewCallback(audioMeterProc), Instance: instance, Cursor: cursor, Name: u("PixelCutAudioMeter")}
	if n, _, err := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); n == 0 {
		panic(err)
	}
}

func (e *editor) pollAudioMeter() {
	if e.audioMeter == 0 {
		return
	}
	view := audioMeterView{label: "Mix meter | Play composite"}
	if run := e.compositeRun; run != nil && run.metrics != nil {
		latest, total, at := run.metrics.AudioMeter()
		view.label = "Mix meter | confirmed PCM"
		view.hold, view.clips = total.Peak, total.ClipCount()
		if !at.IsZero() && time.Since(at) <= 250*time.Millisecond {
			view.peak = latest.Peak
		}
	} else if e.exportJob != nil {
		levels := e.exportJob.audioLevels()
		view.label = "Export | submitted PCM peak"
		view.peak, view.hold, view.clips = levels.Peak, levels.Peak, levels.ClipCount()
	} else if e.previewActive() {
		view.label = "Source audition | not metered"
	} else if e.stillActive && e.lastCompositeMetrics != nil && !e.compositeResume {
		levels := e.lastCompositeMetrics.Snapshot().Levels
		view.label = "Last composite run | peak hold"
		view.hold, view.clips = levels.Peak, levels.ClipCount()
	}
	if view != e.meterView {
		e.meterView = view
		user.NewProc("InvalidateRect").Call(e.audioMeter, 0, 0)
	}
}

func meterWidth(peak uint32, width int32) int32 {
	db := media.PeakDBFS(peak)
	return int32(math.Round(float64(max(0, width)) * max(0, min(1, (db+60)/60))))
}

func paintAudioMeter(dc uintptr, r rect, view audioMeterView) {
	fill(dc, r, palette.panel)
	w := r.Right
	if w < 100 {
		return
	}
	if view.label == "" {
		view.label = "Mix meter | Play composite"
	}
	textAt(dc, rect{6, 0, w - 6, 17}, view.label, palette.muted, 4)
	if view.clips > 0 {
		textAt(dc, rect{w / 2, 0, w - 6, 17}, fmt.Sprintf("CLIP %d", view.clips), rgb(255, 113, 113), 6)
	}
	for ch, name := range []string{"L", "R"} {
		y := int32(18 + ch*13)
		left, right := int32(22), w-69
		width := right - left
		if width <= 0 {
			continue
		}
		textAt(dc, rect{6, y, 20, y + 12}, name, palette.text, 4)
		fill(dc, rect{left, y + 2, right, y + 10}, palette.black)
		n := meterWidth(view.peak[ch], width)
		green, yellow := width*48/60, width*57/60
		fill(dc, rect{left, y + 2, left + min(n, green), y + 10}, rgb(66, 184, 121))
		fill(dc, rect{left + green, y + 2, left + min(n, yellow), y + 10}, palette.accent)
		fill(dc, rect{left + yellow, y + 2, left + n, y + 10}, rgb(226, 77, 77))
		if view.hold[ch] > 0 {
			x := left + max(0, min(width-1, meterWidth(view.hold[ch], width)))
			fill(dc, rect{x, y, x + 1, y + 12}, palette.text)
		}
		textAt(dc, rect{right + 4, y, w - 4, y + 12}, media.PeakLabel(view.peak[ch])+" dB", palette.text, 6)
	}
}

func audioMeterProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0014:
		return 1
	case 0x000f:
		var ps paintStruct
		dc, _, _ := user.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if dc != 0 && ui != nil {
			paintAudioMeter(dc, clientRect(hwnd), ui.meterView)
		}
		user.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
