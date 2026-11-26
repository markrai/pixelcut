package editor

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestAudioDetachHistory(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100, Width: 1080, Height: 1920, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "v", AssetID: "a", Start: 10, In: 20, Out: 40}}
	s := New(p)
	s.SelectIndex(0)
	ref := project.ClipRef{TrackID: "V1", ClipID: "v"}
	q, err := s.Project().DetachAudio(ref, "A1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Commit(q, SelectionUpdate{})
	if err != nil || !changed || s.PrimaryID() != "v" || !s.Dirty() {
		t.Fatal("detach commit failed")
	}
	if !s.Undo() || s.CanUndo() || s.Dirty() || len(s.Project().Tracks[1].Clips) != 0 {
		t.Fatal("detach did not undo atomically")
	}
	if !s.Redo() || !s.Project().Tracks[0].Clips[0].AudioDetached || len(s.Project().Tracks[1].Clips) != 1 {
		t.Fatal("detach did not redo atomically")
	}
	q, err = s.Project().ReattachAudio(ref, project.ClipRef{TrackID: "A1", ClipID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err = s.Commit(q, SelectionUpdate{}); err != nil || !changed || s.Dirty() {
		t.Fatal("reattach did not restore saved content")
	}
	if !s.Undo() || !s.Project().Tracks[0].Clips[0].AudioDetached {
		t.Fatal("reattach did not undo")
	}
}
