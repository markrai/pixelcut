package project

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrTrackLocked = errors.New("track is locked")

type TrackControl string

const (
	Visibility TrackControl = "visibility"
	Mute       TrackControl = "mute"
	Solo       TrackControl = "solo"
	Lock       TrackControl = "lock"
)

type TrackPolicy struct {
	Video, Audio bool
}

func (p Project) TrackPolicies() map[string]TrackPolicy {
	solo := false
	for _, t := range p.Tracks {
		solo = solo || t.Solo
	}
	policies := make(map[string]TrackPolicy, len(p.Tracks))
	for _, t := range p.Tracks {
		included := !solo || t.Solo
		policies[t.ID] = TrackPolicy{
			Video: included && t.Kind == VideoTrack && !t.Hidden,
			Audio: included && !t.Muted,
		}
	}
	return policies
}

func (p Project) ToggleTrackControl(id string, control TrackControl) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	i := p.TrackIndex(id)
	if i < 0 {
		return p, ErrTrackNotFound
	}
	q := p.Clone()
	t := &q.Tracks[i]
	switch control {
	case Visibility:
		if t.Kind != VideoTrack {
			return p, ErrInvalidTrack
		}
		t.Hidden = !t.Hidden
	case Mute:
		t.Muted = !t.Muted
	case Solo:
		t.Solo = !t.Solo
	case Lock:
		t.Locked = !t.Locked
	default:
		return p, errors.New("unknown track control")
	}
	q.Version = Version
	return q, q.Validate()
}

func (p Project) RequireEditableTrack(id string) error {
	i := p.TrackIndex(id)
	if i < 0 {
		return ErrTrackNotFound
	}
	if p.Tracks[i].Locked {
		return fmt.Errorf("%w: %s", ErrTrackLocked, id)
	}
	return nil
}

func (p Project) RequireEditableAsset(id string) error {
	for _, t := range p.Tracks {
		if !t.Locked {
			continue
		}
		for _, c := range t.Clips {
			if c.AssetID == id {
				return fmt.Errorf("%w: %s references this asset", ErrTrackLocked, t.ID)
			}
		}
	}
	return nil
}

func (p Project) ValidateLockedEdit(q Project) error {
	for _, t := range p.Tracks {
		if !t.Locked {
			continue
		}
		i := q.TrackIndex(t.ID)
		if i < 0 || q.Tracks[i].Kind != t.Kind || !reflect.DeepEqual(t.Clips, q.Tracks[i].Clips) {
			return fmt.Errorf("%w: %s", ErrTrackLocked, t.ID)
		}
		for _, c := range t.Clips {
			a, ok := p.Asset(c.AssetID)
			b, present := q.Asset(c.AssetID)
			if !ok || !present || a != b {
				return fmt.Errorf("%w: %s references a changed asset", ErrTrackLocked, t.ID)
			}
		}
	}
	return nil
}
