package project

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestIntervalContract(t *testing.T) {
	c := Clip{Start: 10, In: 20, Out: 30}
	i, ok := c.TimelineInterval()
	if !ok || i != (Interval{Start: 10, End: 20}) || c.Duration() != 10 || c.End() != 20 {
		t.Fatal("incorrect interval")
	}
	for _, tc := range []struct {
		at     int64
		source int64
		ok     bool
	}{{9, 0, false}, {10, 20, true}, {19, 29, true}, {20, 0, false}} {
		source, ok := c.SourceAt(tc.at)
		if ok != tc.ok || source != tc.source || c.Contains(tc.at) != tc.ok {
			t.Fatalf("source mapping: %+v", tc)
		}
	}
	if i.Overlaps(Interval{Start: 20, End: 21}) || !i.Overlaps(Interval{Start: 19, End: 21}) {
		t.Fatal("half-open overlap contract")
	}
	for _, invalid := range []Clip{
		{Start: -1, In: 0, Out: 1},
		{In: -1, Out: math.MaxInt64},
		{In: 1, Out: 1},
		{Start: math.MaxInt64, Out: 1},
		{In: math.MaxInt64, Out: math.MinInt64},
	} {
		if _, ok := invalid.TimelineInterval(); ok || invalid.End() != 0 || invalid.Contains(0) {
			t.Fatalf("invalid interval accepted: %+v", invalid)
		}
	}
	last := Clip{Start: math.MaxInt64 - 1, Out: 1}
	if last.End() != math.MaxInt64 || !last.Contains(math.MaxInt64-1) || last.Contains(math.MaxInt64) {
		t.Fatal("maximum timeline boundary")
	}
	for _, s := range []string{"NaN", "+Inf", "-1", "1e300", "9223372036854.776"} {
		if _, err := ParseSeconds(s); err == nil {
			t.Fatalf("invalid seconds accepted: %s", s)
		}
	}
	if got, err := ParseSeconds("1.234567"); err != nil || got != 1234567 {
		t.Fatal("microsecond conversion")
	}
}

func TestStableReferencesAndCloneIsolation(t *testing.T) {
	p := fixture(t)
	p.Tracks[0].Clips[0].Transform = &media.Transform{CanvasWidth: 1080, CanvasHeight: 1920, Scale: 100}
	before := p.Clone()
	ref, ok := p.LocateClip("c")
	if !ok || ref != (ClipRef{TrackID: "V1", ClipID: "c"}) {
		t.Fatal("clip identity lookup")
	}
	c, ok := p.Clip(ref)
	if !ok {
		t.Fatal("clip reference not resolved")
	}
	c.Crop.X += 2
	c.Transform.X += 2
	c, err := p.DragClip("V1", "c", "move", 1000000)
	if err != nil {
		t.Fatal(err)
	}
	c.Crop.X += 2
	c.Transform.X += 2
	if !reflect.DeepEqual(p, before) {
		t.Fatal("candidate aliases committed state")
	}
	q, id, err := p.AddTrack(VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.TransferClips("V1", id, []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.Clip(ref); ok {
		t.Fatal("stale track reference resolved")
	}
	ref, ok = q.LocateClip("c")
	if !ok || ref.TrackID != id {
		t.Fatal("stable clip ID lost on transfer")
	}
	ti, ci, ok := q.ClipIndex(ref)
	if !ok || ti != q.TrackIndex(id) || ci != 0 || q.AssetIndex("a") != 0 {
		t.Fatal("index derivation failed")
	}
	q.Tracks[ti].Clips[ci].Crop.X += 2
	q.Tracks[ti].Clips[ci].Transform.X += 2
	if !reflect.DeepEqual(p, before) {
		t.Fatal("transfer aliases committed state")
	}
}

func TestValidationAcrossEntryPoints(t *testing.T) {
	for _, tc := range []struct {
		name   string
		want   error
		mutate func(*Project)
	}{
		{"version", ErrUnsupportedVersion, func(p *Project) { p.Version = 99 }},
		{"layout", ErrInvalidTrack, func(p *Project) { p.Tracks = p.Tracks[:1] }},
		{"legacy", ErrInvalidTrack, func(p *Project) {
			p.Version = LegacyVersion
			p.Tracks = append(p.Tracks, Track{ID: "V2", Kind: VideoTrack})
		}},
		{"range", ErrInvalidAsset, func(p *Project) { p.Assets[0].Range = "unknown" }},
		{"asset ID", ErrInvalidAsset, func(p *Project) { p.Assets = append(p.Assets, p.Assets[0]) }},
		{"overflow", ErrInvalidClip, func(p *Project) { p.Tracks[0].Clips[0].Start = math.MaxInt64 }},
		{"overlap", ErrOverlap, func(p *Project) {
			c := p.Tracks[0].Clips[0]
			c.ID = "second"
			p.Tracks[0].Clips = append(p.Tracks[0].Clips, c)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixture(t)
			tc.mutate(&p)
			before, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(p.Validate(), tc.want) || !errors.Is(p.ValidateEditorLayout(), tc.want) {
				t.Fatal("validation contract mismatch")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "invalid.pixelcut.json")
			if err := p.SaveAs(path); !errors.Is(err, tc.want) {
				t.Fatalf("save: %v", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("invalid project published")
			}
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); !errors.Is(err, tc.want) {
				t.Fatalf("load: %v", err)
			}
			if _, err := p.DeleteClips("V1", []string{"c"}); !errors.Is(err, tc.want) {
				t.Fatalf("delete: %v", err)
			}
			if _, err := p.SnapDrag("V1", "c", "move", 1, -1, 1); !errors.Is(err, tc.want) {
				t.Fatalf("snap: %v", err)
			}
			if _, err := p.PlaceAssets("V1", []string{"a"}, []string{"new"}, 20000000, -1, 0); !errors.Is(err, tc.want) {
				t.Fatalf("placement: %v", err)
			}
			if _, err := p.ImportPaths(context.Background(), []string{"source.mp4"}, func(context.Context, string) (Asset, error) {
				t.Fatal("invalid input reached inspection")
				return Asset{}, nil
			}); !errors.Is(err, tc.want) {
				t.Fatalf("import: %v", err)
			}
			after, err := json.Marshal(p)
			if err != nil || string(before) != string(after) {
				t.Fatal("failed edit mutated input")
			}
		})
	}
}

func TestLegacyVersionPreservedUntilRequired(t *testing.T) {
	p := fixture(t)
	p.Version = LegacyVersion
	path := filepath.Join(t.TempDir(), "legacy.pixelcut.json")
	if err := p.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	q, err := Load(path)
	if err != nil || !reflect.DeepEqual(p, q) || p.Fingerprint() != q.Fingerprint() {
		t.Fatal("legacy load migrated project")
	}
	q, err = q.Split("V1", "c", "right", 3000000)
	if err != nil || q.Version != LegacyVersion {
		t.Fatal("ordinary edit migrated legacy project")
	}
	r, err := q.PlaceAssets("A1", []string{"a"}, []string{"audio"}, 0, -1, 0)
	if err != nil || r.Project.Version != Version || q.Version != LegacyVersion {
		t.Fatal("independent audio did not explicitly upgrade")
	}
}
