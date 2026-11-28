package composition

import (
	"errors"
	"sort"

	"github.com/markrai/pixelcut/internal/project"
)

var ErrTransitionRenderingUnavailable = errors.New("video transition rendering is not available yet")

const TransitionWeightScale int64 = 1000000

type TransitionWindow struct {
	From, To             project.ClipRef
	Timeline             project.Interval
	FromSource, ToSource project.Interval
}

type TransitionSample struct {
	Window           TransitionWindow
	FromTime, ToTime int64
	IncomingWeight   int64
}

func (w TransitionWindow) At(at int64) (TransitionSample, bool) {
	if !w.Timeline.Contains(at) || !w.FromSource.Valid() || !w.ToSource.Valid() {
		return TransitionSample{}, false
	}
	duration := w.Timeline.End - w.Timeline.Start
	if duration > project.MaxVideoTransitionDuration || w.FromSource.End-w.FromSource.Start != duration || w.ToSource.End-w.ToSource.Start != duration {
		return TransitionSample{}, false
	}
	offset := at - w.Timeline.Start
	return TransitionSample{Window: w, FromTime: w.FromSource.Start + offset, ToTime: w.ToSource.Start + offset, IncomingWeight: offset * TransitionWeightScale / duration}, true
}

type TransitionEvaluator struct {
	tracks [][]TransitionWindow
}

func CompileTransitions(p project.Project) (*TransitionEvaluator, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	e := &TransitionEvaluator{}
	policies := p.TrackPolicies()
	for _, track := range p.Tracks {
		if !policies[track.ID].Video {
			continue
		}
		clips := append([]project.Clip(nil), track.Clips...)
		sort.Slice(clips, func(i, j int) bool { return clips[i].Start < clips[j].Start })
		var windows []TransitionWindow
		for i, to := range clips {
			if to.TransitionIn == nil {
				continue
			}
			from := clips[i-1]
			duration := to.TransitionIn.Duration
			windows = append(windows, TransitionWindow{
				From:       project.ClipRef{TrackID: track.ID, ClipID: from.ID},
				To:         project.ClipRef{TrackID: track.ID, ClipID: to.ID},
				Timeline:   project.Interval{Start: to.Start, End: to.Start + duration},
				FromSource: project.Interval{Start: from.Out, End: from.Out + duration},
				ToSource:   project.Interval{Start: to.In, End: to.In + duration},
			})
		}
		if len(windows) != 0 {
			e.tracks = append(e.tracks, windows)
		}
	}
	return e, nil
}

func (e *TransitionEvaluator) At(at int64) []TransitionSample {
	var samples []TransitionSample
	for _, windows := range e.tracks {
		i := sort.Search(len(windows), func(i int) bool { return windows[i].Timeline.End > at })
		if i < len(windows) {
			if sample, ok := windows[i].At(at); ok {
				samples = append(samples, sample)
			}
		}
	}
	return samples
}
