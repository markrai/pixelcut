package editor

import (
	"errors"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestTransactionAtomicity(t *testing.T) {
	p := fixture(t)
	s := New(p)
	s.SetAssetID("a")
	s.Select([]string{"early", "middle"}, "middle")
	s.SetAnchor("early")
	before := s.snapshot()
	q, err := p.DeleteClips("V1", []string{"middle"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := q.Clone()
	invalid.Tracks[0].Clips[0].Start = -1
	if changed, err := s.Commit(invalid, SelectionUpdate{}); changed || !errors.Is(err, project.ErrInvalidClip) {
		t.Fatalf("invalid commit: %v %v", changed, err)
	}
	if changed, err := s.Commit(q, SelectionUpdate{ReplaceClips: true, ClipIDs: []string{"missing"}}); changed || err == nil {
		t.Fatal("invalid selection accepted")
	}
	if changed, err := s.Commit(q, SelectionUpdate{ReplaceAsset: true, AssetID: "missing"}); changed || err == nil {
		t.Fatal("invalid asset accepted")
	}
	if !reflect.DeepEqual(before, s.snapshot()) || s.Dirty() || s.CanUndo() || s.CanRedo() {
		t.Fatal("failed transaction changed state")
	}
	changed, err := s.Commit(q, SelectionUpdate{ReplaceClips: true})
	if err != nil || !changed || s.Count() != 0 || !s.Dirty() || !s.CanUndo() {
		t.Fatalf("commit: %v %v", changed, err)
	}
	q.Tracks[0].Clips[0].Start++
	if s.Project().Tracks[0].Clips[0].Start == q.Tracks[0].Clips[0].Start {
		t.Fatal("committed candidate aliases caller")
	}
	if !s.Undo() || !reflect.DeepEqual(before, s.snapshot()) || s.Dirty() {
		t.Fatal("undo did not restore original project and selection")
	}
	if !s.Redo() || s.Count() != 0 || !s.Dirty() {
		t.Fatal("redo did not restore committed selection")
	}
}

func TestNoopPreservesRedoAndSaveBaseline(t *testing.T) {
	p := fixture(t)
	s := New(p)
	q, err := p.DeleteClips("V1", []string{"middle"})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(q, SelectionUpdate{}); !changed || err != nil {
		t.Fatal(err)
	}
	s.MarkSaved()
	if s.Dirty() || !s.Undo() || !s.Dirty() {
		t.Fatal("save baseline did not survive undo")
	}
	if changed, err := s.Commit(p.Clone(), SelectionUpdate{ReplaceClips: true, ClipIDs: []string{"early"}, Primary: "early"}); changed || err != nil {
		t.Fatal("selection-only transaction became history")
	}
	if !s.CanRedo() || s.CanUndo() || s.PrimaryID() != "early" {
		t.Fatal("no-op changed history")
	}
	if !s.Redo() || s.Dirty() {
		t.Fatal("redo to saved state should be clean")
	}
	s.SetSavedBaseline("")
	if !s.Dirty() {
		t.Fatal("unknown baseline treated as saved")
	}
	s.MarkSaved()
	if s.Dirty() {
		t.Fatal("save did not establish baseline")
	}
}

func TestHistoryLimitAndBranch(t *testing.T) {
	s := New(fixture(t))
	for i := 0; i < HistoryLimit+2; i++ {
		q := s.Project().Clone()
		q.Tracks[0].Clips[0].Start++
		if changed, err := s.Commit(q, SelectionUpdate{}); err != nil || !changed {
			t.Fatalf("edit %d: %v", i, err)
		}
	}
	for i := 0; i < HistoryLimit; i++ {
		if !s.Undo() {
			t.Fatal("history prematurely exhausted")
		}
	}
	if s.CanUndo() || !s.CanRedo() {
		t.Fatal("history limit not enforced")
	}
	q := s.Project().Clone()
	q.Tracks[0].Clips[0].Start++
	if changed, err := s.Commit(q, SelectionUpdate{}); err != nil || !changed || s.CanRedo() {
		t.Fatal("new branch did not invalidate redo")
	}
}

func TestTransferTransactionSelection(t *testing.T) {
	p := fixture(t)
	s := New(p)
	s.Select([]string{"early", "middle"}, "middle")
	s.SetAnchor("early")
	q, err := p.TransferClips("V1", "V2", s.IDs())
	if err != nil {
		t.Fatal(err)
	}
	u := SelectionUpdate{TrackID: "V2", ReplaceClips: true, ClipIDs: []string{"early", "middle"}, Primary: "middle", Anchor: "early"}
	if changed, err := s.Commit(q, u); err != nil || !changed {
		t.Fatal(err)
	}
	s.SwitchTrack("A1")
	if !s.Undo() || s.TrackID() != "V1" || s.PrimaryID() != "middle" || s.AnchorID() != "early" {
		t.Fatal("undo lost pre-edit selection")
	}
	if !s.Redo() || s.TrackID() != "V2" || s.Count() != 2 || s.PrimaryID() != "middle" {
		t.Fatal("redo lost post-edit selection")
	}
}

func TestCommandAvailability(t *testing.T) {
	s := New(fixture(t))
	for _, requirement := range []Requirement{SingleClip, VideoClip, SelectedClips, SelectedAsset, Clipboard, UndoAvailable, RedoAvailable} {
		if s.Availability(false).Check(requirement) == nil {
			t.Fatalf("empty state allowed requirement %d", requirement)
		}
	}
	s.SetAssetID("a")
	s.SelectIndex(0)
	for _, requirement := range []Requirement{SingleClip, VideoClip, SelectedClips, SelectedAsset, Clipboard, Unrestricted} {
		if err := s.Availability(true).Check(requirement); err != nil {
			t.Fatal(err)
		}
	}
	s.SelectAll()
	if s.Availability(false).Check(SingleClip) == nil || s.Availability(false).Check(SelectedClips) != nil {
		t.Fatal("group eligibility")
	}
	if err := (Availability{ClipCount: 1, AudioTrack: true}).Check(VideoClip); err == nil {
		t.Fatal("video-only operation allowed on audio")
	}
}
