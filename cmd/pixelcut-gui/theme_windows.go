package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type minMaxInfo struct{ Reserved, MaxSize, MaxPosition, MinTrack, MaxTrack point }
type placedControl struct {
	h                   uintptr
	x, y, w, height, id int
}
type drawItem struct {
	Type, ID, Item, Action, State uint32
	HWND, DC                      uintptr
	Rect                          rect
	Data                          uintptr
}

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

var palette = struct{ bg, panel, input, button, hover, muted, text, accent, black, line, clip, selected, audio uintptr }{
	rgb(22, 24, 29), rgb(30, 33, 40), rgb(38, 41, 49), rgb(46, 50, 60), rgb(65, 70, 82), rgb(153, 162, 176), rgb(233, 236, 242), rgb(255, 206, 27), rgb(10, 12, 15), rgb(55, 60, 71), rgb(42, 91, 98), rgb(57, 116, 125), rgb(48, 73, 59),
}
var theme struct{ background, panel, input uintptr }
var brushes = map[uintptr]uintptr{}

func brush(c uintptr) uintptr {
	if h := brushes[c]; h != 0 {
		return h
	}
	h, _, _ := gdi.NewProc("CreateSolidBrush").Call(c)
	brushes[c] = h
	return h
}

func initTheme() {
	theme.background = brush(palette.bg)
	theme.panel = brush(palette.panel)
	theme.input = brush(palette.input)
}

func freeTheme() {
	for _, h := range brushes {
		gdi.NewProc("DeleteObject").Call(h)
	}
	gdi.NewProc("DeleteObject").Call(font)
}

func modernFont() uintptr {
	height := int32(-16)
	h, _, _ := gdi.NewProc("CreateFontW").Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u("Segoe UI"))))
	if h == 0 {
		h, _, _ = gdi.NewProc("GetStockObject").Call(17)
	}
	return h
}

func applyDarkTitle(h uintptr) {
	dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if dwm.Find() != nil {
		return
	}
	on := uint32(1)
	r, _, _ := dwm.Call(h, 20, uintptr(unsafe.Pointer(&on)), 4)
	if r != 0 {
		dwm.Call(h, 19, uintptr(unsafe.Pointer(&on)), 4)
	}
}

func fill(dc uintptr, r rect, c uintptr) {
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}
	user.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), brush(c))
}

func textAt(dc uintptr, r rect, s string, c uintptr, flags uintptr) {
	gdi.NewProc("SetTextColor").Call(dc, c)
	gdi.NewProc("SetBkMode").Call(dc, 1)
	old, _, _ := gdi.NewProc("SelectObject").Call(dc, font)
	buf := syscall.StringToUTF16(s)
	user.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)-1), uintptr(unsafe.Pointer(&r)), flags|0x20|0x800|0x8000)
	gdi.NewProc("SelectObject").Call(dc, old)
}

func nativeCopy(dst, src, size uintptr) {
	syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory").Call(dst, src, size)
}

func drawButton(lp uintptr) bool {
	if lp == 0 {
		return false
	}
	var d drawItem
	nativeCopy(uintptr(unsafe.Pointer(&d)), lp, unsafe.Sizeof(d))
	runtime.KeepAlive(&d)
	if d.Type != 4 {
		return false
	}
	c, fg := palette.button, palette.text
	if int(d.ID) == idExportMenu {
		c, fg = palette.accent, palette.black
	}
	if d.State&1 != 0 {
		c = palette.hover
		fg = palette.text
	}
	if d.State&4 != 0 {
		c, fg = palette.panel, palette.muted
	}
	fill(d.DC, d.Rect, c)
	textAt(d.DC, d.Rect, read(d.HWND), fg, 1|4)
	if d.State&0x10 != 0 {
		r := d.Rect
		r.Left += 2
		r.Top += 2
		r.Right -= 2
		r.Bottom -= 2
		user.NewProc("DrawFocusRect").Call(d.DC, uintptr(unsafe.Pointer(&r)))
	}
	return true
}

func colorControl(msg uint32, dc, hwnd uintptr) uintptr {
	bg, fg := palette.bg, palette.text
	if msg == 0x0133 || msg == 0x0134 {
		bg = palette.input
	}
	if ui != nil && hwnd == ui.host {
		bg = palette.black
	}
	gdi.NewProc("SetTextColor").Call(dc, fg)
	gdi.NewProc("SetBkColor").Call(dc, bg)
	return brush(bg)
}

func clientRect(h uintptr) rect {
	var r rect
	user.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&r)))
	return r
}

func (e *editor) resize() {
	r := clientRect(mainWindow)
	w, h := int(r.Right), int(r.Bottom)
	dx, dy := w-1108, h-670
	for _, p := range e.layout {
		if p.id == idClips {
			continue
		}
		x, y, ww, hh := p.x, p.y, p.w, p.height
		switch {
		case p.h == e.timeline:
			ww += dx
			hh += dy
		case p.h == e.status:
			y += dy
			ww += dx
		case p.id == idExportMenu:
			x = mainToolbarX(p.id, x, ww, w)
		case p.id == idZoomIn || p.id == idZoomOut || p.id == idFit || p.id == idSnap || (p.x >= 825 && p.y >= 50):
			x += dx
		case p.h == e.host || p.h == e.layerSurface || p.h == e.audioMeter:
			ww += dx
		}
		if ww < 1 {
			ww = 1
		}
		if hh < 1 {
			hh = 1
		}
		user.NewProc("MoveWindow").Call(p.h, uintptr(x), uintptr(y), uintptr(ww), uintptr(hh), 1)
	}
	e.invalidateTimeline()
}
