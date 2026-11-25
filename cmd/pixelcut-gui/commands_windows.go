package main

import editorstate "github.com/markrai/pixelcut/internal/editor"

func commandRequirement(id int) editorstate.Requirement {
	switch id {
	case idApply, idSplit, menuSelectAsset, menuRevealClip, menuRelinkClip, menuReplaceClip:
		return editorstate.SingleClip
	case idCropVisual, idCropReset, idTransform, idExport, idFrameBack, idFrameForward, idSetIn, idSetOut:
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

func eligibleItems(items []menuItem, a editorstate.Availability) []menuItem {
	for i := range items {
		if items[i].enabled && a.Check(commandRequirement(items[i].id)) != nil {
			items[i].enabled = false
		}
	}
	return items
}
