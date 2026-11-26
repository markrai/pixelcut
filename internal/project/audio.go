package project

import "errors"

func (p Project) DetachAudio(video ClipRef, target, id string) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	ti, ci, ok := p.ClipIndex(video)
	if !ok {
		return p, ErrClipNotFound
	}
	dst := p.TrackIndex(target)
	c := p.Tracks[ti].Clips[ci]
	a, _ := p.Asset(c.AssetID)
	if p.Tracks[ti].Kind != VideoTrack || !a.HasAudio || c.AudioDetached {
		return p, errors.New("select a video clip with attached source audio")
	}
	if dst < 0 || p.Tracks[dst].Kind != AudioTrack {
		return p, errors.New("detach requires an audio track")
	}
	if err := p.RequireEditableTrack(video.TrackID); err != nil {
		return p, err
	}
	if err := p.RequireEditableTrack(target); err != nil {
		return p, err
	}
	q := p.Clone()
	q.Version = Version
	q.Tracks[ti].Clips[ci].AudioDetached = true
	q.Tracks[dst].Clips = append(q.Tracks[dst].Clips, Clip{ID: id, AssetID: c.AssetID, Start: c.Start, In: c.In, Out: c.Out})
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func (p Project) MatchingAudio(video ClipRef) []ClipRef {
	c, ok := p.Clip(video)
	ti := p.TrackIndex(video.TrackID)
	if !ok || ti < 0 || p.Tracks[ti].Kind != VideoTrack || !c.AudioDetached {
		return nil
	}
	var refs []ClipRef
	for _, t := range p.Tracks {
		if t.Kind != AudioTrack {
			continue
		}
		for _, a := range t.Clips {
			if audioAligned(c, a) {
				refs = append(refs, ClipRef{TrackID: t.ID, ClipID: a.ID})
			}
		}
	}
	return refs
}

func audioAligned(video, audio Clip) bool {
	return video.AssetID == audio.AssetID && video.Start == audio.Start && video.In == audio.In && video.Out == audio.Out
}

func (p Project) ReattachAudio(video, audio ClipRef) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	vi, vc, vok := p.ClipIndex(video)
	ai, ac, aok := p.ClipIndex(audio)
	if !vok || !aok {
		return p, ErrClipNotFound
	}
	v, a := p.Tracks[vi].Clips[vc], p.Tracks[ai].Clips[ac]
	if p.Tracks[vi].Kind != VideoTrack || p.Tracks[ai].Kind != AudioTrack || !v.AudioDetached || !audioAligned(v, a) {
		return p, errors.New("reattach requires detached video and audio with identical source and timeline intervals")
	}
	if err := p.RequireEditableTrack(video.TrackID); err != nil {
		return p, err
	}
	if err := p.RequireEditableTrack(audio.TrackID); err != nil {
		return p, err
	}
	q := p.Clone()
	q.Tracks[vi].Clips[vc].AudioDetached = false
	clips := q.Tracks[ai].Clips
	q.Tracks[ai].Clips = append(clips[:ac], clips[ac+1:]...)
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}
