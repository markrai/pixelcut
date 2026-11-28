package main

import (
	"fmt"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func commandRequirement(id int) editorstate.Requirement {
	switch id {
	case idApply, idSplit, idClipAudio, menuClipMute, menuSelectAsset, menuRevealClip, menuRelinkClip, menuReplaceClip:
		return editorstate.SingleClip
	case idCropVisual, idCropReset, idTransform, idExport, idFrameBack, idFrameForward, idSetIn, idSetOut, menuDetachAudio, menuReattachAudio, menuLayerOpacity, menuLayerPreset, menuTransitionDuration, menuTransitionRemove:
		return editorstate.VideoClip
	case idCopy, idCut, idDuplicate, idDelete, idRippleDelete:
		return editorstate.SelectedClips
	case idAdd, idRemoveAsset, idRelinkAsset, idReplaceAsset, menuRevealAsset:
		return editorstate.SelectedAsset
	case idPaste, menuPasteHere:
		return editorstate.Clipboard
	case idUndo:
		return editorstate.UndoAvailable
	case idRedo:
		return editorstate.RedoAvailable
	default:
		return editorstate.Unrestricted
	}
}

func (e *editor) availability() editorstate.Availability {
	return e.model.Availability(e.clipboard.Count() > 0)
}

func trackCommandAllowed(id int, a editorstate.Availability) error {
	if a.TrackLocked {
		switch id {
		case idApply, idSplit, idCropVisual, idCropReset, idTransform, idSetIn, idSetOut, menuDetachAudio, menuReattachAudio, menuLayerOpacity, menuLayerPreset, menuTransitionDuration, menuTransitionRemove, idClipAudio, menuClipMute, idCut, idDuplicate, idDelete, idRippleDelete, idAdd, idPaste, menuPasteHere, menuGapHere:
			return project.ErrTrackLocked
		}
	}
	if a.VideoSuppressed && (id == idCropVisual || id == idTransform || id == menuTransitionDuration) {
		return fmt.Errorf("enable the active track's video preview before visual editing")
	}
	return nil
}

func eligibleItems(items []menuItem, a editorstate.Availability) []menuItem {
	for i := range items {
		if items[i].enabled && (a.Check(commandRequirement(items[i].id)) != nil || trackCommandAllowed(items[i].id, a) != nil) {
			items[i].enabled = false
		}
	}
	return items
}
