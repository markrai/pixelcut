package editor

import (
	"errors"

	"github.com/markrai/pixelcut/internal/project"
)

type Requirement uint8

const (
	Unrestricted Requirement = iota
	SingleClip
	VideoClip
	SelectedClips
	SelectedAsset
	Clipboard
	UndoAvailable
	RedoAvailable
)

type Availability struct {
	ClipCount       int
	AudioTrack      bool
	TrackLocked     bool
	VideoSuppressed bool
	AssetSelected   bool
	ClipboardReady  bool
	CanUndo         bool
	CanRedo         bool
}

func (a Availability) Check(requirement Requirement) error {
	switch requirement {
	case SingleClip, VideoClip:
		if a.ClipCount != 1 {
			return errors.New("select a single clip for this operation")
		}
		if requirement == VideoClip && a.AudioTrack {
			return errors.New("this operation requires a video track")
		}
	case SelectedClips:
		if a.ClipCount == 0 {
			return errors.New("select a timeline clip")
		}
	case SelectedAsset:
		if !a.AssetSelected {
			return errors.New("select a media asset")
		}
	case Clipboard:
		if !a.ClipboardReady {
			return errors.New("clip clipboard is empty")
		}
	case UndoAvailable:
		if !a.CanUndo {
			return errors.New("nothing to undo")
		}
	case RedoAvailable:
		if !a.CanRedo {
			return errors.New("nothing to redo")
		}
	case Unrestricted:
	default:
		return errors.New("unknown command requirement")
	}
	return nil
}

func (s *State) Availability(clipboardReady bool) Availability {
	i := s.TrackIndex()
	return Availability{
		ClipCount:       s.Count(),
		AudioTrack:      i >= 0 && s.p.Tracks[i].Kind == project.AudioTrack,
		TrackLocked:     i >= 0 && s.p.Tracks[i].Locked,
		VideoSuppressed: !s.p.TrackPolicies()[s.TrackID()].Video,
		AssetSelected:   s.AssetIndex() >= 0,
		ClipboardReady:  clipboardReady,
		CanUndo:         s.CanUndo(),
		CanRedo:         s.CanRedo(),
	}
}
