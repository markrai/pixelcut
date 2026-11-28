package main

import (
	"fmt"
	"math"
	"runtime"
	"unsafe"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

const (
	menuLayerOpacity = 2300 + iota
	menuLayerPreset
	menuLayerCanvas
	menuLayerLayout
	layerSurfaceID
	menuCompositeStill
	menuCompositePlay
	menuSourcePlay
	menuCompositeDiagnostics
	layerOpacityBase = 2320
	layerPresetBase  = 2340
	layerCanvasBase  = 2360
)

var layerOpacityChoices = []int{0, 25, 50, 75, 100}

var layerPresets = []struct {
	label string
	value media.LayerPreset
}{
	{"Fit canvas", media.LayerFit},
	{"PiP top left (35%)", media.LayerTopLeft},
	{"PiP top right (35%)", media.LayerTopRight},
	{"PiP bottom left (35%)", media.LayerBottomLeft},
	{"PiP bottom right (35%)", media.LayerBottomRight},
}

var layerCanvases = []struct {
	label string
	value media.CanvasSize
}{
	{"Automatic (base video)", media.CanvasSize{}},
	{"Landscape 1920 x 1080", media.CanvasSize{Width: 1920, Height: 1080}},
	{"Portrait 1080 x 1920", media.CanvasSize{Width: 1080, Height: 1920}},
	{"Square 1080 x 1080", media.CanvasSize{Width: 1080, Height: 1080}},
	{"Landscape 3840 x 2160", media.CanvasSize{Width: 3840, Height: 2160}},
	{"Portrait 2160 x 3840", media.CanvasSize{Width: 2160, Height: 3840}},
}

func layerClipLabel(c project.Clip) string {
	if c.OpacityPercent() == 100 {
		return ""
	}
	return fmt.Sprintf(" / %d%% opacity", c.OpacityPercent())
}

func layerOpacityItems(c project.Clip) []menuItem {
	var items []menuItem
	for i, percent := range layerOpacityChoices {
		items = append(items, menuItem{id: layerOpacityBase + i, label: fmt.Sprintf("Opacity %d%%", percent), enabled: true, checked: c.OpacityPercent() == percent})
	}
	return items
}

func (e *editor) layerAction(id int, screen point) error {
	if id == menuLayerLayout {
		return e.showLayerLayout()
	}
	if id == menuCompositeStill {
		return e.showCompositeStill()
	}
	if id == menuCompositePlay {
		return e.startCompositePlayback()
	}
	if id == menuSourcePlay {
		e.stopPreview()
		return e.toggleSourceOrCompositePreview()
	}
	p := e.model.Project()
	if id == menuLayerCanvas {
		var items []menuItem
		for i, option := range layerCanvases {
			items = append(items, menuItem{id: layerCanvasBase + i, label: option.label, enabled: true, checked: p.CanvasWidth == option.value.Width && p.CanvasHeight == option.value.Height})
		}
		choice := e.popup(items, screen) - layerCanvasBase
		if choice < 0 || choice >= len(layerCanvases) {
			return nil
		}
		q, err := p.SetCompositionCanvas(layerCanvases[choice].value)
		if err != nil {
			return err
		}
		if err = e.commit(q); err != nil {
			return err
		}
		return e.showCompositeStill()
	}
	c, ok := e.selected()
	if !ok || e.model.Count() != 1 || e.audioTrack() {
		return fmt.Errorf("select a single video clip")
	}
	if err := p.RequireEditableTrack(e.trackID()); err != nil {
		return err
	}
	ref := project.ClipRef{TrackID: e.trackID(), ClipID: c.ID}
	var q project.Project
	var err error
	switch id {
	case menuLayerOpacity:
		choice := e.popup(layerOpacityItems(c), screen) - layerOpacityBase
		if choice < 0 || choice >= len(layerOpacityChoices) {
			return nil
		}
		q, err = p.SetClipOpacity(ref, layerOpacityChoices[choice])
	case menuLayerPreset:
		items := make([]menuItem, 0, len(layerPresets))
		for i, preset := range layerPresets {
			items = append(items, item(layerPresetBase+i, preset.label, true))
		}
		choice := e.popup(items, screen) - layerPresetBase
		if choice < 0 || choice >= len(layerPresets) {
			return nil
		}
		q, err = p.ApplyLayerPreset(ref, layerPresets[choice].value)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if err = e.commit(q); err != nil {
		return err
	}
	return e.showCompositeStill()
}

func (e *editor) showLayerLayout() error {
	e.stopPreview()
	compiled, err := composition.Compile(e.model.Project())
	if err != nil {
		return err
	}
	plan, err := compiled.PlanLayers(e.playhead)
	if err != nil {
		return err
	}
	if e.layerSurface == 0 {
		return fmt.Errorf("layer layout surface unavailable")
	}
	e.layerPlan = &plan
	user.NewProc("ShowWindow").Call(e.layerSurface, 5)
	user.NewProc("SetWindowPos").Call(e.layerSurface, 0, 0, 0, 0, 0, 0x0013)
	user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
	e.syncPreviewLabel()
	e.tell("Layer layout at playhead. Press Play to return to isolated source preview.")
	return nil
}

func (e *editor) hideLayerLayout() {
	if e.layerPlan == nil && !e.stillActive {
		return
	}
	e.cancelCompositeStill()
	e.layerPlan = nil
	if e.layerSurface != 0 {
		user.NewProc("ShowWindow").Call(e.layerSurface, 0)
	}
	e.syncPreviewLabel()
}

func (e *editor) drawLayerLayout(lp uintptr) bool {
	if lp == 0 || e.layerSurface == 0 {
		return false
	}
	var d drawItem
	nativeCopy(uintptr(unsafe.Pointer(&d)), lp, unsafe.Sizeof(d))
	runtime.KeepAlive(&d)
	if d.HWND != e.layerSurface {
		return false
	}
	fill(d.DC, d.Rect, palette.black)
	if e.stillActive {
		e.drawCompositeStill(d)
		return true
	}
	plan := e.layerPlan
	if plan == nil {
		return true
	}
	w, h := d.Rect.Right-d.Rect.Left, d.Rect.Bottom-d.Rect.Top
	textAt(d.DC, rect{8, 4, w - 8, 24}, fmt.Sprintf("%d x %d | %s | bottom to top", plan.Canvas.Width, plan.Canvas.Height, fmtTime(plan.Time)), palette.text, 4)
	if w < 100 || h < 80 {
		return true
	}
	scale := math.Min(float64(w-24)/float64(plan.Canvas.Width), float64(h-54)/float64(plan.Canvas.Height))
	cw, ch := int32(math.Round(float64(plan.Canvas.Width)*scale)), int32(math.Round(float64(plan.Canvas.Height)*scale))
	x, y := (w-cw)/2, 28+(h-54-ch)/2
	canvas := rect{x, y, x + cw, y + ch}
	fill(d.DC, canvas, palette.panel)
	colors := []uintptr{palette.accent, rgb(115, 206, 220), rgb(151, 205, 141), rgb(195, 155, 230)}
	for i, layer := range plan.Layers {
		b := layer.Geometry.Bounds
		l := x + int32(math.Round(float64(b.DestX)*scale))
		t := y + int32(math.Round(float64(b.DestY)*scale))
		r := min(canvas.Right, x+int32(math.Round(float64(b.DestX+b.Width)*scale)))
		bottom := min(canvas.Bottom, y+int32(math.Round(float64(b.DestY+b.Height)*scale)))
		if r <= l || bottom <= t {
			continue
		}
		col := colors[i%len(colors)]
		fill(d.DC, rect{l, t, r, min(t+2, bottom)}, col)
		fill(d.DC, rect{l, max(t, bottom-2), r, bottom}, col)
		fill(d.DC, rect{l, t, min(l+2, r), bottom}, col)
		fill(d.DC, rect{max(l, r-2), t, r, bottom}, col)
		tagY := min(bottom-18, t+3+int32(i%5)*18)
		if r-l > 65 && bottom-t >= 18 {
			fill(d.DC, rect{l + 2, tagY, min(r-2, l+108), tagY + 18}, palette.black)
			textAt(d.DC, rect{l + 4, tagY, r - 3, tagY + 18}, fmt.Sprintf("%s %d%%", layer.Layer.Ref.TrackID, layer.Layer.Opacity), col, 4)
		}
	}
	footer := fmt.Sprintf("%d visible layer(s) | outlines only, not rendered pixels", len(plan.Layers))
	textAt(d.DC, rect{8, h - 24, w - 8, h - 4}, footer, palette.muted, 4)
	return true
}
