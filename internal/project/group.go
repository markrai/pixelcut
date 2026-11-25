package project

import (
	"errors"
	"math"
	"sort"
)

type GroupMove struct {
	Project Project
	Delta   int64
	Snapped bool
	Target  int64
}

func (p Project) selectedTrack(trackID string, ids []string) (int, map[string]bool, error) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || selected[id] {
			return -1, nil, errors.New("invalid clip selection")
		}
		selected[id] = true
	}
	if len(selected) == 0 {
		return -1, nil, errors.New("select at least one clip")
	}
	i := p.TrackIndex(trackID)
	if i < 0 {
		return -1, nil, ErrTrackNotFound
	}
	n := 0
	for _, c := range p.Tracks[i].Clips {
		if selected[c.ID] {
			n++
		}
	}
	if n != len(selected) {
		return -1, nil, ErrClipNotFound
	}
	return i, selected, nil
}

func (p Project) DeleteClips(trackID string, ids []string) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	i, selected, err := p.selectedTrack(trackID, ids)
	if err != nil {
		return p, err
	}
	q := p.Clone()
	clips := q.Tracks[i].Clips[:0]
	for _, c := range q.Tracks[i].Clips {
		if !selected[c.ID] {
			clips = append(clips, c)
		}
	}
	q.Tracks[i].Clips = clips
	if err = q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func (p Project) MoveClips(trackID string, ids []string, delta, marker, tolerance int64) (GroupMove, error) {
	result := GroupMove{Project: p}
	i, selected, err := p.selectedTrack(trackID, ids)
	if err != nil {
		return result, err
	}
	if err = p.Validate(); err != nil {
		return result, err
	}
	lo, hi := int64(math.MinInt64), int64(math.MaxInt64)
	targets := []int64{0}
	if marker >= 0 {
		targets = append(targets, marker)
	}
	var edges []int64
	for _, c := range p.Tracks[i].Clips {
		end := c.End()
		if selected[c.ID] {
			lo = max(lo, -c.Start)
			hi = min(hi, math.MaxInt64-end)
			edges = append(edges, c.Start, end)
		} else {
			targets = append(targets, c.Start, end)
		}
	}
	delta = max(lo, min(hi, delta))
	try := func(d int64) (Project, error) {
		q := p.Clone()
		for j := range q.Tracks[i].Clips {
			c := &q.Tracks[i].Clips[j]
			if selected[c.ID] {
				c.Start += d
			}
		}
		return q, q.Validate()
	}
	if tolerance > 0 {
		type candidate struct{ delta, target, distance int64 }
		var candidates []candidate
		sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })
		for _, edge := range edges {
			at := edge + delta
			lower := int64(0)
			if at > tolerance {
				lower = at - tolerance
			}
			upper := int64(math.MaxInt64)
			if at <= math.MaxInt64-tolerance {
				upper = at + tolerance
			}
			k := sort.Search(len(targets), func(k int) bool { return targets[k] >= lower })
			for ; k < len(targets) && targets[k] <= upper; k++ {
				target := targets[k]
				d := target - edge
				if d < lo || d > hi {
					continue
				}
				distance := target - at
				if distance < 0 {
					distance = -distance
				}
				candidates = append(candidates, candidate{d, target, distance})
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].distance != candidates[j].distance {
				return candidates[i].distance < candidates[j].distance
			}
			return candidates[i].target < candidates[j].target
		})
		seen := map[int64]bool{}
		for _, c := range candidates {
			if seen[c.delta] {
				continue
			}
			seen[c.delta] = true
			q, err := try(c.delta)
			if err == nil {
				return GroupMove{Project: q, Delta: c.delta, Snapped: true, Target: c.target}, nil
			}
		}
	}
	q, err := try(delta)
	return GroupMove{Project: q, Delta: delta}, err
}

func RangeClipIDs(clips []Clip, anchor, target string) []string {
	ordered := append([]Clip(nil), clips...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	a, b := -1, -1
	for i, c := range ordered {
		if c.ID == anchor {
			a = i
		}
		if c.ID == target {
			b = i
		}
	}
	if b < 0 {
		return nil
	}
	if a < 0 {
		a = b
	}
	ids := make([]string, 0)
	for i := min(a, b); i <= max(a, b); i++ {
		ids = append(ids, ordered[i].ID)
	}
	return ids
}
