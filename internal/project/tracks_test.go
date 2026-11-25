package project

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestMultitrackFoundation(t *testing.T) {
	p := fixture(t)
	p.Version = LegacyVersion
	before := p.Clone()
	q, id, err := p.AddTrack("video")
	if err != nil || id != "V2" || q.Version != Version {
		t.Fatalf("add video: %s %v", id, err)
	}
	q, id, err = q.AddTrack("audio")
	if err != nil || id != "A2" {
		t.Fatalf("add audio: %s %v", id, err)
	}
	r, err := q.PlaceAssets("V2", []string{"a"}, []string{"v"}, 1000000, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	q = r.Project
	r, err = q.PlaceAssets("A1", []string{"a"}, []string{"sound"}, 1000000, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	q = r.Project
	if q.EndTime() != 11000000 || q.Tracks[1].Clips[0].Crop != nil {
		t.Fatal("incorrect independent audio")
	}
	if _, err = q.TransferClips("V1", "V2", []string{"c"}); err == nil {
		t.Fatal("overlapping transfer accepted")
	}
	if _, err = q.TransferClips("V1", "A2", []string{"c"}); err == nil {
		t.Fatal("cross-kind transfer accepted")
	}
	rp, err := q.TransferClips("A1", "A2", []string{"sound"})
	if err != nil || len(rp.Tracks[1].Clips) != 0 || rp.Tracks[rp.TrackIndex("A2")].Clips[0].Start != 1000000 {
		t.Fatal("audio transfer failed")
	}
	if _, err = rp.RemoveTrack("A2"); err == nil {
		t.Fatal("nonempty track removed")
	}
	if _, err = rp.RemoveTrack("A1"); err == nil {
		t.Fatal("base track removed")
	}
	b, err := rp.CopyClips("A2", []string{"sound"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rp.PasteClips("V1", b, 20000000, []string{"bad"}); err == nil {
		t.Fatal("cross-kind paste accepted")
	}
	paste, err := rp.PasteClips("A1", b, 0, []string{"sound2"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "multi.pixelcut.json")
	if err = paste.Project.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, paste.Project) {
		t.Fatal("multitrack roundtrip failed")
	}
	if !reflect.DeepEqual(p, before) {
		t.Fatal("legacy project mutated")
	}
}

func TestAudioPlacementRequiresAudio(t *testing.T) {
	p := fixture(t)
	p.Assets[0].HasAudio = false
	if _, err := p.PlaceAssets("A1", []string{"a"}, []string{"sound"}, 0, -1, 0); err == nil {
		t.Fatal("silent source accepted")
	}
}
