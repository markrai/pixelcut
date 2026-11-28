package editor

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestTransitionUndoRedo(t *testing.T) {
	p := project.New()
	p.Version = project.EnvelopeVersion
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 6000000, Width: 1920, Height: 1080}}
	p.Tracks[0].Clips = []project.Clip{{ID: "a1", AssetID: "a", Out: 2000000}, {ID: "a2", AssetID: "a", Start: 2000000, In: 3000000, Out: 5000000}}
	s := New(p)
	ref := project.ClipRef{TrackID: "V1", ClipID: "a2"}
	q, err := s.Project().SetVideoTransition(ref, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "a1", Duration: 500000})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Commit(q, SelectionUpdate{})
	if err != nil || !changed || !s.Dirty() || !s.Project().HasVideoTransitions() {
		t.Fatalf("commit transition: %v", err)
	}
	if !s.Undo() || s.Project().HasVideoTransitions() || s.Dirty() || s.Project().Version != project.EnvelopeVersion {
		t.Fatal("undo transition did not restore original schema and edit")
	}
	if !s.Redo() || !s.Project().HasVideoTransitions() || !s.Dirty() || s.Project().Version != project.Version {
		t.Fatal("redo transition did not restore edit")
	}
}
