package composition

import (
	"errors"

	"github.com/markrai/pixelcut/internal/media"
)

const MaxTransitionDecoders = 4

func FullCanvasOpaque(canvas media.CanvasSize, b media.Placement, opacity int) bool {
	return opacity == 100 && b.DestX == 0 && b.DestY == 0 && b.Width == canvas.Width && b.Height == canvas.Height
}

func (p LayerPlan) DecodeLayers() []PlannedLayer {
	if len(p.Transitions) == 0 {
		return p.Layers
	}
	layers := make([]PlannedLayer, 0, len(p.Layers)+len(p.Transitions))
	layers = append(layers, p.Layers...)
	for _, t := range p.Transitions {
		layers = append(layers, t.From)
	}
	return layers
}

func (p LayerPlan) ActiveTransition() (int, TransitionSample, error) {
	if len(p.Transitions) == 0 {
		return -1, TransitionSample{}, nil
	}
	if len(p.Transitions) > 1 {
		return -1, TransitionSample{}, errors.New("supports at most one active video transition at a time")
	}
	t := p.Transitions[0]
	w := t.Window
	sample, ok := w.At(p.Time)
	if !ok || len(p.Layers)+1 > MaxTransitionDecoders || t.From.Layer.Ref != w.From || t.From.Layer.Timeline != w.Timeline || t.From.Layer.SourceInterval != w.FromSource || t.From.Layer.SourceTime != sample.FromTime {
		return -1, TransitionSample{}, errors.New("invalid transition plan or four-decoder budget exceeded")
	}
	if !FullCanvasOpaque(p.Canvas, t.From.Geometry.Bounds, t.From.Layer.Opacity) {
		return -1, TransitionSample{}, errors.New("cross-dissolve requires opaque full-canvas outgoing and incoming clips")
	}
	for i, layer := range p.Layers {
		if layer.Layer.Ref == w.To {
			if layer.Layer.SourceTime != sample.ToTime || !FullCanvasOpaque(p.Canvas, layer.Geometry.Bounds, layer.Layer.Opacity) {
				return -1, TransitionSample{}, errors.New("cross-dissolve requires opaque full-canvas outgoing and incoming clips")
			}
			return i, sample, nil
		}
	}
	return -1, TransitionSample{}, errors.New("cross-dissolve requires a visible opaque full-canvas incoming clip")
}
