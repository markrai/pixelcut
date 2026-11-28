package project

import (
	"errors"
	"os"
	"path/filepath"
)

func (p Project) AssetReferences(id string) int {
	n := 0
	for _, track := range p.Tracks {
		for _, clip := range track.Clips {
			if clip.AssetID == id {
				n++
			}
		}
	}
	return n
}

func (p Project) RemoveAsset(id string, removeClips bool) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	if _, ok := p.Asset(id); !ok {
		return p, ErrAssetNotFound
	}
	if err := p.RequireEditableAsset(id); err != nil {
		return p, err
	}
	if !removeClips && p.AssetReferences(id) > 0 {
		return p, errors.New("asset is used by timeline clips")
	}
	q := p.Clone()
	assets := q.Assets[:0]
	for _, a := range q.Assets {
		if a.ID != id {
			assets = append(assets, a)
		}
	}
	q.Assets = assets
	for i := range q.Tracks {
		clips := q.Tracks[i].Clips[:0]
		for _, c := range q.Tracks[i].Clips {
			if c.AssetID != id {
				clips = append(clips, c)
			}
		}
		q.Tracks[i].Clips = clips
	}
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func sameSource(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if a == b {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && ai.Mode().IsRegular() && bi.Mode().IsRegular() && os.SameFile(ai, bi)
}

func (p Project) FindAssetPath(path string) (int, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return -1, false
	}
	for i, a := range p.Assets {
		if sameSource(a.Path, abs) {
			return i, true
		}
	}
	return -1, false
}

func (p Project) AddAsset(a Asset) (Project, int, bool, error) {
	if err := p.Validate(); err != nil {
		return p, -1, false, err
	}
	if err := validateAsset(a); err != nil {
		return p, -1, false, err
	}
	if i, ok := p.FindAssetPath(a.Path); ok {
		return p, i, false, nil
	}
	q := p.Clone()
	q.Assets = append(q.Assets, a)
	if a.Classified() {
		q.Version = Version
	}
	if err := q.Validate(); err != nil {
		return p, -1, false, err
	}
	return q, len(q.Assets) - 1, true, nil
}

func compatibleAssetMetadata(a, b Asset) bool {
	return a.Duration == b.Duration && a.Width == b.Width && a.Height == b.Height && a.HasAudio == b.HasAudio && a.Range == b.Range && a.SourceColor() == b.SourceColor() && a.ColorConfirmed == b.ColorConfirmed
}

func (p Project) MergeDuplicateAssets() (Project, map[string]string, error) {
	if err := p.Validate(); err != nil {
		return p, nil, err
	}
	q := p.Clone()
	q.Assets = q.Assets[:0]
	remap := make(map[string]string)
	for _, a := range p.Assets {
		duplicate := false
		for _, existing := range q.Assets {
			if compatibleAssetMetadata(existing, a) && sameSource(existing.Path, a.Path) {
				remap[a.ID] = existing.ID
				duplicate = true
				break
			}
		}
		if !duplicate {
			q.Assets = append(q.Assets, a)
		}
	}
	for id := range remap {
		if err := p.RequireEditableAsset(id); err != nil {
			return p, nil, err
		}
	}
	if len(remap) == 0 {
		return p, remap, nil
	}
	for i := range q.Tracks {
		for j := range q.Tracks[i].Clips {
			c := &q.Tracks[i].Clips[j]
			if id, ok := remap[c.AssetID]; ok {
				c.AssetID = id
			}
		}
	}
	if err := q.Validate(); err != nil {
		return p, nil, err
	}
	return q, remap, nil
}
