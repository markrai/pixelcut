package projectexport

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestMixedPreviewDoesNotEnableExport(t *testing.T) {
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
	if _, err := Build(q, "hlg", 30); !errors.Is(err, project.ErrMixedExportUnavailable) {
		t.Fatalf("mixed export not gated: %v", err)
	}
}
