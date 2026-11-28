package project

import (
	"errors"

	"github.com/markrai/pixelcut/internal/media"
)

var ErrImportOnlySource = errors.New("source is in the media bin only; SDR/PQ timeline rendering is not available yet")

func (a Asset) ImportOnly() bool {
	return a.ColorFamily != ""
}

func (a Asset) SourceColor() media.SourceColor {
	return media.SourceColor{Family: a.ColorFamily, Defaults: a.ColorDefaults, Codec: a.Codec, PixelFormat: a.PixelFormat, Range: a.Range, Chroma: a.Chroma}
}
