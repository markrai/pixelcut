package project

const MaxGainPercent = 400

func (c Clip) GainPercent() int {
	if c.Gain == nil {
		return 100
	}
	return *c.Gain
}

func (p Project) SetClipGain(ref ClipRef, percent int) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return p, ErrClipNotFound
	}
	c := p.Tracks[ti].Clips[ci]
	a, _ := p.Asset(c.AssetID)
	if !a.HasAudio || c.AudioDetached || percent < 0 || percent > MaxGainPercent {
		return p, ErrInvalidClip
	}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return p, err
	}
	if c.GainPercent() == percent {
		return p, nil
	}
	q := p.Clone()
	if percent == 100 {
		q.Tracks[ti].Clips[ci].Gain = nil
	} else {
		q.Version = Version
		q.Tracks[ti].Clips[ci].Gain = &percent
	}
	return q, q.Validate()
}
