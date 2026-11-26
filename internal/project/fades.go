package project

const MaxAudioFade = 60 * TicksPerSecond

func ValidAudioFades(in, out int64) bool {
	return in >= 0 && out >= 0 && in <= MaxAudioFade && out <= MaxAudioFade
}

func (c Clip) HasFades() bool {
	return c.FadeIn != 0 || c.FadeOut != 0
}

func (p Project) SetClipFades(ref ClipRef, in, out int64) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	ti, ci, ok := p.ClipIndex(ref)
	if !ok {
		return p, ErrClipNotFound
	}
	c := p.Tracks[ti].Clips[ci]
	a, _ := p.Asset(c.AssetID)
	if !a.HasAudio || c.AudioDetached || !ValidAudioFades(in, out) {
		return p, ErrInvalidClip
	}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return p, err
	}
	if c.FadeIn == in && c.FadeOut == out {
		return p, nil
	}
	q := p.Clone()
	q.Version = Version
	q.Tracks[ti].Clips[ci].FadeIn = in
	q.Tracks[ti].Clips[ci].FadeOut = out
	return q, q.Validate()
}
