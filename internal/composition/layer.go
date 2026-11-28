package composition

import (
	"fmt"
	"sort"

	"github.com/markrai/pixelcut/internal/media"
)

type PlannedLayer struct {
	Layer    VideoLayer
	Geometry media.LayerGeometry
}

type PlannedTransition struct {
	Window TransitionWindow
	From   PlannedLayer
}

type LayerPlan struct {
	Time        int64
	Canvas      media.CanvasSize
	Layers      []PlannedLayer
	Transitions []PlannedTransition
}

func planVideoLayer(canvas media.CanvasSize, v VideoLayer) (PlannedLayer, error) {
	var crop *media.Crop
	if v.Crop != nil {
		crop = &media.Crop{X: v.Crop.X, Y: v.Crop.Y, Width: v.Crop.Width, Height: v.Crop.Height}
	}
	g, err := media.PlanLayer(canvas, v.Asset.Width, v.Asset.Height, crop, v.Transform)
	if err != nil {
		return PlannedLayer{}, fmt.Errorf("layer %s/%s: %w", v.Ref.TrackID, v.Ref.ClipID, err)
	}
	return PlannedLayer{Layer: v, Geometry: g}, nil
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
		layer, err := planVideoLayer(e.canvas, v)
		if err != nil {
			return LayerPlan{}, err
		}
		plan.Layers = append(plan.Layers, layer)
	}
	if e.transitions != nil {
		for _, sample := range e.transitions.At(at) {
			outgoing, err := e.planTransitionSource(sample)
			if err != nil {
				return LayerPlan{}, err
			}
			plan.Transitions = append(plan.Transitions, PlannedTransition{Window: sample.Window, From: outgoing})
		}
	}
	return plan, nil
}

func (e *Evaluator) planTransitionSource(sample TransitionSample) (PlannedLayer, error) {
	w := sample.Window
	for _, track := range e.tracks {
		if track.id != w.From.TrackID {
			continue
		}
		i := sort.Search(len(track.clips), func(i int) bool { return track.clips[i].clip.Start >= w.Timeline.Start })
		if i == 0 || i >= len(track.clips) || track.clips[i].clip.ID != w.To.ClipID || track.clips[i-1].clip.ID != w.From.ClipID {
			break
		}
		item := track.clips[i-1]
		c := item.clip.Clone()
		return planVideoLayer(e.canvas, VideoLayer{
			Contribution: Contribution{
				Ref: w.From, Asset: item.asset, Timeline: w.Timeline, SourceInterval: w.FromSource, SourceTime: sample.FromTime,
			},
			Crop: c.Crop, Transform: c.Transform, Opacity: c.OpacityPercent(),
		})
	}
	return PlannedLayer{}, fmt.Errorf("missing transition source %s/%s", w.From.TrackID, w.From.ClipID)
}
