package editor

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func fixture(t *testing.T) project.Project {
	t.Helper()
	p := project.New()
	p.Assets = []project.Asset{
		{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 100, Width: 1080, Height: 1920, HasAudio: true},
		{ID: "b", Path: filepath.Join(t.TempDir(), "b.mp4"), Duration: 100, Width: 1080, Height: 1920},
	}
	p.Tracks[0].Clips = []project.Clip{
		{ID: "late", AssetID: "a", Start: 40, Out: 10},
		{ID: "early", AssetID: "a", Start: 0, Out: 10},
		{ID: "middle", AssetID: "b", Start: 20, Out: 10},
	}
	q, _, err := p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestSelectionByIdentity(t *testing.T) {
	p := fixture(t)
	s := New(p)
	if s.ClipIndex() != -1 || s.AssetIndex() != -1 || s.TrackID() != "V1" {
		t.Fatal("initial selection")
	}
	s.SetAssetIndex(1)
	s.SelectIndex(1)
	s.Modify("late", false, true)
	if !reflect.DeepEqual(s.IDs(), []string{"late", "early", "middle"}) || s.PrimaryID() != "late" || s.AnchorID() != "early" {
		t.Fatalf("range: %v", s.IDs())
	}
	s.Modify("late", true, false)
	if s.Count() != 2 || s.Contains("late") || s.PrimaryID() != "early" {
		t.Fatal("toggle off primary")
	}
	q := p.Clone()
	q.Assets[0], q.Assets[1] = q.Assets[1], q.Assets[0]
	q.Tracks[0].Clips[0], q.Tracks[0].Clips[1] = q.Tracks[0].Clips[1], q.Tracks[0].Clips[0]
	s.adopt(q)
	if s.AssetID() != "b" || s.AssetIndex() != 0 || s.PrimaryID() != "early" || s.ClipIndex() != 0 || s.Count() != 2 {
		t.Fatal("selection shifted with storage order")
	}
	ids := s.IDs()
	ids[0] = "wrong"
	if !s.Contains("early") {
		t.Fatal("returned selection aliases state")
	}
	s.SelectAll()
	if s.Count() != 3 || s.AnchorID() != "" {
		t.Fatal("select all")
	}
}

func TestTransferAndHistoryReconciliation(t *testing.T) {
	p := fixture(t)
	s := New(p)
	s.Select([]string{"early", "middle"}, "middle")
	s.SetAnchor("early")
	q, err := p.TransferClips("V1", "V2", s.IDs())
	if err != nil {
		t.Fatal(err)
	}
	s.adopt(q)
	if s.TrackID() != "V2" || s.PrimaryID() != "middle" || s.Count() != 2 || s.AnchorID() != "early" {
		t.Fatal("selection did not follow transfer")
	}
	s.adopt(p)
	if s.TrackID() != "V1" || s.PrimaryID() != "middle" || s.Count() != 2 {
		t.Fatal("selection did not follow undo")
	}
	s.adopt(q)
	if s.TrackID() != "V2" || s.PrimaryID() != "middle" {
		t.Fatal("selection did not follow redo")
	}
	deleted, err := q.DeleteClips("V2", []string{"middle"})
	if err != nil {
		t.Fatal(err)
	}
	s.adopt(deleted)
	if s.PrimaryID() != "early" || s.Count() != 1 {
		t.Fatal("surviving selection lost")
	}
	if !s.SwitchTrack("A1") || s.Count() != 0 || s.AnchorID() != "" {
		t.Fatal("track switch did not clear clip selection")
	}
	if s.SwitchTrack("missing") || s.TrackID() != "A1" {
		t.Fatal("invalid track switch")
	}
	s.SwitchTrack("V2")
	s.adopt(project.New())
	if s.TrackID() != "V1" || s.Count() != 0 || s.ClipIndex() != -1 {
		t.Fatal("removed track fallback")
	}
}

func TestRestoreLegacySelection(t *testing.T) {
	p := fixture(t)
	s := New(p)
	s.Restore("", 1, 1, []string{"early", "middle"})
	if s.TrackID() != "V1" || s.AssetID() != "b" || s.PrimaryID() != "early" || s.Count() != 2 {
		t.Fatal("legacy index conversion")
	}
	s.Restore("V2", -1, -1, nil)
	if s.TrackID() != "V2" || s.ClipIndex() != -1 || s.AssetIndex() != -1 {
		t.Fatal("empty track restore")
	}
	s.Restore("", 0, 0, nil)
	if s.TrackID() != "V1" || s.PrimaryID() != "late" {
		t.Fatal("legacy restore did not reset active track")
	}
	q, err := p.RemoveAsset("a", true)
	if err != nil {
		t.Fatal(err)
	}
	s.adopt(q)
	if s.AssetIndex() != -1 || s.ClipIndex() != -1 || s.Count() != 0 {
		t.Fatal("removed references retained")
	}
}
