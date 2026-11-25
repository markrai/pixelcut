package project

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/markrai/pixelcut/internal/media"
)

const Version = 1
const TicksPerSecond int64 = 1000000

type Asset struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Duration int64  `json:"duration_us"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	HasAudio bool   `json:"has_audio"`
	Range string `json:"range,omitempty"`
}

type Crop struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Clip struct {
	ID       string `json:"id"`
	AssetID  string `json:"asset_id"`
	Start    int64  `json:"start_us"`
	In       int64  `json:"in_us"`
	Out      int64  `json:"out_us"`
	Crop     *Crop  `json:"crop,omitempty"`
	Transform *media.Transform `json:"transform,omitempty"`
}

type Track struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Clips []Clip `json:"clips"`
}

type Project struct {
	Version int     `json:"version"`
	Assets  []Asset `json:"assets"`
	Tracks  []Track `json:"tracks"`
}

func New() Project {
	return Project{Version: Version, Assets: []Asset{}, Tracks: []Track{
		{ID: "V1", Kind: "video", Clips: []Clip{}},
		{ID: "A1", Kind: "audio", Clips: []Clip{}},
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
	a := Asset{ID: id, Path: path, Duration: d, Width: v.Width, Height: v.Height, Range:v.Range}
	for _, s := range r.Streams {
		if s.CodecType == "audio" {
			a.HasAudio = true
		}
	}
	return a, nil
}

func ParseSeconds(s string) (int64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= float64(math.MaxInt64)/float64(TicksPerSecond) {
		return 0, errors.New("invalid nonnegative seconds")
	}
	return int64(math.Round(n * float64(TicksPerSecond))), nil
}

func Seconds(t int64) float64 { return float64(t) / float64(TicksPerSecond) }

func (p Project) Asset(id string) (Asset, bool) {
	for _, a := range p.Assets {
		if a.ID == id {
			return a, true
		}
	}
	return Asset{}, false
}

func (p Project) Validate() error {
	if p.Version != Version {
		return fmt.Errorf("unsupported project version %d", p.Version)
	}
	assets := map[string]Asset{}
	for _, a := range p.Assets {
		if a.ID == "" || assets[a.ID].ID != "" || !filepath.IsAbs(a.Path) || a.Duration <= 0 || a.Width <= 0 || a.Height <= 0 || a.Width%2 != 0 || a.Height%2 != 0 {
			return fmt.Errorf("invalid or duplicate asset %q", a.ID)
		}
		assets[a.ID] = a
	}
	tracks, clips := map[string]bool{}, map[string]bool{}
	for _, t := range p.Tracks {
		if t.ID == "" || tracks[t.ID] || (t.Kind != "video" && t.Kind != "audio") {
			return fmt.Errorf("invalid or duplicate track %q", t.ID)
		}
		tracks[t.ID] = true
		ordered := append([]Clip(nil), t.Clips...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
		var end int64
		for _, c := range ordered {
			a, ok := assets[c.AssetID]
			if c.ID == "" || clips[c.ID] || !ok || c.In < 0 || c.Out <= c.In || c.Out > a.Duration || c.Start < end || c.Start > math.MaxInt64-(c.Out-c.In) {
				return fmt.Errorf("invalid, overlapping or duplicate clip %q", c.ID)
			}
			if t.Kind == "audio" && (!a.HasAudio || c.Crop != nil || c.Transform != nil) {
				return fmt.Errorf("invalid audio clip %q", c.ID)
			}
			if c.Crop != nil {
				v := media.Stream{Width: a.Width, Height: a.Height}
				if err := (media.Crop{X: c.Crop.X, Y: c.Crop.Y, Width: c.Crop.Width, Height: c.Crop.Height}).Validate(v); err != nil {
					return fmt.Errorf("clip %q: %w", c.ID, err)
				}
			}
			if c.Transform!=nil {
				if a.Range!="" && a.Range!="tv" {return fmt.Errorf("clip %q: canvas transforms require limited-range HLG",c.ID)}
				w,h:=a.Width,a.Height
				if c.Crop!=nil {w,h=c.Crop.Width,c.Crop.Height}
				if _,err:=c.Transform.Placement(w,h);err!=nil {return fmt.Errorf("clip %q: %w",c.ID,err)}
			}
			clips[c.ID] = true
			end = c.Start + (c.Out - c.In)
		}
	}
	return nil
}

func (p Project) Clone() Project {
	q := p
	q.Assets = append([]Asset{}, p.Assets...)
	q.Tracks = append([]Track{}, p.Tracks...)
	for i := range q.Tracks {
		q.Tracks[i].Clips = append([]Clip{}, p.Tracks[i].Clips...)
		for j := range q.Tracks[i].Clips {
			if tr:=q.Tracks[i].Clips[j].Transform;tr!=nil {
				copy:=*tr
				q.Tracks[i].Clips[j].Transform=&copy
			}
			if c := q.Tracks[i].Clips[j].Crop; c != nil {
				copy := *c
				q.Tracks[i].Clips[j].Crop = &copy
			}
		}
	}
	return q
}

func (p Project) Split(track, clip, rightID string, at int64) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	q := p.Clone()
	for i := range q.Tracks {
		if q.Tracks[i].ID != track {
			continue
		}
		for j, c := range q.Tracks[i].Clips {
			if c.ID != clip {
				continue
			}
			if at <= c.Start || at >= c.Start+(c.Out-c.In) {
				return p, errors.New("split must be strictly inside clip")
			}
			right := c
			right.ID, right.Start, right.In = rightID, at, c.In+(at-c.Start)
			if right.Transform!=nil {copy:=*right.Transform;right.Transform=&copy}
			if right.Crop != nil {
				copy := *right.Crop
				right.Crop = &copy
			}
			q.Tracks[i].Clips[j].Out = right.In
			q.Tracks[i].Clips = append(q.Tracks[i].Clips, right)
			if err := q.Validate(); err != nil {
				return p, err
			}
			return q, nil
		}
	}
	return p, errors.New("clip not found")
}

func (p Project) MediaEdit(c Clip) (Asset, media.Edit, error) {
	if err := p.Validate(); err != nil {
		return Asset{}, media.Edit{}, err
	}
	a, ok := p.Asset(c.AssetID)
	if !ok {
		return a, media.Edit{}, errors.New("asset not found")
	}
	if c.In < 0 || c.Out <= c.In || c.Out > a.Duration {
		return a, media.Edit{}, errors.New("invalid source interval")
	}
	e := media.Edit{Start: Seconds(c.In), Duration: Seconds(c.Out-c.In), Trim: c.In != 0 || c.Out != a.Duration}
	if c.Crop != nil {
		e.Crop = &media.Crop{X: c.Crop.X, Y: c.Crop.Y, Width: c.Crop.Width, Height: c.Crop.Height}
		if err := e.Crop.Validate(media.Stream{Width: a.Width, Height: a.Height}); err != nil {
			return a, media.Edit{}, err
		}
	}
	if c.Transform!=nil {
		copy:=*c.Transform
		e.Transform=&copy
	}
	return a, e, nil
}

func Load(path string) (Project,error) {
	p,_,err:=LoadRevision(path)
	return p,err
}

func (p Project) SaveAs(path string) error {
	_,err:=p.SaveNew(path)
	return err
}
