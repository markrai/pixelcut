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
}

type AudioContribution struct {
	Contribution
	Linked bool
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
}

type Evaluator struct {
	duration   int64
	tracks     []track
	boundaries []int64
}

func Compile(p project.Project) (*Evaluator, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	e := &Evaluator{duration: p.EndTime(), boundaries: []int64{0}}
	assets := make(map[string]project.Asset, len(p.Assets))
	for _, a := range p.Assets {
		assets[a.ID] = a
	}
	for _, t := range p.Tracks {
		indexed := track{id: t.ID, kind: t.Kind, clips: make([]entry, 0, len(t.Clips))}
		for _, c := range t.Clips {
			indexed.clips = append(indexed.clips, entry{clip: c.Clone(), asset: assets[c.AssetID]})
			e.boundaries = append(e.boundaries, c.Start, c.End())
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
		if t.kind == project.VideoTrack {
			c = c.Clone()
			f.Video = append(f.Video, VideoLayer{Contribution: source, Crop: c.Crop, Transform: c.Transform})
		}
		if item.asset.HasAudio {
			f.Audio = append(f.Audio, AudioContribution{Contribution: source, Linked: t.kind == project.VideoTrack})
		}
	}
	return f, nil
}
