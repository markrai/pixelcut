package main

import (
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
)

func TestMenuUsesCommandRequirements(t *testing.T) {
	items := []menuItem{item(idCropVisual, "crop", true), item(idSplit, "split", true), item(idDelete, "delete", true), item(idPaste, "paste", true), item(idUndo, "undo", true), item(idSnap, "snap", true)}
	a := editorstate.Availability{ClipCount: 1, AudioTrack: true, CanUndo: true}
	for _, v := range eligibleItems(items, a) {
		want := a.Check(commandRequirement(v.id)) == nil
		if v.enabled != want {
			t.Fatalf("menu/command disagreement: %d", v.id)
		}
	}
	if items[0].enabled || items[3].enabled || !items[1].enabled || !items[2].enabled || !items[4].enabled {
		t.Fatal("unexpected command availability")
	}
}
