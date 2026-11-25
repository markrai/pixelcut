package project

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

type baselineSize struct {
	name                  string
	tracks, assets, clips int
}

var baselineSizes = []baselineSize{
	{"small", 2, 4, 12},
	{"medium", 6, 30, 240},
	{"large", 12, 100, 2400},
}

func baselineProject(tb testing.TB, size baselineSize) Project {
	tb.Helper()
	root, err := filepath.Abs("testdata")
	if err != nil {
		tb.Fatal(err)
	}
	p := New()
	for i := 0; i < size.assets; i++ {
		p.Assets = append(p.Assets, Asset{ID: fmt.Sprintf("asset-%03d", i), Path: filepath.Join(root, fmt.Sprintf("source-%03d.mp4", i)), Duration: 10 * TicksPerSecond, Width: 1080, Height: 1920, Range: "tv", HasAudio: true})
	}
	for i := 2; i < size.tracks; i++ {
		kind, prefix := "video", "V"
		if i%2 != 0 {
			kind, prefix = "audio", "A"
		}
		p.Tracks = append(p.Tracks, Track{ID: fmt.Sprintf("%s%d", prefix, i/2+1), Kind: kind, Clips: []Clip{}})
	}
	for i := range p.Tracks {
		track := &p.Tracks[i]
		for j := 0; j < size.clips/size.tracks; j++ {
			c := Clip{ID: fmt.Sprintf("clip-%02d-%04d", i, j), AssetID: p.Assets[(i+j)%len(p.Assets)].ID, Start: int64(j) * 2 * TicksPerSecond, In: TicksPerSecond, Out: 2 * TicksPerSecond}
			if track.Kind == "video" && j%3 == 0 {
				c.Crop = &Crop{X: 20, Y: 20, Width: 1040, Height: 1880}
				c.Transform = &media.Transform{CanvasWidth: 1080, CanvasHeight: 1920, Scale: 100}
			}
			track.Clips = append(track.Clips, c)
		}
	}
	if err = p.ValidateEditorLayout(); err != nil {
		tb.Fatal(err)
	}
	return p
}

func TestBaselineFixtures(t *testing.T) {
	for _, size := range baselineSizes {
		t.Run(size.name, func(t *testing.T) {
			p := baselineProject(t, size)
			clips := 0
			for _, track := range p.Tracks {
				clips += len(track.Clips)
			}
			if len(p.Tracks) != size.tracks || len(p.Assets) != size.assets || clips != size.clips {
				t.Fatal("fixture count mismatch")
			}
			if p.EndTime() != int64(2*(size.clips/size.tracks)-1)*TicksPerSecond {
				t.Fatal("fixture timeline mismatch")
			}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var q Project
			if err = json.Unmarshal(data, &q); err != nil {
				t.Fatal(err)
			}
			if err = q.ValidateEditorLayout(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(q, p) {
				t.Fatal("fixture roundtrip mismatch")
			}
			clone := p.Clone()
			clone.Tracks[0].Clips[0].Crop.X += 2
			clone.Tracks[0].Clips[0].Transform.X += 2
			if p.Tracks[0].Clips[0].Crop.X != 20 || p.Tracks[0].Clips[0].Transform.X != 0 {
				t.Fatal("fixture clone aliases nested properties")
			}
		})
	}
}

func BenchmarkProjectBaseline(b *testing.B) {
	for _, size := range baselineSizes {
		b.Run(size.name, func(b *testing.B) {
			p := baselineProject(b, size)
			track := p.Tracks[0]
			first := track.Clips[0]
			ids := []string{first.ID, track.Clips[1].ID}
			data, err := json.Marshal(p)
			if err != nil {
				b.Fatal(err)
			}
			cases := []struct {
				name string
				run  func() error
			}{
				{"validate", p.Validate},
				{"clone", func() error { _ = p.Clone(); return nil }},
				{"snap", func() error { _, err := p.SnapDrag(track.ID, first.ID, "move", 990000, -1, 20000); return err }},
				{"group_move", func() error { _, err := p.MoveClips(track.ID, ids, 100000, -1, 20000); return err }},
				{"split", func() error { _, err := p.Split(track.ID, first.ID, "baseline-right", 500000); return err }},
				{"encode", func() error { _, err := json.Marshal(p); return err }},
				{"decode_validate", func() error {
					var q Project
					if err := json.Unmarshal(data, &q); err != nil {
						return err
					}
					return q.Validate()
				}},
			}
			for _, tc := range cases {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					if err := tc.run(); err != nil {
						b.Fatal(err)
					}
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := tc.run(); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
