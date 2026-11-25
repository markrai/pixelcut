package editor

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestCrossTrackMoveHistory(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 10000000, Width: 1080, Height: 1920, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{
		{ID: "first", AssetID: "a", Start: 1000000, In: 1000000, Out: 2000000},
		{ID: "second", AssetID: "a", Start: 4000000, In: 2000000, Out: 3000000},
	}
	p, target, err := p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	state := New(p)
	ids := []string{"first", "second"}
	state.Select(ids, "second")
	state.SetAnchor("first")
	before := state.Fingerprint()
	r, err := state.Project().MoveClipsTo("V1", target, ids, 1000000, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	update := SelectionUpdate{TrackID: target, ReplaceClips: true, ClipIDs: ids, Primary: "second", Anchor: "first"}
	changed, err := state.Commit(r.Project, update)
	if err != nil || !changed || state.TrackID() != target || !reflect.DeepEqual(state.IDs(), ids) || state.PrimaryID() != "second" || !state.Dirty() {
		t.Fatalf("move commit: %v %t", err, changed)
	}
	after := state.Fingerprint()
	if !state.Undo() || state.CanUndo() || state.Fingerprint() != before || state.TrackID() != "V1" || state.Dirty() || state.AnchorID() != "first" {
		t.Fatal("move did not undo atomically")
	}
	if !state.Redo() || state.Fingerprint() != after || state.TrackID() != target || state.PrimaryID() != "second" || !reflect.DeepEqual(state.IDs(), ids) {
		t.Fatal("move did not redo atomically")
	}
}
