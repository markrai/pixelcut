package project

import (
	"errors"
	"math"
	"sort"
)

type RippleEdit struct {
	Project Project
	Delta   int64
	Snapped bool
	Target  int64
}

func (p Project) RippleDelete(trackID string, ids []string) (Project, error) {
	ti, selected, err := p.selectedTrack(trackID, ids)
	if err != nil {
		return p, err
	}
	if err = p.Validate(); err != nil {
		return p, err
	}
	if err := p.RequireEditableTrack(trackID); err != nil {
		return p, err
	}
	q := p.Clone()
	ordered := append([]Clip(nil), q.Tracks[ti].Clips...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	var shift int64
	positions := map[string]int64{}
	for _, c := range ordered {
		if selected[c.ID] {
			shift += c.Duration()
		} else {
			positions[c.ID] = c.Start - shift
		}
	}
	kept := q.Tracks[ti].Clips[:0]
	for _, c := range q.Tracks[ti].Clips {
		if selected[c.ID] {
			continue
		}
		c.Start = positions[c.ID]
		kept = append(kept, c)
	}
	q.Tracks[ti].Clips = kept
	if err = q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func (p Project) RemoveGap(trackID string, at int64) (Project, int64, error) {
	if at < 0 {
		return p, 0, errors.New("invalid gap position")
	}
	if err := p.Validate(); err != nil {
		return p, 0, err
	}
	for i, t := range p.Tracks {
		if t.ID != trackID {
			continue
		}
		if err := p.RequireEditableTrack(trackID); err != nil {
			return p, 0, err
		}
		var left, right int64
		found := false
		for _, c := range t.Clips {
			end := c.End()
			if c.Contains(at) {
				return p, 0, errors.New("place the playhead in a gap")
			}
			if end <= at {
				left = max(left, end)
			}
			if c.Start > at && (!found || c.Start < right) {
				right = c.Start
				found = true
			}
		}
		if !found || right <= left {
			return p, 0, errors.New("no bounded gap at playhead")
		}
		q := p.Clone()
		for j := range q.Tracks[i].Clips {
			c := &q.Tracks[i].Clips[j]
			if c.Start >= right {
				c.Start -= right - left
			}
		}
		if err := q.Validate(); err != nil {
			return p, 0, err
		}
		return q, right - left, nil
	}
	return p, 0, ErrTrackNotFound
}

func (p Project) RippleTrim(trackID, clipID, mode string, delta, marker, tolerance int64) (RippleEdit, error) {
	result := RippleEdit{Project: p}
	if mode != "left" && mode != "right" {
		return result, errors.New("invalid ripple trim edge")
	}
	if err := p.Validate(); err != nil {
		return result, err
	}
	ti, _, err := p.selectedTrack(trackID, []string{clipID})
	if err != nil {
		return result, err
	}
	if err := p.RequireEditableTrack(trackID); err != nil {
		return result, err
	}
	var original Clip
	for _, c := range p.Tracks[ti].Clips {
		if c.ID == clipID {
			original = c
			break
		}
	}
	a, _ := p.Asset(original.AssetID)
	duration := original.Duration()
	oldEnd := original.End()
	last := oldEnd
	for _, c := range p.Tracks[ti].Clips {
		last = max(last, c.End())
	}
	extension := int64(math.MaxInt64) - last
	lo, hi := int64(0), int64(0)
	base := oldEnd
	if mode == "right" {
		lo = 1 - duration
		hi = min(a.Duration-original.Out, extension)
	} else {
		lo = -min(original.In, extension)
		hi = duration - 1
		base = original.Start
	}
	delta = max(lo, min(hi, delta))
	if tolerance > 0 && marker >= 0 {
		d := marker - base
		if d >= lo && d <= hi {
			distance := float64(d) - float64(delta)
			if math.Abs(distance) <= float64(tolerance) {
				delta = d
				result.Snapped = true
				result.Target = marker
			}
		}
	}
	q := p.Clone()
	shift := delta
	if mode == "left" {
		shift = -delta
	}
	for j := range q.Tracks[ti].Clips {
		c := &q.Tracks[ti].Clips[j]
		if c.ID == clipID {
			if mode == "left" {
				c.In += delta
			} else {
				c.Out += delta
			}
		} else if c.Start >= oldEnd {
			c.Start += shift
		}
	}
	if err = q.Validate(); err != nil {
		return result, err
	}
	result.Project = q
	result.Delta = delta
	return result, nil
}
