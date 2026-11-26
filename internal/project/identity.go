package project

type ClipRef struct {
	TrackID string
	ClipID  string
}

func (p Project) AssetIndex(id string) int {
	for i, a := range p.Assets {
		if a.ID == id {
			return i
		}
	}
	return -1
}

func (p Project) Asset(id string) (Asset, bool) {
	i := p.AssetIndex(id)
	if i < 0 {
		return Asset{}, false
	}
	return p.Assets[i], true
}

func (p Project) TrackIndex(id string) int {
	for i, t := range p.Tracks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (p Project) ClipIndex(ref ClipRef) (int, int, bool) {
	ti := p.TrackIndex(ref.TrackID)
	if ti >= 0 {
		for ci, c := range p.Tracks[ti].Clips {
			if c.ID == ref.ClipID {
				return ti, ci, true
			}
		}
	}
	return -1, -1, false
}

func (p Project) LocateClip(id string) (ClipRef, bool) {
	for _, t := range p.Tracks {
		for _, c := range t.Clips {
			if c.ID == id {
				return ClipRef{TrackID: t.ID, ClipID: id}, true
			}
		}
	}
	return ClipRef{}, false
}

func (p Project) Clip(ref ClipRef) (Clip, bool) {
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return Clip{}, false
	}
	return p.Tracks[ti].Clips[ci].Clone(), true
}

func (c Clip) Clone() Clip {
	if c.Crop != nil {
		v := *c.Crop
		c.Crop = &v
	}
	if c.Transform != nil {
		v := *c.Transform
		c.Transform = &v
	}
	if c.Opacity != nil {
		v := *c.Opacity
		c.Opacity = &v
	}
	if c.Gain != nil {
		v := *c.Gain
		c.Gain = &v
	}
	return c
}
