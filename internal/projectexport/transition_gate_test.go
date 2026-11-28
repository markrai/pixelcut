package projectexport

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/project"
)

func TestTransitionExportFailsClosed(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 6000000, Width: 1920, Height: 1080, Range: "tv"}}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: "a", Out: 2000000}, {ID: "next", AssetID: "a", Start: 2000000, In: 3000000, Out: 5000000}}
	if _, err := Build(p, "hlg", 30); err != nil {
		t.Fatal(err)
	}
	q, err := p.SetVideoTransition(project.ClipRef{TrackID: "V1", ClipID: "next"}, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first", Duration: 500000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(q, "hlg", 30); !errors.Is(err, composition.ErrTransitionRenderingUnavailable) {
		t.Fatalf("export planner silently ignored transition: %v", err)
	}
}
