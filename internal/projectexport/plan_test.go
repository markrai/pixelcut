package projectexport

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func exportFixture(t *testing.T) project.Project {
	t.Helper()
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 1000000, Width: 64, Height: 48, HasAudio: true, Range: "tv"}}
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", Start: 10001, In: 20000, Out: 120000}}
	p.Tracks[1].Clips = []project.Clip{{ID: "audio", AssetID: "a", Start: 150000, Out: 20001}}
	var err error
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "top", AssetID: "a", Start: 50000, In: 100000, Out: 120000, Transform: &media.Transform{CanvasWidth: 64, CanvasHeight: 48, Scale: 50}}}
	p, err = p.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "top"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildExportPlanSpansAndQuantization(t *testing.T) {
	p := exportFixture(t)
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Duration != 170001 || plan.VideoFrames != 6 || plan.ContentAudioFrames != 8161 || plan.OutputAudioFrames != 9600 || !plan.Contract.Audio {
		t.Fatalf("wrong output duration: %+v", plan)
	}
	if len(plan.Spans) != 6 || len(plan.Spans[0].Layers) != 0 || len(plan.Spans[0].Audio) != 0 || plan.Spans[0].End != 10001 {
		t.Fatalf("wrong leading gap or edit spans: %+v", plan.Spans)
	}
	s := plan.Spans[2]
	if s.Start != 50000 || s.End != 70000 || len(s.Layers) != 2 || s.Layers[0].Layer.Ref.ClipID != "base" || s.Layers[1].Layer.Opacity != 50 || s.Layers[1].Layer.SourceTime != 100000 || len(s.Audio) != 2 {
		t.Fatalf("wrong layered interval: %+v", s)
	}
	for i, span := range plan.Spans {
		if i > 0 {
			prev := plan.Spans[i-1]
			if prev.End != span.Start || prev.EndFrame != span.FirstFrame || prev.EndSample != span.FirstSample {
				t.Fatal("noncontiguous output grid")
			}
		}
		for n := span.FirstFrame; n < span.EndFrame; n++ {
			tick, ok := plan.FrameTick(n)
			if !ok || tick < span.Start || tick >= span.End {
				t.Fatalf("frame %d maps outside span: %d", n, tick)
			}
		}
	}
	if _, ok := plan.FrameTick(plan.VideoFrames); ok {
		t.Fatal("accepted exclusive final frame")
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("missing output quantization warning")
	}
	if p.Fingerprint() != plan.ProjectFingerprint {
		t.Fatal("planner changed project")
	}
}

func TestExportPlanPolicyAndUnsupportedContracts(t *testing.T) {
	p := exportFixture(t)
	p.Tracks[0].Hidden = true
	p.Tracks[2].Muted = true
	p.Tracks[1].Muted = true
	p.Tracks[0].Clips[0].AudioDetached = true
	plan, err := Build(p, "hlg", 24)
	if err != nil || plan.Contract.Audio || plan.OutputAudioFrames != 0 || plan.VideoFrames != 5 {
		t.Fatalf("wrong policy: %+v %v", plan, err)
	}
	for _, span := range plan.Spans {
		if len(span.Audio) != 0 || len(span.Layers) > 1 {
			t.Fatal("hidden or muted contribution escaped policy")
		}
	}
	for _, tc := range []struct {
		color string
		fps   int
	}{{"", 30}, {"sdr", 30}, {"hlg", 0}, {"hlg", 29}} {
		if _, err := Build(p, tc.color, tc.fps); err == nil {
			t.Fatal("unsupported output contract accepted")
		}
	}
	p.Tracks[2].Hidden = true
	if _, err := Build(p, "hlg", 30); err == nil {
		t.Fatal("no enabled video accepted")
	}
	p = project.New()
	if _, err := Build(p, "hlg", 30); err == nil {
		t.Fatal("empty project accepted")
	}
}
