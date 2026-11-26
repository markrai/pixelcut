package editor

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestLockedEditCommitGuardAndHistory(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100, Width: 1080, Height: 1920}}
	p.Tracks[0].Clips = []project.Clip{{ID: "v", AssetID: "a", Start: 10, Out: 20}}
	s := New(p)
	s.SelectIndex(0)
	locked, err := s.Project().ToggleTrackControl("V1", project.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(locked, SelectionUpdate{}); err != nil || !changed {
		t.Fatal("lock commit failed")
	}
	before := s.Fingerprint()
	for _, mutate := range []func(*project.Project){
		func(q *project.Project) { q.Tracks[0].Clips[0].Start++ },
		func(q *project.Project) { q.Tracks[0].Clips[0].Out++ },
		func(q *project.Project) {
			q.Tracks[0].Locked = false
			q.Tracks[0].Clips[0].Start++
		},
		func(q *project.Project) { q.Assets[0].Path += ".other" },
	} {
		q := s.Project().Clone()
		mutate(&q)
		if changed, err := s.Commit(q, SelectionUpdate{}); changed || !errors.Is(err, project.ErrTrackLocked) {
			t.Fatalf("locked commit accepted: %t %v", changed, err)
		}
		if s.Fingerprint() != before || s.PrimaryID() != "v" || s.CanRedo() {
			t.Fatal("failed commit changed state")
		}
	}
	q, err := s.Project().ToggleTrackControl("V1", project.Mute)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(q, SelectionUpdate{}); err != nil || !changed {
		t.Fatal("track flag change should be allowed on locked track")
	}
	if !s.Undo() || s.Project().Tracks[0].Muted || !s.Project().Tracks[0].Locked || !s.Undo() || s.Project().Tracks[0].Locked || s.Dirty() {
		t.Fatal("track flags did not undo atomically")
	}
	if !s.Redo() || !s.Project().Tracks[0].Locked {
		t.Fatal("lock redo failed")
	}
}
