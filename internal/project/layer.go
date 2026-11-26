package project

import (
	"errors"

	"github.com/markrai/pixelcut/internal/media"
)

func (c Clip) OpacityPercent() int {
	if c.Opacity == nil {
		return 100
	}
	return *c.Opacity
}

func (p Project) CompositionCanvas() media.CanvasSize {
	if p.CanvasWidth != 0 || p.CanvasHeight != 0 {
		return media.CanvasSize{Width: p.CanvasWidth, Height: p.CanvasHeight}
	}
	for _, t := range p.Tracks {
		if t.Kind != VideoTrack || len(t.Clips) == 0 {
			continue
		}
		c := t.Clips[0]
		for _, other := range t.Clips[1:] {
			if other.Start < c.Start {
				c = other
			}
		}
		a, ok := p.Asset(c.AssetID)
		if !ok {
			continue
		}
		w, h := a.Width, a.Height
		if c.Crop != nil {
			w, h = c.Crop.Width, c.Crop.Height
		}
		if c.Transform != nil {
			w, h = c.Transform.CanvasWidth, c.Transform.CanvasHeight
		}
		size := media.CanvasSize{Width: w, Height: h}
		if size.Validate() == nil {
			return size
		}
		if h > w {
			return media.CanvasSize{Width: 1080, Height: 1920}
		}
		if h == w {
			return media.CanvasSize{Width: 1080, Height: 1080}
		}
		break
	}
	return media.CanvasSize{Width: 1920, Height: 1080}
}

func (p Project) SetCompositionCanvas(size media.CanvasSize) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	if size != (media.CanvasSize{}) {
		if err := size.Validate(); err != nil {
			return p, err
		}
	}
	if p.CanvasWidth == size.Width && p.CanvasHeight == size.Height {
		return p, nil
	}
	q := p.Clone()
	q.Version = Version
	q.CanvasWidth, q.CanvasHeight = size.Width, size.Height
	return q, q.Validate()
}

func (p Project) SetClipOpacity(ref ClipRef, percent int) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return p, ErrClipNotFound
	}
	if p.Tracks[ti].Kind != VideoTrack || percent < 0 || percent > 100 {
		return p, ErrInvalidClip
	}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return p, err
	}
	if p.Tracks[ti].Clips[ci].OpacityPercent() == percent {
		return p, nil
	}
	q := p.Clone()
	q.Version = Version
	if percent == 100 {
		q.Tracks[ti].Clips[ci].Opacity = nil
	} else {
		q.Tracks[ti].Clips[ci].Opacity = &percent
	}
	return q, q.Validate()
}

func (p Project) ApplyLayerPreset(ref ClipRef, preset media.LayerPreset) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return p, ErrClipNotFound
	}
	if p.Tracks[ti].Kind != VideoTrack {
		return p, ErrInvalidTrack
	}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return p, err
	}
	c := p.Tracks[ti].Clips[ci]
	a, _ := p.Asset(c.AssetID)
	if a.Range != "" && a.Range != "tv" {
		return p, errors.New("layer placement presets currently require limited-range HLG")
	}
	canvas := p.CompositionCanvas()
	w, h := a.Width, a.Height
	if c.Crop != nil {
		w, h = c.Crop.Width, c.Crop.Height
	}
	transform, err := media.PresetTransform(canvas, w, h, c.Transform, preset)
	if err != nil {
		return p, err
	}
	q := p.Clone()
	q.Version = Version
	q.CanvasWidth, q.CanvasHeight = canvas.Width, canvas.Height
	q.Tracks[ti].Clips[ci].Transform = &transform
	return q, q.Validate()
}
