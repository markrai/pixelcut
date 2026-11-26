package main

import (
	"path/filepath"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func TestAudioPreviewTargetAndMenus(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 100, Width: 1080, Height: 1920, HasAudio: true}}
	p.Tracks[1].Clips = []project.Clip{{ID: "late", AssetID: "a", Start: 40, Out: 10}, {ID: "early", AssetID: "a", Start: 10, In: 20, Out: 30}}
	e := editor{model: editorstate.New(p)}
	e.model.SwitchTrack("A1")
	i, at, ok, err := e.previewTarget(0, true)
	if err != nil || !ok || i != 1 || at != 10 {
		t.Fatal("audio gap-start target")
	}
	if _, _, ok, err = e.previewTarget(50, true); err != nil || ok {
		t.Fatal("audio end wrapped")
	}
	menu := timelineMenuItems(timelineMenuState{count: 1, detachAudio: true})
	if v, ok := findMenuItem(menu, menuDetachAudio); !ok || !v.enabled {
		t.Fatal("detach menu missing")
	}
	menu = timelineMenuItems(timelineMenuState{count: 1, reattachAudio: true})
	if v, ok := findMenuItem(menu, menuReattachAudio); !ok || !v.enabled {
		t.Fatal("reattach menu missing")
	}
	menu = timelineMenuItems(timelineMenuState{count: 1, audioTrack: true, detachAudio: true})
	if _, ok := findMenuItem(menu, menuDetachAudio); ok {
		t.Fatal("detach menu offered on audio track")
	}
}
