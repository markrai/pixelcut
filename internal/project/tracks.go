package project

import (
	"errors"
	"fmt"
)

const MaxTracks = 32

func (p Project) EndTime() int64 {
	var end int64
	for _, t := range p.Tracks {
		end = max(end, t.EndTime())
	}
	return end
}

func (p Project) AddTrack(kind string) (Project, string, error) {
	if err := p.Validate(); err != nil {
		return p, "", err
	}
	if len(p.Tracks) >= MaxTracks {
		return p, "", errors.New("track limit reached")
	}
	prefix := "V"
	if kind == AudioTrack {
		prefix = "A"
	} else if kind != VideoTrack {
		return p, "", errors.New("invalid track kind")
	}
	id := ""
	for i := 2; i <= MaxTracks+1; i++ {
		candidate := fmt.Sprintf("%s%d", prefix, i)
		if p.TrackIndex(candidate) < 0 {
			id = candidate
			break
		}
	}
	if id == "" {
		return p, "", errors.New("no free track ID")
	}
	q := p.Clone()
	q.Version = Version
	q.Tracks = append(q.Tracks, Track{ID: id, Kind: kind, Clips: []Clip{}})
	return q, id, q.Validate()
}

func (p Project) RemoveTrack(id string) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	i := p.TrackIndex(id)
	if i < 2 {
		return p, errors.New("base tracks V1 and A1 cannot be removed")
	}
	if err := p.RequireEditableTrack(id); err != nil {
		return p, err
	}
	if len(p.Tracks[i].Clips) != 0 {
		return p, errors.New("remove or move the track's clips first")
	}
	q := p.Clone()
	q.Tracks = append(q.Tracks[:i], q.Tracks[i+1:]...)
	return q, q.Validate()
}

func (p Project) TransferClips(from, to string, ids []string) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	src, selected, err := p.selectedTrack(from, ids)
	if err != nil {
		return p, err
	}
	dst := p.TrackIndex(to)
	if dst < 0 || src == dst || p.Tracks[src].Kind != p.Tracks[dst].Kind {
		return p, errors.New("select a different track of the same kind")
	}
	if err := p.RequireEditableTrack(from); err != nil {
		return p, err
	}
	if err := p.RequireEditableTrack(to); err != nil {
		return p, err
	}
	q := p.Clone()
	q.Version = Version
	kept := q.Tracks[src].Clips[:0]
	for _, c := range q.Tracks[src].Clips {
		if selected[c.ID] {
			q.Tracks[dst].Clips = append(q.Tracks[dst].Clips, c)
		} else {
			kept = append(kept, c)
		}
	}
	q.Tracks[src].Clips = kept
	if err = q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}
