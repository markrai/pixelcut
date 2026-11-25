package main

import (
	"path/filepath"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/playback"
	"github.com/markrai/pixelcut/internal/project"
)

func TestPlaybackAdapterRequests(t *testing.T) {
	c := newPreviewClock(project.Clip{Start: 10000000, In: 2000000, Out: 7000000}, 12000000, true)
	if !c.paused() {
		t.Fatal("initial pause lost")
	}
	c.queue(13000000, true, true)
	c.queue(14000000, true, true)
	c.queue(14000000, false, false)
	r := c.transport.Request()
	if r.Tick != 14000000 || r.SeekSerial != 2 || r.Serial != 3 || r.Paused {
		t.Fatalf("latest seek lost: %+v", r)
	}
	c.transport.Report(playback.Sample{Serial: 1, Tick: 13000000, Paused: true})
	if c.paused() {
		t.Fatal("stale sample overrode request")
	}
	c.transport.Report(playback.Sample{Serial: 3, Tick: 14000000, Paused: true})
	if !c.paused() {
		t.Fatal("player-side pause ignored")
	}
	c.fail()
	if !c.unavailable.Load() || c.transport.Usable() || !c.paused() {
		t.Fatal("backend failure not propagated")
	}
}

func TestPreviewTargetFollowsProjectRevision(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 100, Width: 1080, Height: 1920}}
	p.Tracks[0].Clips = []project.Clip{
		{ID: "late", AssetID: "a", Start: 40, Out: 10},
		{ID: "early", AssetID: "a", Start: 10, Out: 10},
	}
	e := editor{model: editorstate.New(p)}
	e.model.SelectIndex(0)
	index, at, ok, err := e.previewTarget(0, true)
	if err != nil || !ok || index != 1 || at != 10 {
		t.Fatal("selected clip overrode timeline target")
	}
	cached := e.composition
	index, at, ok, err = e.previewTarget(12, false)
	if err != nil || !ok || index != 1 || at != 12 || cached != e.composition {
		t.Fatal("composition not reused")
	}
	q := p.Clone()
	q.Tracks[0].Clips[1].Start = 20
	if changed, err := e.model.Commit(q, editorstate.SelectionUpdate{}); err != nil || !changed {
		t.Fatal("project edit failed")
	}
	index, at, ok, err = e.previewTarget(0, true)
	if err != nil || !ok || index != 1 || at != 20 || cached == e.composition {
		t.Fatal("stale composition reused after edit")
	}
	e.model.Undo()
	_, at, ok, err = e.previewTarget(0, true)
	if err != nil || !ok || at != 10 {
		t.Fatal("undo did not invalidate composition")
	}
}
