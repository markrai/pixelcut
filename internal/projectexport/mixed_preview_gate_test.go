package projectexport

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestMixedTimelineEnablesHLGExport(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{
		{ID: "hlg", Path: filepath.Join(t.TempDir(), "hlg.mp4"), Duration: 1000000, Width: 192, Height: 144, Range: "tv"},
		{ID: "sdr", Path: filepath.Join(t.TempDir(), "sdr.mp4"), Duration: 1000000, Width: 192, Height: 144, Range: "tv", ColorFamily: media.ColorSDR709, Codec: "h264", PixelFormat: "yuv420p", Chroma: "left"},
	}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: "hlg", Out: 500000}}
	if _, err := Build(p, "hlg", 30); err != nil {
		t.Fatalf("unused SDR bin asset blocked HLG export: %v", err)
	}
	p.Tracks[0].Clips = append(p.Tracks[0].Clips, project.Clip{ID: "next", AssetID: "sdr", Start: 500000, Out: 500000})
	q, err := p.SetVideoTransition(project.ClipRef{TrackID: "V1", ClipID: "next"}, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first", Duration: 250000})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransitionLayout(q); err != nil {
		t.Fatalf("mixed transition layout: %v", err)
	}
	plan, err := Build(q, "hlg", 30)
	if err != nil || len(plan.Warnings) == 0 {
		t.Fatalf("mixed export planning: %v", err)
	}
	if sdr, err := Build(q, "sdr", 30); err != nil || len(sdr.Warnings) == 0 || sdr.Warnings[0] != SDRToneMappingDescription {
		t.Fatalf("native SDR export plan: %v", err)
	}
}
