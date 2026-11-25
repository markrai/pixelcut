package project

import (
	"errors"
	"math"
	"sort"
)

type AssetPlacement struct {
	Project    Project
	Start, End int64
	Snapped    bool
	Target     int64
	FirstClip  int
}

func (p Project) PlaceAssets(trackID string, assetIDs, clipIDs []string, start, marker, tolerance int64) (AssetPlacement, error) {
	result := AssetPlacement{Project: p, Start: max(0, start), FirstClip: -1}
	if len(assetIDs) == 0 || len(assetIDs) != len(clipIDs) || len(assetIDs) > MaxImportFiles {
		return result, errors.New("invalid asset placement batch")
	}
	ti := -1
	for i, t := range p.Tracks {
		if t.ID == trackID {
			ti = i
			break
		}
	}
	if ti < 0 {
		return result, errors.New("track not found")
	}
	var duration int64
	clips := make([]Clip, 0, len(assetIDs))
	for i, id := range assetIDs {
		a, ok := p.Asset(id)
		if !ok || (p.Tracks[ti].Kind == "audio" && !a.HasAudio) || a.Duration <= 0 || a.Duration > math.MaxInt64-duration {
			return result, errors.New("invalid placement source or duration")
		}
		clips = append(clips, Clip{ID: clipIDs[i], AssetID: id, Start: duration, Out: a.Duration})
		duration += a.Duration
	}
	result.Start = min(result.Start, math.MaxInt64-duration)
	result.End = result.Start + duration
	type candidate struct{ start, target, distance int64 }
	var candidates []candidate
	if tolerance > 0 {
		targets := []int64{0}
		if marker >= 0 {
			targets = append(targets, marker)
		}
		for _, c := range p.Tracks[ti].Clips {
			if edge, ok := clipEdge(c, false); ok {
				targets = append(targets, edge)
			}
			if edge, ok := clipEdge(c, true); ok {
				targets = append(targets, edge)
			}
		}
		for _, target := range targets {
			for _, offset := range []int64{0, duration} {
				if target < offset {
					continue
				}
				at := target - offset
				if at > math.MaxInt64-duration {
					continue
				}
				d := at - result.Start
				if d < 0 {
					d = -d
				}
				if d <= tolerance {
					candidates = append(candidates, candidate{at, target, d})
				}
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].distance != candidates[j].distance {
				return candidates[i].distance < candidates[j].distance
			}
			return candidates[i].target < candidates[j].target
		})
	}
	try := func(at int64) (Project, error) {
		q := p.Clone()
		if q.Tracks[ti].Kind == "audio" {
			q.Version = Version
		}
		for _, c := range clips {
			c.Start += at
			q.Tracks[ti].Clips = append(q.Tracks[ti].Clips, c)
		}
		return q, q.Validate()
	}
	for _, c := range candidates {
		q, err := try(c.start)
		if err == nil {
			return AssetPlacement{Project: q, Start: c.start, End: c.start + duration, Snapped: true, Target: c.target, FirstClip: len(p.Tracks[ti].Clips)}, nil
		}
	}
	q, err := try(result.Start)
	if err != nil {
		return result, err
	}
	result.Project = q
	result.FirstClip = len(p.Tracks[ti].Clips)
	return result, nil
}
