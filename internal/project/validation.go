package project

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/markrai/pixelcut/internal/media"
)

const (
	VideoTrack = "video"
	AudioTrack = "audio"
)

var (
	ErrUnsupportedVersion = errors.New("unsupported project version")
	ErrInvalidTrack       = errors.New("invalid track")
	ErrInvalidAsset       = errors.New("invalid asset")
	ErrInvalidClip        = errors.New("invalid clip")
	ErrOverlap            = errors.New("overlapping clips")
	ErrClipNotFound       = errors.New("clip not found")
	ErrTrackNotFound      = errors.New("track not found")
	ErrAssetNotFound      = errors.New("asset not found")
)

func SupportedVersion(version int) bool {
	return version == LegacyVersion || version == TrackVersion || version == AudioVersion || version == ControlsVersion || version == LayerVersion || version == GainVersion || version == MuteVersion || version == FadeVersion || version == EnvelopeVersion || version == TransitionVersion || version == Version
}

func validTrackKind(kind string) bool {
	return kind == VideoTrack || kind == AudioTrack
}

func validateTrackLayout(p Project) error {
	if !SupportedVersion(p.Version) {
		return fmt.Errorf("%w %d", ErrUnsupportedVersion, p.Version)
	}
	if p.CanvasWidth != 0 || p.CanvasHeight != 0 {
		if p.Version < LayerVersion {
			return fmt.Errorf("%w: composition canvas requires version %d", ErrUnsupportedVersion, LayerVersion)
		}
		if err := (media.CanvasSize{Width: p.CanvasWidth, Height: p.CanvasHeight}).Validate(); err != nil {
			return err
		}
	}
	if len(p.Tracks) < 2 || len(p.Tracks) > MaxTracks || p.Tracks[0].ID != "V1" || p.Tracks[0].Kind != VideoTrack || p.Tracks[1].ID != "A1" || p.Tracks[1].Kind != AudioTrack {
		return fmt.Errorf("%w: unsupported editor track layout", ErrInvalidTrack)
	}
	if p.Version == LegacyVersion && (len(p.Tracks) != 2 || len(p.Tracks[1].Clips) != 0) {
		return fmt.Errorf("%w: legacy projects require V1 and empty A1", ErrInvalidTrack)
	}
	return nil
}

func validateAsset(a Asset) error {
	if a.ID == "" || !filepath.IsAbs(a.Path) || a.Duration <= 0 || a.Width <= 0 || a.Height <= 0 || a.Width%2 != 0 || a.Height%2 != 0 {
		return fmt.Errorf("%w %q", ErrInvalidAsset, a.ID)
	}
	if a.Range != "" && a.Range != "tv" && a.Range != "pc" {
		return fmt.Errorf("%w %q: unsupported color range", ErrInvalidAsset, a.ID)
	}
	if a.ColorFamily == "" {
		if a.ColorDefaults != 0 || a.Codec != "" || a.PixelFormat != "" || a.Chroma != "" {
			return fmt.Errorf("%w %q: incomplete source color descriptor", ErrInvalidAsset, a.ID)
		}
	} else if a.ColorFamily == media.ColorHLG || a.SourceColor().Validate() != nil || (a.ColorDefaults != 0 && (max(a.Width, a.Height) < 1280 || min(a.Width, a.Height) < 720)) {
		return fmt.Errorf("%w %q: invalid classified color descriptor", ErrInvalidAsset, a.ID)
	}
	return nil
}

func validateClip(c Clip, a Asset, kind string) error {
	if _, ok := c.TimelineInterval(); !ok || c.ID == "" || c.AssetID != a.ID || c.Out > a.Duration {
		return fmt.Errorf("%w %q: invalid identity or interval", ErrInvalidClip, c.ID)
	}
	if a.ImportOnly() {
		return fmt.Errorf("%w %q: %w", ErrInvalidClip, c.ID, ErrImportOnlySource)
	}
	if c.AudioDetached && (kind != VideoTrack || !a.HasAudio) {
		return fmt.Errorf("%w %q: invalid detached audio state", ErrInvalidClip, c.ID)
	}
	if c.Opacity != nil && (kind != VideoTrack || *c.Opacity < 0 || *c.Opacity > 100) {
		return fmt.Errorf("%w %q: opacity requires a video clip and a value from 0 to 100", ErrInvalidClip, c.ID)
	}
	if c.Gain != nil && (!a.HasAudio || c.AudioDetached || *c.Gain < 0 || *c.Gain > MaxGainPercent) {
		return fmt.Errorf("%w %q: gain requires attached or independent source audio and a value from 0 to %d", ErrInvalidClip, c.ID, MaxGainPercent)
	}
	if c.AudioMuted && (!a.HasAudio || c.AudioDetached) {
		return fmt.Errorf("%w %q: clip mute requires attached or independent source audio", ErrInvalidClip, c.ID)
	}
	if !ValidAudioFades(c.FadeIn, c.FadeOut) || (c.HasFades() && (!a.HasAudio || c.AudioDetached)) {
		return fmt.Errorf("%w %q: invalid audio fades", ErrInvalidClip, c.ID)
	}
	if c.Envelope != nil && (!a.HasAudio || c.AudioDetached || !c.Envelope.Valid(a.Duration)) {
		return fmt.Errorf("%w %q: invalid audio envelope", ErrInvalidClip, c.ID)
	}
	if kind == AudioTrack && (!a.HasAudio || c.Crop != nil || c.Transform != nil) {
		return fmt.Errorf("%w %q: invalid audio clip", ErrInvalidClip, c.ID)
	}
	if c.Crop != nil {
		v := media.Stream{Width: a.Width, Height: a.Height}
		if err := (media.Crop{X: c.Crop.X, Y: c.Crop.Y, Width: c.Crop.Width, Height: c.Crop.Height}).Validate(v); err != nil {
			return fmt.Errorf("%w %q: %w", ErrInvalidClip, c.ID, err)
		}
	}
	if c.Transform != nil {
		if a.Range != "" && a.Range != "tv" {
			return fmt.Errorf("%w %q: canvas transforms require limited-range HLG", ErrInvalidClip, c.ID)
		}
		w, h := a.Width, a.Height
		if c.Crop != nil {
			w, h = c.Crop.Width, c.Crop.Height
		}
		if _, err := c.Transform.Placement(w, h); err != nil {
			return fmt.Errorf("%w %q: %w", ErrInvalidClip, c.ID, err)
		}
	}
	return nil
}

func (p Project) Validate() error {
	if err := validateTrackLayout(p); err != nil {
		return err
	}
	assets := make(map[string]Asset, len(p.Assets))
	for _, a := range p.Assets {
		if err := validateAsset(a); err != nil {
			return err
		}
		if a.ImportOnly() && p.Version < Version {
			return fmt.Errorf("%w: classified media requires version %d", ErrUnsupportedVersion, Version)
		}
		if _, ok := assets[a.ID]; ok {
			return fmt.Errorf("%w %q: duplicate identity", ErrInvalidAsset, a.ID)
		}
		assets[a.ID] = a
	}
	tracks, clips := map[string]bool{}, map[string]bool{}
	for _, t := range p.Tracks {
		if t.ID == "" || tracks[t.ID] || !validTrackKind(t.Kind) {
			return fmt.Errorf("%w %q: invalid or duplicate identity or kind", ErrInvalidTrack, t.ID)
		}
		if t.Hidden && t.Kind != VideoTrack {
			return fmt.Errorf("%w: only video tracks support visibility", ErrInvalidTrack)
		}
		if p.Version < ControlsVersion && (t.Hidden || t.Muted || t.Solo || t.Locked) {
			return fmt.Errorf("%w: track controls require version %d", ErrUnsupportedVersion, ControlsVersion)
		}
		tracks[t.ID] = true
		ordered := append([]Clip(nil), t.Clips...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
		var end int64
		for i, c := range ordered {
			a, ok := assets[c.AssetID]
			if !ok || clips[c.ID] {
				return fmt.Errorf("%w %q: missing asset or duplicate identity", ErrInvalidClip, c.ID)
			}
			if c.AudioDetached && p.Version < AudioVersion {
				return fmt.Errorf("%w: detached audio requires version %d", ErrUnsupportedVersion, AudioVersion)
			}
			if c.Opacity != nil && p.Version < LayerVersion {
				return fmt.Errorf("%w: opacity requires version %d", ErrUnsupportedVersion, LayerVersion)
			}
			if c.Gain != nil && p.Version < GainVersion {
				return fmt.Errorf("%w: audio gain requires version %d", ErrUnsupportedVersion, GainVersion)
			}
			if c.AudioMuted && p.Version < MuteVersion {
				return fmt.Errorf("%w: clip mute requires version %d", ErrUnsupportedVersion, MuteVersion)
			}
			if c.HasFades() && p.Version < FadeVersion {
				return fmt.Errorf("%w: audio fades require version %d", ErrUnsupportedVersion, FadeVersion)
			}
			if c.Envelope != nil && p.Version < EnvelopeVersion {
				return fmt.Errorf("%w: audio envelopes require version %d", ErrUnsupportedVersion, EnvelopeVersion)
			}
			if err := validateClip(c, a, t.Kind); err != nil {
				return err
			}
			if c.Start < end {
				return fmt.Errorf("%w: %w on track %q at clip %q", ErrInvalidClip, ErrOverlap, t.ID, c.ID)
			}
			if c.TransitionIn != nil {
				if p.Version < TransitionVersion {
					return fmt.Errorf("%w: video transitions require version %d", ErrUnsupportedVersion, TransitionVersion)
				}
				if t.Kind != VideoTrack || i == 0 {
					return fmt.Errorf("%w: transition requires an adjacent preceding video clip", ErrInvalidTransition)
				}
				previous := ordered[i-1]
				if err := validateVideoTransition(previous, c, assets[previous.AssetID]); err != nil {
					return err
				}
			}
			clips[c.ID] = true
			end = c.End()
		}
	}
	return nil
}

func (p Project) ValidateEditorLayout() error {
	return p.Validate()
}
