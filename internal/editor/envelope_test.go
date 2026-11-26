package editor

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestEnvelopeHistoryIsolation(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 1000000, Width: 64, Height: 48, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "v", AssetID: "a", Out: 1000000}}
	s := New(p)
	s.SelectIndex(0)
	ref := project.ClipRef{TrackID: "V1", ClipID: "v"}
	points := []project.VolumeKeyframe{{SourceTime: 0, Percent: 100}, {SourceTime: 500000, Percent: 25}, {SourceTime: 1000000, Percent: 100}}
	q, err := s.Project().SetClipEnvelope(ref, points)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Commit(q, SelectionUpdate{})
	if err != nil || !changed || !s.Dirty() {
		t.Fatal("envelope commit failed")
	}
	points[1].Percent = 50
	q, err = s.Project().SetClipEnvelope(ref, points)
	if err != nil {
		t.Fatal(err)
	}
	changed, err = s.Commit(q, SelectionUpdate{})
	if err != nil || !changed {
		t.Fatal("second envelope commit failed")
	}
	if !s.Undo() || s.Project().Tracks[0].Clips[0].Envelope.Points[1].Percent != 25 {
		t.Fatal("undo lost original envelope")
	}
	if !s.Undo() || s.Dirty() || s.Project().Tracks[0].Clips[0].HasEnvelope() {
		t.Fatal("undo did not restore saved project")
	}
	if !s.Redo() || !s.Redo() || s.Project().Tracks[0].Clips[0].Envelope.Points[1].Percent != 50 {
		t.Fatal("redo lost edited envelope")
	}
}
