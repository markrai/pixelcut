package project

import (
	"errors"
	"fmt"
)

const MaxVideoTransitionDuration = 10 * TicksPerSecond
const CrossDissolve = "cross-dissolve"

var ErrInvalidTransition = errors.New("invalid video transition")

type VideoTransition struct {
	Kind       string `json:"kind"`
	FromClipID string `json:"from_clip_id"`
	Duration   int64  `json:"duration_us"`
}

func validateVideoTransition(from, to Clip, source Asset) error {
	t := to.TransitionIn
	if t == nil {
		return nil
	}
	if t.Kind != CrossDissolve || t.FromClipID != from.ID || from.End() != to.Start || t.Duration <= 0 || t.Duration > MaxVideoTransitionDuration || t.Duration > to.Duration() {
		return fmt.Errorf("%w: dissolve requires adjacent clips and a bounded incoming duration", ErrInvalidTransition)
	}
	if source.Duration-from.Out < t.Duration {
		return fmt.Errorf("%w: outgoing clip lacks unused source footage after its cut", ErrInvalidTransition)
	}
	return nil
}

func (p Project) HasVideoTransitions() bool {
	for _, track := range p.Tracks {
		for _, clip := range track.Clips {
			if clip.TransitionIn != nil {
				return true
			}
		}
	}
	return false
}

func (p Project) ClipHasTransition(ref ClipRef) bool {
	i := p.TrackIndex(ref.TrackID)
	if i < 0 {
		return false
	}
	for _, c := range p.Tracks[i].Clips {
		if c.TransitionIn != nil && (c.ID == ref.ClipID || c.TransitionIn.FromClipID == ref.ClipID) {
			return true
		}
	}
	return false
}

func (p Project) SetVideoTransition(ref ClipRef, transition *VideoTransition) (Project, error) {
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
	old := p.Tracks[ti].Clips[ci].TransitionIn
	if (old == nil && transition == nil) || (old != nil && transition != nil && *old == *transition) {
		return p, nil
	}
	q := p.Clone()
	q.Tracks[ti].Clips[ci].TransitionIn = nil
	if transition != nil {
		v := *transition
		q.Tracks[ti].Clips[ci].TransitionIn = &v
		q.Version = Version
	}
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}
