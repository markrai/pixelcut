package media

import (
	"errors"
	"math"
)

type CanvasSize struct {
	Width, Height int
}

func (c CanvasSize) Validate() error {
	if c.Width < 2 || c.Height < 2 || c.Width > 8192 || c.Height > 8192 || c.Width%2 != 0 || c.Height%2 != 0 || int64(c.Width)*int64(c.Height) > 33554432 {
		return errors.New("composition canvas requires even dimensions within 2–8192 pixels and the frame allocation budget")
	}
	return nil
}

type LayerGeometry struct {
	LocalCanvas CanvasSize
	Content     Placement
	CanvasFit   Placement
	Bounds      Placement
}

func PlanLayer(canvas CanvasSize, width, height int, crop *Crop, transform *Transform) (LayerGeometry, error) {
	var g LayerGeometry
	if err := canvas.Validate(); err != nil {
		return g, err
	}
	w, h := width, height
	if crop != nil {
		if err := crop.Validate(Stream{Width: w, Height: h}); err != nil {
			return g, err
		}
		w, h = crop.Width, crop.Height
	}
	if w < 2 || h < 2 || w%2 != 0 || h%2 != 0 || w > 16384 || h > 16384 {
		return g, errors.New("unsupported layer dimensions")
	}
	if transform == nil {
		fit := Transform{CanvasWidth: canvas.Width, CanvasHeight: canvas.Height, Scale: 100}
		p, err := fit.Placement(w, h)
		if err != nil {
			return g, err
		}
		g.LocalCanvas = canvas
		g.Content = p
		g.CanvasFit = Placement{ScaledWidth: canvas.Width, ScaledHeight: canvas.Height, Width: canvas.Width, Height: canvas.Height}
		g.Bounds = p
		return g, nil
	}
	p, err := transform.Placement(w, h)
	if err != nil {
		return g, err
	}
	g.LocalCanvas = CanvasSize{Width: transform.CanvasWidth, Height: transform.CanvasHeight}
	g.Content = p
	fit := Transform{CanvasWidth: canvas.Width, CanvasHeight: canvas.Height, Scale: 100}
	f, err := fit.Placement(g.LocalCanvas.Width, g.LocalCanvas.Height)
	if err != nil {
		return g, err
	}
	g.CanvasFit = f
	x := func(v int) int {
		return f.DestX + int(math.Round(float64(v)*float64(f.ScaledWidth)/float64(g.LocalCanvas.Width)))
	}
	y := func(v int) int {
		return f.DestY + int(math.Round(float64(v)*float64(f.ScaledHeight)/float64(g.LocalCanvas.Height)))
	}
	left, top := x(p.DestX), y(p.DestY)
	right, bottom := x(p.DestX+p.Width), y(p.DestY+p.Height)
	g.Bounds = Placement{DestX: left, DestY: top, Width: max(0, right-left), Height: max(0, bottom-top)}
	return g, nil
}

type LayerPreset string

const (
	LayerFit         LayerPreset = "fit"
	LayerTopLeft     LayerPreset = "top-left"
	LayerTopRight    LayerPreset = "top-right"
	LayerBottomLeft  LayerPreset = "bottom-left"
	LayerBottomRight LayerPreset = "bottom-right"
)

func PresetTransform(canvas CanvasSize, w, h int, previous *Transform, preset LayerPreset) (Transform, error) {
	t := Transform{CanvasWidth: canvas.Width, CanvasHeight: canvas.Height, Scale: 100}
	if previous != nil {
		t.Rotation, t.FlipHorizontal, t.FlipVertical = previous.Rotation, previous.FlipHorizontal, previous.FlipVertical
	}
	if preset == LayerFit {
		_, err := t.Placement(w, h)
		return t, err
	}
	switch preset {
	case LayerTopLeft, LayerTopRight, LayerBottomLeft, LayerBottomRight:
	default:
		return t, errors.New("unknown layer placement preset")
	}
	t.Scale = 35
	p, err := t.Placement(w, h)
	if err != nil {
		return t, err
	}
	margin := max(0, min(evenRound(float64(min(canvas.Width, canvas.Height))*0.04), (min(canvas.Width-p.Width, canvas.Height-p.Height)/4)*2))
	left, top := margin, margin
	if preset == LayerTopRight || preset == LayerBottomRight {
		left = canvas.Width - p.Width - margin
	}
	if preset == LayerBottomLeft || preset == LayerBottomRight {
		top = canvas.Height - p.Height - margin
	}
	t.X, t.Y = left-p.DestX, top-p.DestY
	_, err = t.Placement(w, h)
	return t, err
}
