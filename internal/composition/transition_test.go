package composition

import (
	"context"
	"errors"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestTransitionEvaluatorExactWindow(t *testing.T) {
	p := fixture(t)
	ref := project.ClipRef{TrackID: "V1", ClipID: "later"}
	q, err := p.SetVideoTransition(ref, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "early", Duration: 8})
	if err != nil {
		t.Fatal(err)
	}
	e, err := CompileTransitions(q)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at, from, to, weight int64
	}{{30, 120, 200, 0}, {32, 122, 202, 250000}, {34, 124, 204, 500000}, {37, 127, 207, 875000}} {
		got := e.At(tc.at)
		if len(got) != 1 || got[0].FromTime != tc.from || got[0].ToTime != tc.to || got[0].IncomingWeight != tc.weight || got[0].Window.To != ref {
			t.Fatalf("transition sample at %d: %+v", tc.at, got)
		}
	}
	for _, at := range []int64{-1, 0, 29, 38, 40, 55} {
		if got := e.At(at); len(got) != 0 {
			t.Fatalf("unexpected active transition at %d: %+v", at, got)
		}
	}
	q.Tracks[0].Clips[0].TransitionIn.Duration = 1
	if got := e.At(34); len(got) != 1 || got[0].IncomingWeight != 500000 {
		t.Fatal("compiled transition aliases project")
	}
	q.Tracks[0].Hidden = true
	hidden, err := CompileTransitions(q)
	if err != nil || len(hidden.At(30)) != 0 {
		t.Fatalf("hidden video transition evaluated: %v", err)
	}
	if _, err := Compile(q); !errors.Is(err, ErrTransitionRenderingUnavailable) {
		t.Fatalf("existing renderer silently ignored transition: %v", err)
	}
	if _, err := Compile(p); err != nil {
		t.Fatalf("ordinary project no longer compiles: %v", err)
	}
}

func TestTransitionExportEvaluatorBoundaries(t *testing.T) {
	p := fixture(t)
	ref := project.ClipRef{TrackID: "V1", ClipID: "later"}
	q, err := p.SetVideoTransition(ref, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "early", Duration: 8})
	if err != nil {
		t.Fatal(err)
	}
	e, err := CompileForExport(q)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at, next int64
		active   bool
	}{{29, 30, false}, {30, 35, true}, {35, 38, true}, {38, 40, false}} {
		f, err := e.At(tc.at)
		if err != nil || f.NextBoundary != tc.next {
			t.Fatalf("boundary at %d: %+v %v", tc.at, f, err)
		}
		plan, err := e.PlanLayers(tc.at)
		if err != nil || (len(plan.Transitions) == 1) != tc.active {
			t.Fatalf("transition plan at %d: %+v %v", tc.at, plan, err)
		}
		if tc.active {
			tr := plan.Transitions[0]
			if tr.From.Layer.SourceTime != 120+tc.at-30 || tr.From.Layer.SourceInterval != (project.Interval{Start: 120, End: 128}) {
				t.Fatal("outgoing handle mapping incorrect")
			}
			if _, err := RenderStill(context.Background(), nil, plan, 640); !errors.Is(err, ErrTransitionRenderingUnavailable) {
				t.Fatalf("still renderer ignored transition: %v", err)
			}
		}
	}
}
