package media

import (
	"errors"
	"fmt"
)

type ColorFamily string

const (
	ColorHLG    ColorFamily = "hlg"
	ColorSDR709 ColorFamily = "sdr-bt709"
	ColorPQ     ColorFamily = "pq-bt2020"
)

type ColorDefaults uint8

const (
	DefaultPrimaries ColorDefaults = 1 << iota
	DefaultTransfer
	DefaultMatrix
	DefaultRange
	DefaultChroma
	DefaultSAR
)

const allColorDefaults = DefaultPrimaries | DefaultTransfer | DefaultMatrix | DefaultRange | DefaultChroma | DefaultSAR

type SourceColor struct {
	Family                            ColorFamily
	Defaults                          ColorDefaults
	Codec, PixelFormat, Range, Chroma string
}

func (c SourceColor) Validate() error {
	if c.Defaults&^allColorDefaults != 0 || (c.Range != "tv" && c.Range != "pc") || (c.Chroma != "left" && c.Chroma != "center") {
		return errors.New("invalid source color descriptor")
	}
	switch c.Family {
	case ColorHLG:
		if c.Defaults == 0 && c.PixelFormat == "yuv420p10le" {
			return nil
		}
	case ColorSDR709:
		if (c.Codec == "h264" || c.Codec == "hevc") && (c.PixelFormat == "yuv420p" || c.PixelFormat == "yuv420p10le") && (c.Defaults == 0 || c.PixelFormat == "yuv420p") {
			if (c.Defaults&DefaultRange == 0 || c.Range == "tv") && (c.Defaults&DefaultChroma == 0 || c.Chroma == "left") {
				return nil
			}
		}
	case ColorPQ:
		if c.Codec == "hevc" && c.PixelFormat == "yuv420p10le" && c.Defaults == 0 {
			return nil
		}
	}
	return errors.New("unsupported source color descriptor")
}

func missingColorTag(s string) bool {
	return s == "" || s == "unknown" || s == "unspecified" || s == "N/A"
}

func sourceSAR(s string) string {
	if s == "0:1" {
		return ""
	}
	return s
}

func (v Stream) ClassifySourceColor() (SourceColor, error) {
	c := SourceColor{Codec: v.CodecName, PixelFormat: v.PixelFormat, Range: v.Range, Chroma: v.ChromaLocation}
	if v.ValidateHLG() == nil {
		c.Family = ColorHLG
		return c, nil
	}
	if v.Width <= 0 || v.Height <= 0 || v.Width%2 != 0 || v.Height%2 != 0 {
		return SourceColor{}, errors.New("invalid or odd input dimensions")
	}
	if _, err := v.DisplayRotation(); err != nil {
		return SourceColor{}, err
	}
	if v.CodecName != "h264" && v.CodecName != "hevc" {
		return SourceColor{}, fmt.Errorf("unsupported source codec %q", v.CodecName)
	}
	if v.PixelFormat != "yuv420p" && v.PixelFormat != "yuv420p10le" {
		return SourceColor{}, fmt.Errorf("unsupported source pixel format %q; expected planar 4:2:0", v.PixelFormat)
	}
	if v.Transfer == "smpte2084" {
		if v.Primaries != "bt2020" || v.Matrix != "bt2020nc" || v.SampleAspectRatio != "1:1" {
			return SourceColor{}, errors.New("PQ requires explicit BT.2020 nonconstant-luminance color and square pixels")
		}
		c.Family = ColorPQ
		if err := c.Validate(); err != nil {
			return SourceColor{}, err
		}
		return c, nil
	}
	if v.HasStaticHDRMetadata() {
		return SourceColor{}, errors.New("static HDR metadata conflicts with SDR classification")
	}
	infer := v.PixelFormat == "yuv420p" && max(v.Width, v.Height) >= 1280 && min(v.Width, v.Height) >= 720
	resolve := func(value, want string, bit ColorDefaults) (string, error) {
		if value == want {
			return value, nil
		}
		if infer && missingColorTag(value) {
			c.Defaults |= bit
			return want, nil
		}
		return "", fmt.Errorf("unsupported or ambiguous SDR color signaling: expected %s, got %q", want, value)
	}
	for _, tag := range []struct {
		value, want string
		bit         ColorDefaults
	}{
		{v.Primaries, "bt709", DefaultPrimaries},
		{v.Transfer, "bt709", DefaultTransfer},
		{v.Matrix, "bt709", DefaultMatrix},
		{sourceSAR(v.SampleAspectRatio), "1:1", DefaultSAR},
	} {
		if _, err := resolve(tag.value, tag.want, tag.bit); err != nil {
			return SourceColor{}, err
		}
	}
	var err error
	if c.Range != "pc" {
		c.Range, err = resolve(c.Range, "tv", DefaultRange)
		if err != nil {
			return SourceColor{}, err
		}
	}
	if c.Chroma != "center" {
		c.Chroma, err = resolve(c.Chroma, "left", DefaultChroma)
		if err != nil {
			return SourceColor{}, err
		}
	}
	c.Family = ColorSDR709
	if err := c.Validate(); err != nil {
		return SourceColor{}, err
	}
	return c, nil
}

func (r Report) ValidateClassifiedImportStreams() error {
	if r.audioCount() > 1 {
		return errors.New("multiple source audio streams are not supported")
	}
	return r.validateExportStreams()
}
