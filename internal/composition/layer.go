package composition

import (
	"fmt"

	"github.com/markrai/pixelcut/internal/media"
)

type PlannedLayer struct {
	Layer    VideoLayer
	Geometry media.LayerGeometry
}

type LayerPlan struct {
	Time   int64
	Canvas media.CanvasSize
	Layers []PlannedLayer
}

func (e *Evaluator) PlanLayers(at int64) (LayerPlan, error) {
	plan := LayerPlan{Time: at, Canvas: e.canvas}
	f, err := e.At(at)
	if err != nil {
		return plan, err
	}
	for _, v := range f.Video {
		if v.Opacity == 0 {
			continue
		}
		var crop *media.Crop
		if v.Crop != nil {
			crop = &media.Crop{X: v.Crop.X, Y: v.Crop.Y, Width: v.Crop.Width, Height: v.Crop.Height}
		}
		g, err := media.PlanLayer(e.canvas, v.Asset.Width, v.Asset.Height, crop, v.Transform)
		if err != nil {
			return LayerPlan{}, fmt.Errorf("layer %s/%s: %w", v.Ref.TrackID, v.Ref.ClipID, err)
		}
		plan.Layers = append(plan.Layers, PlannedLayer{Layer: v, Geometry: g})
	}
	return plan, nil
}
