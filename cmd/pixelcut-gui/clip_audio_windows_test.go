package main

import (
	"path/filepath"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func TestClipAudioMenusAndTransactions(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100, Width: 8, Height: 8, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "v", AssetID: "a", Out: 100}}
	ref := project.ClipRef{TrackID: "V1", ClipID: "v"}
	s := editorstate.New(p)
	s.SelectIndex(0)
	if !clipAudioAvailable(p, ref) {
		t.Fatal("audio control unavailable")
	}
	q, err := clipAudioChoice(p, ref, clipGainBase+4)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(q, editorstate.SelectionUpdate{}); err != nil || !changed {
		t.Fatal("gain transaction failed")
	}
	q, err = clipAudioChoice(s.Project(), ref, menuClipMute)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Commit(q, editorstate.SelectionUpdate{}); err != nil || !changed {
		t.Fatal("mute transaction failed")
	}
	c, _ := s.Project().Clip(ref)
	if c.GainPercent() != 75 || !c.AudioMuted || clipAudioButtonLabel(c) != "Audio: muted" {
		t.Fatal("incorrect mute display/state")
	}
	items := clipAudioItems(c)
	ids := map[int]bool{}
	for _, v := range items {
		if v.id != 0 && ids[v.id] {
			t.Fatal("duplicate menu ID")
		}
		ids[v.id] = true
	}
	if v, ok := findMenuItem(items, menuClipMute); !ok || !v.checked || v.label != "Unmute clip audio" {
		t.Fatal("mute menu state missing")
	}
	if v, ok := findMenuItem(items, clipGainBase+4); !ok || !v.checked {
		t.Fatal("stored gain not indicated while muted")
	}
	q, err = clipAudioChoice(q, ref, clipGainUp)
	c, _ = q.Clip(ref)
	if err != nil || c.GainPercent() != 76 || !c.AudioMuted {
		t.Fatal("gain adjustment unmuted clip")
	}
	if !s.Undo() || s.Project().Tracks[0].Clips[0].AudioMuted || s.Project().Tracks[0].Clips[0].GainPercent() != 75 || !s.Redo() || !s.Project().Tracks[0].Clips[0].AudioMuted {
		t.Fatal("mute undo/redo lost gain")
	}
	p.Tracks[0].Locked = true
	if clipAudioAvailable(p, ref) {
		t.Fatal("locked clip controls enabled")
	}
	for _, id := range []int{idClipAudio, menuClipMute} {
		items := eligibleItems(timelineMenuItems(timelineMenuState{count: 1, clipAudio: true, audioTrack: true}), editorstate.Availability{ClipCount: 1, AudioTrack: true, TrackLocked: true})
		if v, ok := findMenuItem(items, id); !ok || v.enabled {
			t.Fatal("locked audio-track menu enabled")
		}
	}
}
