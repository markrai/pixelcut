package projectexport

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func dissolvePlanFixture(t *testing.T) project.Project {
	t.Helper()
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 6000000, Width: 1920, Height: 1080, Range: "tv", HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: "a", Out: 2000000}, {ID: "next", AssetID: "a", Start: 2000000, In: 3000000, Out: 5000000}}
	q, err := p.SetVideoTransition(project.ClipRef{TrackID: "V1", ClipID: "next"}, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first", Duration: 500000})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestTransitionExportPlan(t *testing.T) {
	p := dissolvePlanFixture(t)
	p.Tracks[1].Clips = []project.Clip{{ID: "sound", AssetID: "a", Start: 2200000, In: 1000000, Out: 1100000}}
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Duration != 4000000 || plan.VideoFrames != 120 || plan.OutputAudioFrames != 192000 || !plan.Contract.Audio {
		t.Fatalf("transition changed output timing: %+v", plan)
	}
	count := 0
	for _, span := range plan.Spans {
		active := span.Start >= 2000000 && span.Start < 2500000
		if (span.Transition != nil) != active {
			t.Fatalf("wrong transition boundary: %+v", span)
		}
		if !active {
			continue
		}
		count++
		tr := span.Transition
		if tr.Window.Timeline.Start != 2000000 || tr.Window.Timeline.End != 2500000 || tr.From.Layer.SourceTime != span.Start || len(span.Layers) != 1 || span.Layers[0].Layer.SourceTime != span.Start+1000000 {
			t.Fatalf("wrong source mapping: %+v", span)
		}
		for _, a := range span.Audio {
			if a.Ref.ClipID == "first" {
				t.Fatal("outgoing source handle extended audio")
			}
		}
		if span.End > 2500000 {
			t.Fatal("transition spans past end")
		}
	}
	if count != 3 {
		t.Fatalf("expected transition split at audio boundaries, got %d", count)
	}
}

func TestTransitionExportRejectsUnsupportedCoverage(t *testing.T) {
	p := dissolvePlanFixture(t)
	for _, id := range []string{"first", "next"} {
		for _, opacity := range []int{0, 50} {
			q, err := p.SetClipOpacity(project.ClipRef{TrackID: "V1", ClipID: id}, opacity)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Build(q, "hlg", 30); err == nil || !strings.Contains(err.Error(), "opaque full-canvas") {
				t.Fatalf("unsupported %s opacity %d: %v", id, opacity, err)
			}
		}
	}
	q := p.Clone()
	q.Tracks[0].Clips[0].Transform = &media.Transform{CanvasWidth: 1920, CanvasHeight: 1080, Scale: 50}
	if _, err := Build(q, "hlg", 30); err == nil || !strings.Contains(err.Error(), "opaque full-canvas") {
		t.Fatalf("partial geometry accepted: %v", err)
	}
	q = p.Clone()
	q.Tracks = append(q.Tracks, project.Track{ID: "V2", Kind: project.VideoTrack, Clips: []project.Clip{
		{ID: "first2", AssetID: "a", Out: 2000000},
		{ID: "next2", AssetID: "a", Start: 2000000, In: 3000000, Out: 5000000, TransitionIn: &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first2", Duration: 500000}},
	}})
	if _, err := Build(q, "hlg", 30); err == nil || !strings.Contains(err.Error(), "one active") {
		t.Fatalf("simultaneous dissolves accepted: %v", err)
	}
	q.Tracks[2].Hidden = true
	if _, err := Build(q, "hlg", 30); err != nil {
		t.Fatalf("hidden transition affected rendering: %v", err)
	}
	q = p.Clone()
	for _, id := range []string{"V2", "V3", "V4"} {
		q.Tracks = append(q.Tracks, project.Track{ID: id, Kind: project.VideoTrack, Clips: []project.Clip{{ID: id + "clip", AssetID: "a", Out: 4000000}}})
	}
	if _, err := Build(q, "hlg", 30); err == nil || !strings.Contains(err.Error(), "four-decoder") {
		t.Fatalf("transition decoder budget not enforced: %v", err)
	}
}
