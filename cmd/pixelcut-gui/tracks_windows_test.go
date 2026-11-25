package main

import (
	"path/filepath"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func TestTrackMenuEligibility(t *testing.T) {
	p := project.New()
	p.Assets = append(p.Assets, project.Asset{ID: "a", Path: filepath.Join(t.TempDir(), "media.mp4"), Duration: 1000000, Width: 1080, Height: 1920, HasAudio: true})
	r, err := p.PlaceAssets("V1", []string{"a"}, []string{"c"}, 0, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, id, err := r.Project.AddTrack("video")
	if err != nil {
		t.Fatal(err)
	}
	items := trackMenuItems(p, "V1", []string{"c"})
	for _, tc := range []struct {
		id               int
		enabled, checked bool
	}{
		{trackMenuSelect, true, true},
		{trackMenuSelect + 1, true, false},
		{trackMenuRemove, false, false},
		{trackMenuMove + p.TrackIndex(id), true, false},
		{trackMenuVideo, true, false},
		{trackMenuAudio, true, false},
	} {
		v, ok := findMenuItem(items, tc.id)
		if !ok || v.enabled != tc.enabled || v.checked != tc.checked {
			t.Fatalf("menu action %d: %+v", tc.id, v)
		}
	}
	if _, ok := findMenuItem(items, trackMenuMove+1); ok {
		t.Fatal("cross-kind move offered")
	}
	ed := editor{model: editorstate.New(p)}
	ed.model.SwitchTrack(id)
	if ed.trackID() != id || ed.audioTrack() {
		t.Fatal("active track not resolved")
	}
	ed.model.SwitchTrack("A1")
	if !ed.audioTrack() {
		t.Fatal("audio track not resolved")
	}
	ed.model.SwitchTrack("missing")
	if ed.trackID() != "A1" {
		t.Fatal("invalid active track switch")
	}
}

func TestAudioContextEligibility(t *testing.T) {
	items := timelineMenuItems(timelineMenuState{count: 1, split: true, audioTrack: true})
	for _, id := range []int{idCropVisual, idTransform, idExport, idSetIn, idSetOut} {
		v, ok := findMenuItem(items, id)
		if !ok || v.enabled {
			t.Fatalf("video action %d enabled on audio track", id)
		}
	}
	for _, id := range []int{idSplit, idCopy, idDelete, menuRevealClip} {
		v, ok := findMenuItem(items, id)
		if !ok || !v.enabled {
			t.Fatalf("audio editing action %d disabled", id)
		}
	}
}
