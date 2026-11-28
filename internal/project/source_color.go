package project

import (
	"errors"

	"github.com/markrai/pixelcut/internal/media"
)

var ErrImportOnlySource = errors.New("source is media-bin only: confirm inferred SDR tags or wait for PQ rendering support")
var ErrMixedExportUnavailable = errors.New("SDR/PQ export is not available yet; use HLG-only timeline footage")

func (a Asset) Classified() bool {
	return a.ColorFamily != ""
}

func (a Asset) ImportOnly() bool {
	return a.ColorFamily == media.ColorPQ || (a.ColorFamily == media.ColorSDR709 && a.ColorDefaults != 0 && !a.ColorConfirmed)
}

func (a Asset) SourceColor() media.SourceColor {
	return media.SourceColor{Family: a.ColorFamily, Defaults: a.ColorDefaults, Codec: a.Codec, PixelFormat: a.PixelFormat, Range: a.Range, Chroma: a.Chroma}
}

func (p Project) ConfirmSDRColor(id string) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	a, ok := p.Asset(id)
	if !ok {
		return p, ErrAssetNotFound
	}
	if a.ColorFamily != media.ColorSDR709 || a.ColorDefaults == 0 {
		return p, errors.New("source does not require SDR color confirmation")
	}
	if a.ColorConfirmed {
		return p, nil
	}
	if err := p.RequireEditableAsset(id); err != nil {
		return p, err
	}
	q := p.Clone()
	for i := range q.Assets {
		if q.Assets[i].ID == id {
			q.Assets[i].ColorConfirmed = true
		}
	}
	q.Version = Version
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func (p Project) HasClassifiedClips() bool {
	for _, t := range p.Tracks {
		for _, c := range t.Clips {
			if a, ok := p.Asset(c.AssetID); ok && a.Classified() {
				return true
			}
		}
	}
	return false
}

func (a Asset) PreviewColor() media.SourceColor {
	if !a.Classified() {
		return media.SourceColor{}
	}
	return a.SourceColor()
}
