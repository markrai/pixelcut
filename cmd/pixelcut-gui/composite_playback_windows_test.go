package main

import (
	"context"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestCompositePlaybackMenuAndRetirement(t *testing.T) {
	for _, items := range [][]menuItem{timelineMenuItems(timelineMenuState{}), trackMenuItems(project.New(), "V1", nil)} {
		for _, id := range []int{menuCompositePlay, menuSourcePlay} {
			v, ok := findMenuItem(items, id)
			if !ok || !v.enabled {
				t.Fatalf("playback mode unavailable: %d", id)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &compositePreviewRun{cancel: cancel, done: make(chan struct{})}
	e := editor{compositeRun: run, stillActive: true}
	if !e.playing() {
		t.Fatal("composite run not reported as playing")
	}
	e.cancelCompositeRun()
	if e.compositeRun != nil || ctx.Err() == nil || e.compositeRetired() || e.playing() {
		t.Fatal("composite cancellation did not retire the run")
	}
	close(run.done)
	if !e.compositeRetired() || e.retiringComposite != nil {
		t.Fatal("completed decoder retirement not cleared")
	}
}
