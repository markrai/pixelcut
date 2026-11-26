package main

import (
	"errors"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func TestTrackControlMenuAndCommandEligibility(t *testing.T) {
	track := project.Track{Kind: project.VideoTrack, Hidden: true, Muted: true, Solo: true, Locked: true}
	items := trackControlItems(track)
	for _, id := range []int{trackMenuHide, trackMenuMute, trackMenuSolo, trackMenuLock} {
		v, ok := findMenuItem(items, id)
		if !ok || !v.enabled || !v.checked {
			t.Fatalf("incorrect track control menu: %d", id)
		}
	}
	items = trackControlItems(project.Track{Kind: project.AudioTrack})
	if v, ok := findMenuItem(items, trackMenuHide); !ok || v.enabled {
		t.Fatal("hide available on audio track")
	}
	a := editorstate.Availability{ClipCount: 1, TrackLocked: true, AssetSelected: true, ClipboardReady: true}
	for _, id := range []int{idApply, idSplit, idDelete, idCut, idDuplicate, idCropVisual, idTransform, idAdd, idPaste, menuDetachAudio, menuReattachAudio, menuGapHere} {
		if !errors.Is(trackCommandAllowed(id, a), project.ErrTrackLocked) {
			t.Fatalf("locked mutation allowed: %d", id)
		}
	}
	for _, id := range []int{idCopy, idPlay, idExport, idUndo, idRedo, idTracks} {
		if trackCommandAllowed(id, a) != nil {
			t.Fatalf("nonmutating command blocked: %d", id)
		}
	}
	a.TrackLocked, a.VideoSuppressed = false, true
	if trackCommandAllowed(idCropVisual, a) == nil || trackCommandAllowed(idTransform, a) == nil || trackCommandAllowed(idSplit, a) != nil {
		t.Fatal("hidden video visual edit eligibility")
	}
}
