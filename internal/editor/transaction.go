package editor

import (
	"errors"
	"fmt"

	"github.com/markrai/pixelcut/internal/project"
)

const HistoryLimit = 50

type SelectionUpdate struct {
	TrackID      string
	ReplaceClips bool
	ClipIDs      []string
	Primary      string
	Anchor       string
	ReplaceAsset bool
	AssetID      string
}

type snapshot struct {
	p                             project.Project
	fingerprint                   string
	track, asset, primary, anchor string
	ids                           []string
}

type historyEntry struct {
	before, after snapshot
}

type history struct {
	undo, redo []historyEntry
}

func (s *State) snapshot() snapshot {
	return snapshot{p: s.p, fingerprint: s.fingerprint, track: s.TrackID(), asset: s.asset, primary: s.primary, anchor: s.anchor, ids: s.IDs()}
}

func (s *State) restoreSnapshot(v snapshot) {
	s.p, s.fingerprint = v.p, v.fingerprint
	s.track, s.asset = v.track, v.asset
	s.Select(v.ids, v.primary)
	s.SetAnchor(v.anchor)
}

func (s *State) SavedFingerprint() string { return s.savedFingerprint }

func (s *State) Dirty() bool { return s.savedFingerprint == "" || s.fingerprint != s.savedFingerprint }

func (s *State) MarkSaved() { s.savedFingerprint = s.fingerprint }

func (s *State) SetSavedBaseline(fingerprint string) {
	if !project.ValidRevision(fingerprint) {
		fingerprint = ""
	}
	s.savedFingerprint = fingerprint
}

func (s *State) CanUndo() bool { return len(s.history.undo) > 0 }

func (s *State) CanRedo() bool { return len(s.history.redo) > 0 }

func (s *State) applySelection(u SelectionUpdate) error {
	if u.TrackID != "" {
		if s.p.TrackIndex(u.TrackID) < 0 {
			return project.ErrTrackNotFound
		}
		s.SwitchTrack(u.TrackID)
	}
	if u.ReplaceClips {
		seen := make(map[string]bool, len(u.ClipIDs))
		for _, id := range u.ClipIDs {
			if _, _, ok := s.p.ClipIndex(project.ClipRef{TrackID: s.TrackID(), ClipID: id}); !ok || seen[id] {
				return fmt.Errorf("invalid selection reference %q", id)
			}
			seen[id] = true
		}
		if u.Primary != "" && !seen[u.Primary] {
			return errors.New("primary clip is outside selection")
		}
		if u.Anchor != "" {
			if _, _, ok := s.p.ClipIndex(project.ClipRef{TrackID: s.TrackID(), ClipID: u.Anchor}); !ok {
				return errors.New("invalid selection anchor")
			}
		}
		s.Select(u.ClipIDs, u.Primary)
		s.SetAnchor(u.Anchor)
	} else if len(u.ClipIDs) != 0 || u.Primary != "" || u.Anchor != "" {
		return errors.New("selection replacement not requested")
	}
	if u.ReplaceAsset {
		if u.AssetID != "" && s.p.AssetIndex(u.AssetID) < 0 {
			return project.ErrAssetNotFound
		}
		s.SetAssetID(u.AssetID)
	} else if u.AssetID != "" {
		return errors.New("asset replacement not requested")
	}
	return nil
}

func (s *State) Commit(p project.Project, u SelectionUpdate) (bool, error) {
	if err := p.Validate(); err != nil {
		return false, err
	}
	p = p.Clone()
	fingerprint := p.Fingerprint()
	if fingerprint == "" {
		return false, errors.New("project fingerprint unavailable")
	}
	candidate := *s
	candidate.adopt(p)
	candidate.fingerprint = fingerprint
	if err := candidate.applySelection(u); err != nil {
		return false, err
	}
	if fingerprint == s.fingerprint {
		s.restoreSnapshot(candidate.snapshot())
		return false, nil
	}
	entry := historyEntry{before: s.snapshot(), after: candidate.snapshot()}
	candidate.history.undo = append(s.history.undo, entry)
	if len(candidate.history.undo) > HistoryLimit {
		candidate.history.undo[0] = historyEntry{}
		candidate.history.undo = candidate.history.undo[1:]
	}
	candidate.history.redo = nil
	*s = candidate
	return true, nil
}

func pop(stack *[]historyEntry) historyEntry {
	i := len(*stack) - 1
	entry := (*stack)[i]
	(*stack)[i] = historyEntry{}
	*stack = (*stack)[:i]
	return entry
}

func (s *State) Undo() bool {
	if !s.CanUndo() {
		return false
	}
	entry := pop(&s.history.undo)
	s.restoreSnapshot(entry.before)
	s.history.redo = append(s.history.redo, entry)
	return true
}

func (s *State) Redo() bool {
	if !s.CanRedo() {
		return false
	}
	entry := pop(&s.history.redo)
	s.restoreSnapshot(entry.after)
	s.history.undo = append(s.history.undo, entry)
	return true
}
