package project

import (
	"errors"
	"math"
)

func (p Project) DragClip(trackID, clipID, mode string, delta int64) (Clip, error) {
	if err := p.Validate(); err != nil {
		return Clip{}, err
	}
	return p.dragClip(trackID, clipID, mode, delta)
}

func (p Project) dragClip(trackID, clipID, mode string, delta int64) (Clip, error) {
	ti, ci, ok := p.ClipIndex(ClipRef{TrackID: trackID, ClipID: clipID})
	if !ok {
		return Clip{}, ErrClipNotFound
	}
	c := p.Tracks[ti].Clips[ci].Clone()
	a, _ := p.Asset(c.AssetID)
	switch mode {
	case "move":
		if delta < -c.Start {
			delta = -c.Start
		}
		if delta > math.MaxInt64-c.Start {
			delta = math.MaxInt64 - c.Start
		}
		c.Start += delta
	case "left":
		if delta < -c.In {
			delta = -c.In
		}
		if delta < -c.Start {
			delta = -c.Start
		}
		if delta >= c.Duration() {
			delta = c.Duration() - 1
		}
		if delta > math.MaxInt64-c.Start {
			return c, errors.New("timeline overflow")
		}
		c.In += delta
		c.Start += delta
	case "right":
		if delta > a.Duration-c.Out {
			delta = a.Duration - c.Out
		}
		if delta <= c.In-c.Out {
			delta = c.In - c.Out + 1
		}
		c.Out += delta
	default:
		return c, errors.New("unknown drag mode")
	}
	q := p.Clone()
	q.Tracks[ti].Clips[ci] = c
	return c, q.Validate()
}
