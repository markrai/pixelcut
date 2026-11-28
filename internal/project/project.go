package project

import (
	"errors"
	"path/filepath"

	"github.com/markrai/pixelcut/internal/media"
)

const LegacyVersion = 1
const TrackVersion = 2
const AudioVersion = 3
const ControlsVersion = 4
const LayerVersion = 5
const GainVersion = 6
const MuteVersion = 7
const FadeVersion = 8
const EnvelopeVersion = 9
const TransitionVersion = 10
const Version = 11

type Asset struct {
	ID            string              `json:"id"`
	Path          string              `json:"path"`
	Duration      int64               `json:"duration_us"`
	Width         int                 `json:"width"`
	Height        int                 `json:"height"`
	HasAudio      bool                `json:"has_audio"`
	Range         string              `json:"range,omitempty"`
	ColorFamily   media.ColorFamily   `json:"color_family,omitempty"`
	ColorDefaults media.ColorDefaults `json:"color_defaults,omitempty"`
	Codec         string              `json:"codec,omitempty"`
	PixelFormat   string              `json:"pixel_format,omitempty"`
	Chroma        string              `json:"chroma,omitempty"`
}

type Crop struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Clip struct {
	ID            string           `json:"id"`
	AssetID       string           `json:"asset_id"`
	Start         int64            `json:"start_us"`
	In            int64            `json:"in_us"`
	Out           int64            `json:"out_us"`
	Crop          *Crop            `json:"crop,omitempty"`
	Transform     *media.Transform `json:"transform,omitempty"`
	AudioDetached bool             `json:"audio_detached,omitempty"`
	Opacity       *int             `json:"opacity_percent,omitempty"`
	Gain          *int             `json:"audio_gain_percent,omitempty"`
	AudioMuted    bool             `json:"audio_muted,omitempty"`
	FadeIn        int64            `json:"audio_fade_in_us,omitempty"`
	FadeOut       int64            `json:"audio_fade_out_us,omitempty"`
	Envelope      *VolumeEnvelope  `json:"audio_envelope,omitempty"`
	TransitionIn  *VideoTransition `json:"transition_in,omitempty"`
}

type Track struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Clips  []Clip `json:"clips"`
	Hidden bool   `json:"hidden,omitempty"`
	Muted  bool   `json:"muted,omitempty"`
	Solo   bool   `json:"solo,omitempty"`
	Locked bool   `json:"locked,omitempty"`
}

type Project struct {
	Version      int     `json:"version"`
	Assets       []Asset `json:"assets"`
	Tracks       []Track `json:"tracks"`
	CanvasWidth  int     `json:"canvas_width,omitempty"`
	CanvasHeight int     `json:"canvas_height,omitempty"`
}

func New() Project {
	return Project{Version: Version, Assets: []Asset{}, Tracks: []Track{
		{ID: "V1", Kind: VideoTrack, Clips: []Clip{}},
		{ID: "A1", Kind: AudioTrack, Clips: []Clip{}},
	}}
}

func FromReport(id, path string, r media.Report) (Asset, error) {
	return assetFromReport(id, path, r, false)
}

func FromImportReport(id, path string, r media.Report) (Asset, error) {
	return assetFromReport(id, path, r, true)
}

func assetFromReport(id, path string, r media.Report, classify bool) (Asset, error) {
	v, err := r.Video()
	if err != nil {
		return Asset{}, err
	}
	var color media.SourceColor
	if classify {
		color, err = v.ClassifySourceColor()
	} else {
		err = v.ValidateHLG()
	}
	if err != nil {
		return Asset{}, err
	}
	v, err = v.DisplayGeometry()
	if err != nil {
		return Asset{}, err
	}
	d, err := ParseSeconds(v.Duration)
	if err != nil || d <= 0 {
		return Asset{}, errors.New("source needs a positive video duration")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Asset{}, err
	}
	a := Asset{ID: id, Path: path, Duration: d, Width: v.Width, Height: v.Height, Range: v.Range}
	if classify && color.Family != media.ColorHLG {
		a.ColorFamily, a.ColorDefaults = color.Family, color.Defaults
		a.Codec, a.PixelFormat, a.Range, a.Chroma = color.Codec, color.PixelFormat, color.Range, color.Chroma
		if err := r.ValidateClassifiedImportStreams(); err != nil {
			return Asset{}, err
		}
	}
	for _, s := range r.Streams {
		if s.CodecType == "audio" {
			a.HasAudio = true
		}
	}
	if err := validateAsset(a); err != nil {
		return Asset{}, err
	}
	return a, nil
}

func (p Project) Clone() Project {
	q := p
	q.Assets = append([]Asset{}, p.Assets...)
	q.Tracks = append([]Track{}, p.Tracks...)
	for i := range q.Tracks {
		q.Tracks[i].Clips = append([]Clip{}, p.Tracks[i].Clips...)
		for j := range q.Tracks[i].Clips {
			q.Tracks[i].Clips[j] = q.Tracks[i].Clips[j].Clone()
		}
	}
	return q
}

func (p Project) Split(track, clip, rightID string, at int64) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	i, j, ok := p.ClipIndex(ClipRef{TrackID: track, ClipID: clip})
	if !ok {
		return p, ErrClipNotFound
	}
	if err := p.RequireEditableTrack(track); err != nil {
		return p, err
	}
	c := p.Tracks[i].Clips[j]
	if p.ClipHasTransition(ClipRef{TrackID: track, ClipID: clip}) {
		return p, errors.New("remove the transition before splitting a participating clip")
	}
	if at <= c.Start || !c.Contains(at) {
		return p, errors.New("split must be strictly inside clip")
	}
	q := p.Clone()
	right := c.Clone()
	source, _ := c.SourceAt(at)
	right.ID, right.Start, right.In = rightID, at, source
	q.Tracks[i].Clips[j].Out = right.In
	q.Tracks[i].Clips[j].FadeOut = 0
	right.FadeIn = 0
	q.Tracks[i].Clips = append(q.Tracks[i].Clips, right)
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func (p Project) MediaEdit(c Clip) (Asset, media.Edit, error) {
	return p.MediaEditWithMute(c, false)
}

func (p Project) MediaEditWithMute(c Clip, muted bool) (Asset, media.Edit, error) {
	if err := p.Validate(); err != nil {
		return Asset{}, media.Edit{}, err
	}
	a, ok := p.Asset(c.AssetID)
	if !ok {
		return a, media.Edit{}, ErrAssetNotFound
	}
	if err := validateClip(c, a, VideoTrack); err != nil {
		return a, media.Edit{}, err
	}
	if ref, ok := p.LocateClip(c.ID); ok && p.ClipHasTransition(ref) {
		return a, media.Edit{}, errors.New("video transitions require whole-project rendering")
	}
	if c.OpacityPercent() != 100 {
		return a, media.Edit{}, errors.New("layer opacity requires project composition; selected-clip export cannot apply it")
	}
	silent := muted || c.AudioDetached || c.EffectiveGainPercent() == 0
	if !silent && c.HasEnvelope() {
		return a, media.Edit{}, errors.New("clip audio envelopes require whole-project export")
	}
	if !silent && c.HasFades() {
		return a, media.Edit{}, errors.New("clip audio fades require whole-project export")
	}
	if !silent && c.GainPercent() != 100 {
		return a, media.Edit{}, errors.New("clip audio gain requires whole-project export")
	}
	e := media.Edit{Start: Seconds(c.In), Duration: Seconds(c.Duration()), Trim: c.In != 0 || c.Out != a.Duration, Silent: silent}
	if c.Crop != nil {
		e.Crop = &media.Crop{X: c.Crop.X, Y: c.Crop.Y, Width: c.Crop.Width, Height: c.Crop.Height}
	}
	if c.Transform != nil {
		copy := *c.Transform
		e.Transform = &copy
	}
	return a, e, nil
}

func Load(path string) (Project, error) {
	p, _, err := LoadRevision(path)
	return p, err
}

func (p Project) SaveAs(path string) error {
	_, err := p.SaveNew(path)
	return err
}
