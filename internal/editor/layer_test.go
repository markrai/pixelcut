package editor

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestLayerSettingsHistory(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100, Width: 1080, Height: 1920}}
	p.Tracks[0].Clips = []project.Clip{{ID: "v", AssetID: "a", Start: 10, Out: 20}}
	s := New(p)
	s.SelectIndex(0)
	ref := project.ClipRef{TrackID: "V1", ClipID: "v"}
	q, err := s.Project().ApplyLayerPreset(ref, media.LayerBottomRight)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(q, SelectionUpdate{}); err != nil || !changed {
		t.Fatal("PiP commit failed")
	}
	q, err = s.Project().SetClipOpacity(ref, 50)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(q, SelectionUpdate{}); err != nil || !changed {
		t.Fatal("opacity commit failed")
	}
	if !s.Undo() || s.Project().Tracks[0].Clips[0].OpacityPercent() != 100 || !s.Undo() || s.Project().Tracks[0].Clips[0].Transform != nil || s.Dirty() {
		t.Fatal("layer history did not restore original project")
	}
	if !s.Redo() || !s.Redo() || s.Project().Tracks[0].Clips[0].OpacityPercent() != 50 || s.PrimaryID() != "v" {
		t.Fatal("layer history did not restore edits or selection")
	}
}
