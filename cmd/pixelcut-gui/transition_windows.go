package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/projectexport"
	"github.com/markrai/pixelcut/internal/timeline"
)

const (
	menuTransitionDuration = 2380
	menuTransitionRemove   = 2381
	transitionDurationBase = 2390
)

var transitionPresets = [...]int64{100000, 250000, 500000, 1000000, 2000000, 3000000, 5000000, 10000000}

func transitionHandle(p project.Project, ref project.ClipRef) (project.Asset, int64, error) {
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return project.Asset{}, 0, project.ErrClipNotFound
	}
	t := p.Tracks[ti]
	if t.Kind != project.VideoTrack {
		return project.Asset{}, 0, project.ErrInvalidTrack
	}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return project.Asset{}, 0, err
	}
	to := t.Clips[ci]
	for _, from := range t.Clips {
		if from.ID == to.ID || from.End() != to.Start {
			continue
		}
		a, ok := p.Asset(from.AssetID)
		if !ok {
			return project.Asset{}, 0, project.ErrAssetNotFound
		}
		limit := min(project.MaxVideoTransitionDuration, to.Duration(), a.Duration-from.Out)
		if limit <= 0 {
			return a, 0, errors.New("preceding clip has no unused source footage after its cut")
		}
		return a, limit, nil
	}
	return project.Asset{}, 0, errors.New("cross-dissolve requires a preceding adjacent clip on this track")
}

func transitionChoices(limit, current int64) []int64 {
	var choices []int64
	for _, d := range transitionPresets {
		if d <= limit {
			choices = append(choices, d)
		}
	}
	for _, d := range []int64{limit, current} {
		if d > 0 && d <= limit && !slices.Contains(choices, d) {
			choices = append(choices, d)
		}
	}
	slices.Sort(choices)
	return choices
}

func transitionDurationItems(choices []int64, current int64) []menuItem {
	items := make([]menuItem, 0, len(choices))
	for i, d := range choices {
		label := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", project.Seconds(d)), "0"), ".") + " s"
		items = append(items, menuItem{id: transitionDurationBase + i, label: label, enabled: true, checked: d == current})
	}
	return items
}

func applyTransitionDuration(p project.Project, ref project.ClipRef, duration int64) (project.Project, error) {
	if duration == 0 {
		return p.SetVideoTransition(ref, nil)
	}
	if !p.TrackPolicies()[ref.TrackID].Video {
		return p, errors.New("enable the video track before adding or changing a transition")
	}
	_, limit, err := transitionHandle(p, ref)
	if err != nil {
		return p, err
	}
	if duration < 0 || duration > limit {
		return p, project.ErrInvalidTransition
	}
	ti, ci, _ := p.ClipIndex(ref)
	to := p.Tracks[ti].Clips[ci]
	for _, from := range p.Tracks[ti].Clips {
		if from.ID == to.ID || from.End() != to.Start {
			continue
		}
		q, err := p.SetVideoTransition(ref, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: from.ID, Duration: duration})
		if err != nil {
			return p, err
		}
		if _, err := projectexport.Build(q, "hlg", 30); err != nil {
			return p, fmt.Errorf("cross-dissolve cannot be rendered: %w", err)
		}
		return q, nil
	}
	return p, project.ErrInvalidTransition
}

func (e *editor) transitionAction(id int, screen point) error {
	if err := e.availability().Check(commandRequirement(id)); err != nil {
		return err
	}
	if err := trackCommandAllowed(id, e.availability()); err != nil {
		return err
	}
	p := e.model.Project()
	c, ok := e.selected()
	if !ok || e.model.Count() != 1 || e.audioTrack() {
		return errors.New("select a single incoming video clip")
	}
	ref := project.ClipRef{TrackID: e.trackID(), ClipID: c.ID}
	var duration int64
	switch id {
	case menuTransitionDuration:
		from, limit, err := transitionHandle(p, ref)
		if err != nil {
			return err
		}
		to, ok := p.Asset(c.AssetID)
		if !ok || !project.SourceAvailable(from.Path) || !project.SourceAvailable(to.Path) {
			return errors.New("both transition sources must be available")
		}
		var current int64
		if c.TransitionIn != nil {
			current = c.TransitionIn.Duration
		}
		choices := transitionChoices(limit, current)
		choice := e.popup(transitionDurationItems(choices, current), screen) - transitionDurationBase
		if choice < 0 || choice >= len(choices) {
			return nil
		}
		duration = choices[choice]
	case menuTransitionRemove:
		if c.TransitionIn == nil {
			return nil
		}
	default:
		return nil
	}
	q, err := applyTransitionDuration(p, ref, duration)
	if err != nil {
		return err
	}
	if q.Fingerprint() == p.Fingerprint() {
		return nil
	}
	if err := e.commit(q); err != nil {
		return err
	}
	e.playhead = c.Start + duration/2
	if err := e.showCompositeStill(); err != nil {
		e.tell("Transition edit applied; preview unavailable: " + err.Error())
		return nil
	}
	if duration == 0 {
		e.tell("Cross-dissolve removed. Audio unchanged.")
	} else {
		e.tell(fmt.Sprintf("Cross-dissolve %.3g s. Composite preview; audio cut unchanged.", project.Seconds(duration)))
	}
	return nil
}

func transitionMarker(view timeline.View, clip project.Clip) (timeline.Span, bool) {
	if clip.TransitionIn == nil || clip.TransitionIn.Duration <= 0 || clip.TransitionIn.Duration > clip.Duration() {
		return timeline.Span{}, false
	}
	return view.Span(clip.Start, clip.Start+clip.TransitionIn.Duration)
}

func paintTransitionMarker(dc uintptr, span timeline.Span, duration int64, enabled bool) {
	left, right := span.Left, span.Right
	bg, fg := rgb(99, 70, 132), rgb(240, 216, 255)
	if !enabled {
		bg, fg = palette.line, palette.muted
	}
	top, bottom := int32(laneTop+20), int32(laneTop+39)
	bottom = min(bottom, int32(laneBottom))
	fill(dc, rect{left, top, right, bottom}, bg)
	fill(dc, rect{left, top, min(right, left+2), bottom}, fg)
	fill(dc, rect{max(left, right-1), top, right, bottom}, fg)
	if right-left >= 62 {
		textAt(dc, rect{left + 4, top, right - 3, bottom}, fmt.Sprintf("Dissolve %gs", project.Seconds(duration)), fg, 4)
	}
}
