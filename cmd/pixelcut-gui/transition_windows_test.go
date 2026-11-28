package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

func transitionControlFixture(t *testing.T) (project.Project, project.ClipRef) {
	t.Helper()
	p := project.New()
	p.Assets = []project.Asset{
		{ID: "a", Path: filepath.Join(t.TempDir(), "first.mp4"), Duration: 2750000, Width: 192, Height: 144, Range: "tv", HasAudio: true},
		{ID: "b", Path: filepath.Join(t.TempDir(), "next.mp4"), Duration: 4000000, Width: 192, Height: 144, Range: "tv", HasAudio: true},
	}
	p.CanvasWidth, p.CanvasHeight = 192, 144
	p.Tracks[0].Clips = []project.Clip{
		{ID: "next", AssetID: "b", Start: 2000000, In: 1000000, Out: 3000000},
		{ID: "first", AssetID: "a", Out: 2000000},
	}
	return p, project.ClipRef{TrackID: "V1", ClipID: "next"}
}

func TestTransitionDurationControls(t *testing.T) {
	p, ref := transitionControlFixture(t)
	a, limit, err := transitionHandle(p, ref)
	if err != nil || a.ID != "a" || limit != 750000 {
		t.Fatalf("source handle: %+v %d %v", a, limit, err)
	}
	choices := transitionChoices(limit, 333333)
	want := []int64{100000, 250000, 333333, 500000, 750000}
	if !reflect.DeepEqual(choices, want) {
		t.Fatalf("duration choices: %v", choices)
	}
	items := transitionDurationItems(choices, 333333)
	if !items[2].checked || items[2].label != "0.333333 s" || items[4].label != "0.75 s" {
		t.Fatalf("duration labels: %+v", items)
	}
	if got := transitionChoices(1000, 0); !reflect.DeepEqual(got, []int64{1000}) {
		t.Fatalf("short handle not selectable: %v", got)
	}
	q, err := applyTransitionDuration(p, ref, 500000)
	if err != nil || q.EndTime() != p.EndTime() || p.HasVideoTransitions() {
		t.Fatalf("add transition: %v", err)
	}
	c, ok := q.Clip(ref)
	if !ok || c.TransitionIn == nil || c.TransitionIn.FromClipID != "first" || c.TransitionIn.Duration != 500000 {
		t.Fatalf("incorrect transition: %+v", c)
	}
	updated, err := applyTransitionDuration(q, ref, 750000)
	if err != nil || updated.EndTime() != q.EndTime() {
		t.Fatalf("resize: %v", err)
	}
	c, _ = updated.Clip(ref)
	if c.TransitionIn.Duration != 750000 {
		t.Fatal("duration did not change")
	}
	removed, err := applyTransitionDuration(updated, ref, 0)
	if err != nil || removed.HasVideoTransitions() || removed.EndTime() != p.EndTime() {
		t.Fatalf("remove: %v", err)
	}
	if got, err := applyTransitionDuration(q, ref, 750001); !errors.Is(err, project.ErrInvalidTransition) || got.Fingerprint() != q.Fingerprint() {
		t.Fatalf("invalid duration changed project: %v", err)
	}
}

func TestTransitionControlsRejectUnsupportedEdits(t *testing.T) {
	p, ref := transitionControlFixture(t)
	gap := p.Clone()
	gap.Tracks[0].Clips[0].Start++
	if _, _, err := transitionHandle(gap, ref); err == nil {
		t.Fatal("gap accepted")
	}
	noTail := p.Clone()
	noTail.Assets[0].Duration = 2000000
	if _, _, err := transitionHandle(noTail, ref); err == nil {
		t.Fatal("empty source handle accepted")
	}
	locked := p.Clone()
	locked.Tracks[0].Locked = true
	if _, err := applyTransitionDuration(locked, ref, 500000); !errors.Is(err, project.ErrTrackLocked) {
		t.Fatalf("locked edit: %v", err)
	}
	hidden := p.Clone()
	hidden.Tracks[0].Hidden = true
	if _, err := applyTransitionDuration(hidden, ref, 500000); err == nil {
		t.Fatal("hidden transition created")
	}
	partial, err := p.SetClipOpacity(ref, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := applyTransitionDuration(partial, ref, 500000); err == nil || !strings.Contains(err.Error(), "opaque full-canvas") || got.Fingerprint() != partial.Fingerprint() {
		t.Fatalf("unsupported opacity accepted: %v", err)
	}
}

func TestTransitionMenuEligibility(t *testing.T) {
	for _, tc := range []struct {
		state       timelineMenuState
		avail       editorstate.Availability
		add, remove bool
	}{
		{timelineMenuState{count: 1, transition: true}, editorstate.Availability{ClipCount: 1}, true, false},
		{timelineMenuState{count: 1, transition: true, removeTransition: true}, editorstate.Availability{ClipCount: 1}, true, true},
		{timelineMenuState{count: 1, transition: true, removeTransition: true, offline: true}, editorstate.Availability{ClipCount: 1}, false, true},
		{timelineMenuState{count: 1, transition: true, removeTransition: true}, editorstate.Availability{ClipCount: 1, TrackLocked: true}, false, false},
		{timelineMenuState{count: 1, transition: true, removeTransition: true}, editorstate.Availability{ClipCount: 1, VideoSuppressed: true}, false, true},
		{timelineMenuState{count: 2, transition: true}, editorstate.Availability{ClipCount: 2}, false, false},
		{timelineMenuState{count: 1, audioTrack: true, transition: true}, editorstate.Availability{ClipCount: 1, AudioTrack: true}, false, false},
	} {
		items := eligibleItems(timelineMenuItems(tc.state), tc.avail)
		add, _ := findMenuItem(items, menuTransitionDuration)
		remove, _ := findMenuItem(items, menuTransitionRemove)
		if add.enabled != tc.add || remove.enabled != tc.remove {
			t.Fatalf("incorrect menu eligibility: %+v %+v %+v", tc, add, remove)
		}
	}
}

func TestTransitionMarkerBounds(t *testing.T) {
	p, ref := transitionControlFixture(t)
	q, err := applyTransitionDuration(p, ref, 500000)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := q.Clip(ref)
	view := timeline.View{Width: 500, Zoom: 100, End: q.EndTime()}
	if span, ok := transitionMarker(view, c); !ok || span != (timeline.Span{Left: timeline.Header + 200, Right: timeline.Header + 250}) {
		t.Fatalf("marker: %+v %t", span, ok)
	}
	view.Origin = 2250000
	if span, ok := transitionMarker(view, c); !ok || span.Left != timeline.Header || span.Right != timeline.Header+25 {
		t.Fatalf("clipped marker: %+v %t", span, ok)
	}
	view.Origin = 3000000
	if _, ok := transitionMarker(view, c); ok {
		t.Fatal("offscreen transition visible")
	}
	c.TransitionIn = nil
	if _, ok := transitionMarker(view, c); ok {
		t.Fatal("removed transition visible")
	}
}
