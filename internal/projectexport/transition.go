package projectexport

import (
	"errors"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
)

func fullCanvasOpaque(canvas media.CanvasSize, b media.Placement, opacity int) bool {
	return composition.FullCanvasOpaque(canvas, b, opacity)
}

func spanDecodeLayers(span Span) []composition.PlannedLayer {
	if span.Transition == nil {
		return span.Layers
	}
	return (composition.LayerPlan{Layers: span.Layers, Transitions: []composition.PlannedTransition{*span.Transition}}).DecodeLayers()
}

func validateTransitionSpan(span Span, canvas media.CanvasSize) error {
	t := span.Transition
	if t == nil {
		return nil
	}
	if span.End <= span.Start || span.End > t.Window.Timeline.End {
		return errors.New("invalid export transition span")
	}
	_, _, err := (composition.LayerPlan{Time: span.Start, Canvas: canvas, Layers: span.Layers, Transitions: []composition.PlannedTransition{*t}}).ActiveTransition()
	return err
}
