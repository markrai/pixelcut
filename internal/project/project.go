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
const Version = 6

type Asset struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Duration int64  `json:"duration_us"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	HasAudio bool   `json:"has_audio"`
	Range    string `json:"range,omitempty"`
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
	v, err := r.Video()
	if err != nil {
		return Asset{}, err
	}
	if err = v.ValidateHLG(); err != nil {
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
	if at <= c.Start || !c.Contains(at) {
		return p, errors.New("split must be strictly inside clip")
	}
	q := p.Clone()
	right := c.Clone()
	source, _ := c.SourceAt(at)
	right.ID, right.Start, right.In = rightID, at, source
	q.Tracks[i].Clips[j].Out = right.In
	q.Tracks[i].Clips = append(q.Tracks[i].Clips, right)
	if err := q.Validate(); err != nil {
		return p, err
	}
	return q, nil
}

func (p Project) MediaEdit(c Clip) (Asset, media.Edit, error) {
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
	if c.OpacityPercent() != 100 {
		return a, media.Edit{}, errors.New("layer opacity requires project composition; selected-clip export cannot apply it")
	}
	if !c.AudioDetached && c.GainPercent() != 100 && c.GainPercent() != 0 {
		return a, media.Edit{}, errors.New("clip audio gain requires whole-project export")
	}
	e := media.Edit{Start: Seconds(c.In), Duration: Seconds(c.Duration()), Trim: c.In != 0 || c.Out != a.Duration, Silent: c.AudioDetached || c.GainPercent() == 0}
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
