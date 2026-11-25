package editor

import "github.com/markrai/pixelcut/internal/project"

type State struct {
	p                project.Project
	track            string
	asset            string
	ids              map[string]bool
	primary          string
	anchor           string
	history          history
	fingerprint      string
	savedFingerprint string
}

func New(p project.Project) State {
	p = p.Clone()
	fingerprint := p.Fingerprint()
	return State{p: p, track: "V1", fingerprint: fingerprint, savedFingerprint: fingerprint}
}

func (s *State) Project() project.Project { return s.p }

func (s *State) Fingerprint() string { return s.fingerprint }

func (s *State) TrackIndex() int {
	i := s.p.TrackIndex(s.track)
	if i < 0 {
		return s.p.TrackIndex("V1")
	}
	return i
}

func (s *State) TrackID() string {
	if i := s.TrackIndex(); i >= 0 {
		return s.p.Tracks[i].ID
	}
	return ""
}

func (s *State) AssetID() string { return s.asset }

func (s *State) AssetIndex() int { return s.p.AssetIndex(s.asset) }

func (s *State) PrimaryID() string { return s.primary }

func (s *State) AnchorID() string { return s.anchor }

func (s *State) Count() int { return len(s.ids) }

func (s *State) IDs() []string {
	var ids []string
	if i := s.TrackIndex(); i >= 0 {
		for _, c := range s.p.Tracks[i].Clips {
			if s.ids[c.ID] {
				ids = append(ids, c.ID)
			}
		}
	}
	return ids
}

func (s *State) Contains(id string) bool {
	return s.ids[id]
}

func (s *State) ClipIndex() int {
	_, i, ok := s.p.ClipIndex(project.ClipRef{TrackID: s.TrackID(), ClipID: s.primary})
	if !ok {
		return -1
	}
	return i
}

func (s *State) Selected() (project.Clip, bool) {
	return s.p.Clip(project.ClipRef{TrackID: s.TrackID(), ClipID: s.primary})
}

func (s *State) SetAssetID(id string) {
	if s.p.AssetIndex(id) < 0 {
		id = ""
	}
	s.asset = id
}

func (s *State) SetAssetIndex(i int) {
	id := ""
	if i >= 0 && i < len(s.p.Assets) {
		id = s.p.Assets[i].ID
	}
	s.SetAssetID(id)
}

func (s *State) Clear() {
	s.ids = nil
	s.primary = ""
	s.anchor = ""
}

func (s *State) SwitchTrack(id string) bool {
	if s.p.TrackIndex(id) < 0 || id == s.TrackID() {
		return false
	}
	s.track = id
	s.Clear()
	return true
}

func (s *State) Select(ids []string, primary string) {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	s.ids = make(map[string]bool, len(wanted))
	s.primary = ""
	if i := s.TrackIndex(); i >= 0 {
		for _, c := range s.p.Tracks[i].Clips {
			if wanted[c.ID] {
				s.ids[c.ID] = true
				if s.primary == "" || c.ID == primary {
					s.primary = c.ID
				}
			}
		}
	}
	s.SetAnchor(s.anchor)
}

func (s *State) SetAnchor(id string) {
	if _, _, ok := s.p.ClipIndex(project.ClipRef{TrackID: s.TrackID(), ClipID: id}); !ok {
		id = ""
	}
	s.anchor = id
}

func (s *State) SelectIndex(i int) {
	ti := s.TrackIndex()
	if ti < 0 || i < 0 || i >= len(s.p.Tracks[ti].Clips) {
		s.Clear()
		return
	}
	id := s.p.Tracks[ti].Clips[i].ID
	s.Select([]string{id}, id)
	s.SetAnchor(id)
}

func (s *State) SetPrimary(id string) {
	if s.Contains(id) {
		s.primary = id
	}
}

func (s *State) SelectAll() {
	if i := s.TrackIndex(); i >= 0 {
		ids := make([]string, 0, len(s.p.Tracks[i].Clips))
		for _, c := range s.p.Tracks[i].Clips {
			ids = append(ids, c.ID)
		}
		s.Select(ids, "")
	}
	s.anchor = ""
}

func (s *State) Modify(id string, ctrl, shift bool) {
	ti := s.TrackIndex()
	if _, _, ok := s.p.ClipIndex(project.ClipRef{TrackID: s.TrackID(), ClipID: id}); !ok || ti < 0 {
		return
	}
	if shift {
		anchor := s.anchor
		if anchor == "" {
			anchor = s.primary
		}
		if anchor == "" {
			anchor = id
		}
		s.Select(project.RangeClipIDs(s.p.Tracks[ti].Clips, anchor, id), id)
		s.SetAnchor(anchor)
	} else if ctrl {
		ids := make([]string, 0, len(s.ids)+1)
		for _, selected := range s.IDs() {
			if selected != id {
				ids = append(ids, selected)
			}
		}
		if !s.Contains(id) {
			ids = append(ids, id)
		}
		s.Select(ids, id)
		s.SetAnchor(id)
	}
}

func (s *State) adopt(p project.Project) {
	ids, primary, anchor := s.IDs(), s.primary, s.anchor
	s.p = p
	s.SetAssetID(s.asset)
	if ref, ok := p.LocateClip(primary); ok {
		s.track = ref.TrackID
	} else if p.TrackIndex(s.track) < 0 {
		s.track = "V1"
	}
	s.Select(ids, primary)
	s.SetAnchor(anchor)
}

func (s *State) Restore(track string, asset, clip int, group []string) {
	s.Clear()
	s.track = "V1"
	s.SwitchTrack(track)
	s.SetAssetIndex(asset)
	s.SelectIndex(clip)
	if len(group) > 1 {
		s.Select(group, s.primary)
	}
}
