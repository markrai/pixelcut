package project

import (
	"math/bits"
	"slices"
	"sort"
)

const MaxVolumeKeyframes = 128
const EnvelopeScale int64 = 1000000

type VolumeKeyframe struct {
	SourceTime int64 `json:"source_us"`
	Percent    int   `json:"percent"`
}

type VolumeEnvelope struct {
	Points []VolumeKeyframe `json:"points"`
}

func (e *VolumeEnvelope) Clone() *VolumeEnvelope {
	if e == nil {
		return nil
	}
	return &VolumeEnvelope{Points: slices.Clone(e.Points)}
}

func (e *VolumeEnvelope) Valid(duration int64) bool {
	if e == nil {
		return true
	}
	if duration <= 0 || len(e.Points) == 0 || len(e.Points) > MaxVolumeKeyframes {
		return false
	}
	var last int64 = -1
	for _, p := range e.Points {
		if p.SourceTime <= last || p.SourceTime > duration || p.Percent < 0 || p.Percent > 100 {
			return false
		}
		last = p.SourceTime
	}
	return true
}

func (e *VolumeEnvelope) Factor(at int64) int64 {
	if e == nil || len(e.Points) == 0 {
		return EnvelopeScale
	}
	i := sort.Search(len(e.Points), func(i int) bool { return e.Points[i].SourceTime >= at })
	if i == 0 {
		return int64(e.Points[0].Percent) * (EnvelopeScale / 100)
	}
	if i == len(e.Points) {
		return int64(e.Points[i-1].Percent) * (EnvelopeScale / 100)
	}
	a, b := e.Points[i-1], e.Points[i]
	first := int64(a.Percent) * (EnvelopeScale / 100)
	delta := int64(b.Percent-a.Percent) * (EnvelopeScale / 100)
	span := b.SourceTime - a.SourceTime
	hi, lo := bits.Mul64(uint64(at-a.SourceTime), uint64(max(delta, -delta)))
	q, _ := bits.Div64(hi, lo, uint64(span))
	if delta < 0 {
		return first - int64(q)
	}
	return first + int64(q)
}

func (c Clip) HasEnvelope() bool {
	return c.Envelope != nil
}

func (p Project) SetClipEnvelope(ref ClipRef, points []VolumeKeyframe) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return p, ErrClipNotFound
	}
	c := p.Tracks[ti].Clips[ci]
	a, _ := p.Asset(c.AssetID)
	if !a.HasAudio || c.AudioDetached {
		return p, ErrInvalidClip
	}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return p, err
	}
	var env *VolumeEnvelope
	if len(points) > 0 {
		env = &VolumeEnvelope{Points: points}
		if !env.Valid(a.Duration) {
			return p, ErrInvalidClip
		}
	}
	if (env == nil && c.Envelope == nil) || (env != nil && c.Envelope != nil && slices.Equal(env.Points, c.Envelope.Points)) {
		return p, nil
	}
	q := p.Clone()
	q.Version = Version
	q.Tracks[ti].Clips[ci].Envelope = env.Clone()
	return q, q.Validate()
}
