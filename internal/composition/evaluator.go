package composition

import (
	"errors"
	"sort"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

var ErrOutsideTimeline = errors.New("timestamp outside composition timeline")

type Contribution struct {
	Ref            project.ClipRef
	Asset          project.Asset
	Timeline       project.Interval
	SourceInterval project.Interval
	SourceTime     int64
}

type VideoLayer struct {
	Contribution
	Crop      *project.Crop
	Transform *media.Transform
	Opacity   int
}

type AudioContribution struct {
	Contribution
	Linked      bool
	GainPercent int
}

type Evaluation struct {
	Time         int64
	End          int64
	NextBoundary int64
	Video        []VideoLayer
	Audio        []AudioContribution
}

type entry struct {
	clip  project.Clip
	asset project.Asset
}

type track struct {
	id, kind string
	clips    []entry
	policy   project.TrackPolicy
}

type Evaluator struct {
	duration        int64
	canvas          media.CanvasSize
	tracks          []track
	boundaries      []int64
	videoBoundaries []int64
}

func Compile(p project.Project) (*Evaluator, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	e := &Evaluator{duration: p.EndTime(), canvas: p.CompositionCanvas(), boundaries: []int64{0}}
	assets := make(map[string]project.Asset, len(p.Assets))
	for _, a := range p.Assets {
		assets[a.ID] = a
	}
	policies := p.TrackPolicies()
	for _, t := range p.Tracks {
		indexed := track{id: t.ID, kind: t.Kind, clips: make([]entry, 0, len(t.Clips)), policy: policies[t.ID]}
		for _, c := range t.Clips {
			indexed.clips = append(indexed.clips, entry{clip: c.Clone(), asset: assets[c.AssetID]})
			e.boundaries = append(e.boundaries, c.Start, c.End())
			if indexed.policy.Video && c.OpacityPercent() > 0 {
				e.videoBoundaries = append(e.videoBoundaries, c.Start, c.End())
			}
		}
		sort.Slice(indexed.clips, func(i, j int) bool {
			return indexed.clips[i].clip.Start < indexed.clips[j].clip.Start
		})
		e.tracks = append(e.tracks, indexed)
	}
	sort.Slice(e.boundaries, func(i, j int) bool { return e.boundaries[i] < e.boundaries[j] })
	unique := e.boundaries[:0]
	for _, at := range e.boundaries {
		if len(unique) == 0 || unique[len(unique)-1] != at {
			unique = append(unique, at)
		}
	}
	e.boundaries = unique
	sort.Slice(e.videoBoundaries, func(i, j int) bool { return e.videoBoundaries[i] < e.videoBoundaries[j] })
	return e, nil
}

func (e *Evaluator) Duration() int64 { return e.duration }

func (e *Evaluator) At(at int64) (Evaluation, error) {
	if at < 0 || at > e.duration {
		return Evaluation{}, ErrOutsideTimeline
	}
	f := Evaluation{Time: at, End: e.duration, NextBoundary: e.duration}
	if at == e.duration {
		return f, nil
	}
	if i := sort.Search(len(e.boundaries), func(i int) bool { return e.boundaries[i] > at }); i < len(e.boundaries) {
		f.NextBoundary = e.boundaries[i]
	}
	for _, t := range e.tracks {
		i := sort.Search(len(t.clips), func(i int) bool { return t.clips[i].clip.End() > at })
		if i == len(t.clips) {
			continue
		}
		item := t.clips[i]
		c := item.clip
		sourceTime, ok := c.SourceAt(at)
		if !ok {
			continue
		}
		interval, _ := c.TimelineInterval()
		source := Contribution{
			Ref:            project.ClipRef{TrackID: t.id, ClipID: c.ID},
			Asset:          item.asset,
			Timeline:       interval,
			SourceInterval: c.SourceInterval(),
			SourceTime:     sourceTime,
		}
		if t.policy.Video {
			c = c.Clone()
			f.Video = append(f.Video, VideoLayer{Contribution: source, Crop: c.Crop, Transform: c.Transform, Opacity: c.OpacityPercent()})
		}
		if t.policy.Audio && item.asset.HasAudio && !c.AudioDetached && c.GainPercent() > 0 {
			f.Audio = append(f.Audio, AudioContribution{Contribution: source, Linked: t.kind == project.VideoTrack, GainPercent: c.GainPercent()})
		}
	}
	return f, nil
}

func (e *Evaluator) AuditionTarget(trackID string, at int64, advance bool) (project.ClipRef, int64, bool) {
	if at < 0 || at >= e.duration {
		return project.ClipRef{}, at, false
	}
	for _, t := range e.tracks {
		if t.id != trackID || (!t.policy.Video && !t.policy.Audio) {
			continue
		}
		i := sort.Search(len(t.clips), func(i int) bool { return t.clips[i].clip.End() > at })
		for ; i < len(t.clips); i++ {
			item := t.clips[i]
			c := item.clip
			if c.Start > at && !advance {
				break
			}
			if t.policy.Video || (t.policy.Audio && item.asset.HasAudio && !c.AudioDetached && c.GainPercent() > 0) {
				return project.ClipRef{TrackID: t.id, ClipID: c.ID}, max(at, c.Start), true
			}
			if !advance {
				break
			}
		}
		break
	}
	return project.ClipRef{}, at, false
}

func (e *Evaluator) AudioTarget(trackID string, at int64, advance bool) (AudioContribution, int64, bool) {
	if at < 0 || at >= e.duration {
		return AudioContribution{}, at, false
	}
	for _, t := range e.tracks {
		if t.id != trackID || t.kind != project.AudioTrack {
			continue
		}
		i := sort.Search(len(t.clips), func(i int) bool { return t.clips[i].clip.End() > at })
		if i == len(t.clips) {
			break
		}
		c := t.clips[i].clip
		if at < c.Start {
			if !advance {
				break
			}
			at = c.Start
		}
		f, err := e.At(at)
		if err != nil {
			break
		}
		for _, a := range f.Audio {
			if a.Ref.TrackID == trackID && !a.Linked {
				return a, at, true
			}
		}
		break
	}
	return AudioContribution{}, at, false
}

func (e *Evaluator) VideoTarget(trackID string, at int64, advance bool) (VideoLayer, int64, bool) {
	if at < 0 || at >= e.duration {
		return VideoLayer{}, at, false
	}
	for _, t := range e.tracks {
		if t.id != trackID || t.kind != project.VideoTrack {
			continue
		}
		i := sort.Search(len(t.clips), func(i int) bool { return t.clips[i].clip.End() > at })
		if i == len(t.clips) {
			break
		}
		item := t.clips[i]
		c := item.clip
		if at < c.Start {
			if !advance {
				break
			}
			at = c.Start
		}
		f, err := e.At(at)
		if err != nil {
			break
		}
		for _, layer := range f.Video {
			if layer.Ref.TrackID == trackID {
				return layer, at, true
			}
		}
		break
	}
	return VideoLayer{}, at, false
}
