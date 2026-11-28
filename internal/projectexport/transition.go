package projectexport

import (
	"errors"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
)

func fullCanvasOpaque(canvas media.CanvasSize, b media.Placement, opacity int) bool {
	return opacity == 100 && b.DestX == 0 && b.DestY == 0 && b.Width == canvas.Width && b.Height == canvas.Height
}

func spanDecodeLayers(span Span) []composition.PlannedLayer {
	if span.Transition == nil {
		return span.Layers
	}
	out := make([]composition.PlannedLayer, 0, len(span.Layers)+1)
	out = append(out, span.Layers...)
	return append(out, span.Transition.From)
}

func validateTransitionSpan(span Span, canvas media.CanvasSize) error {
	t := span.Transition
	if t == nil {
		return nil
	}
	w := t.Window
	sample, ok := w.At(span.Start)
	if !ok || span.End <= span.Start || span.End > w.Timeline.End || len(span.Layers)+1 > MaxVideoLayers || t.From.Layer.Ref != w.From || t.From.Layer.Timeline != w.Timeline || t.From.Layer.SourceInterval != w.FromSource || t.From.Layer.SourceTime != sample.FromTime {
		return errors.New("invalid export transition span or four-decoder budget exceeded")
	}
	if !fullCanvasOpaque(canvas, t.From.Geometry.Bounds, t.From.Layer.Opacity) {
		return errors.New("cross-dissolve export currently requires opaque full-canvas outgoing and incoming clips")
	}
	for _, layer := range span.Layers {
		if layer.Layer.Ref == w.To {
			if layer.Layer.SourceTime != sample.ToTime || !fullCanvasOpaque(canvas, layer.Geometry.Bounds, layer.Layer.Opacity) {
				return errors.New("cross-dissolve export currently requires opaque full-canvas outgoing and incoming clips")
			}
			return nil
		}
	}
	return errors.New("cross-dissolve export requires a visible opaque full-canvas incoming clip")
}
