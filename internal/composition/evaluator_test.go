package composition

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func fixture(t testing.TB) project.Project {
	t.Helper()
	dir := t.TempDir()
	p := project.New()
	p.Assets = []project.Asset{
		{ID: "a", Path: filepath.Join(dir, "offline-a.mp4"), Duration: 1000, Width: 1080, Height: 1920, HasAudio: true, Range: "tv"},
		{ID: "b", Path: filepath.Join(dir, "offline-b.mp4"), Duration: 1000, Width: 1080, Height: 1920, Range: "tv"},
	}
	p.Tracks[0].Clips = []project.Clip{
		{ID: "later", AssetID: "b", Start: 30, In: 200, Out: 210},
		{ID: "early", AssetID: "a", Start: 10, In: 100, Out: 120},
	}
	p.Tracks[1].Clips = []project.Clip{{ID: "sound", AssetID: "a", Start: 12, In: 300, Out: 310}}
	p.Tracks = append(p.Tracks,
		project.Track{ID: "V3", Kind: project.VideoTrack, Clips: []project.Clip{
			{ID: "upper", AssetID: "b", Start: 20, In: 400, Out: 415, Transform: &media.Transform{CanvasWidth: 1080, CanvasHeight: 1920, Scale: 100}},
		}},
		project.Track{ID: "V2", Kind: project.VideoTrack, Clips: []project.Clip{
			{ID: "overlay", AssetID: "a", Start: 15, In: 500, Out: 510, Crop: &project.Crop{X: 20, Y: 20, Width: 1040, Height: 1880}},
		}},
		project.Track{ID: "A2", Kind: project.AudioTrack, Clips: []project.Clip{
			{ID: "tail", AssetID: "a", Start: 45, In: 600, Out: 610},
		}},
	)
	return p
}

func TestCompositionContributions(t *testing.T) {
	p := fixture(t)
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if e.Duration() != 55 {
		t.Fatal("incorrect duration")
	}
	f, err := e.At(20)
	if err != nil || f.Time != 20 || f.End != 55 || f.NextBoundary != 22 || len(f.Video) != 3 || len(f.Audio) != 3 {
		t.Fatalf("incorrect contributions: %+v %v", f, err)
	}
	for i, want := range []struct {
		track, clip string
		source      int64
	}{{"V1", "early", 110}, {"V3", "upper", 400}, {"V2", "overlay", 505}} {
		got := f.Video[i]
		if got.Ref != (project.ClipRef{TrackID: want.track, ClipID: want.clip}) || got.SourceTime != want.source {
			t.Fatalf("video layer %d: %+v", i, got)
		}
	}
	if f.Video[1].Transform == nil || f.Video[2].Crop == nil || f.Video[2].Asset.Range != "tv" {
		t.Fatal("missing source geometry")
	}
	for i, want := range []struct {
		track  string
		source int64
		linked bool
	}{{"V1", 110, true}, {"A1", 308, false}, {"V2", 505, true}} {
		got := f.Audio[i]
		if got.Ref.TrackID != want.track || got.SourceTime != want.source || got.Linked != want.linked || got.Asset.ID != "a" {
			t.Fatalf("audio contribution %d: %+v", i, got)
		}
	}
	if f.Video[0].Timeline != (project.Interval{Start: 10, End: 30}) || f.Video[0].SourceInterval != (project.Interval{Start: 100, End: 120}) {
		t.Fatal("incorrect source interval")
	}
}

func TestBoundariesAndGaps(t *testing.T) {
	e, err := Compile(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at, next     int64
		video, audio int
	}{
		{0, 10, 0, 0}, {9, 10, 0, 0}, {10, 12, 1, 1}, {12, 15, 1, 2},
		{15, 20, 2, 3}, {20, 22, 3, 3}, {22, 25, 3, 2}, {25, 30, 2, 1},
		{29, 30, 2, 1}, {30, 35, 2, 0}, {35, 40, 1, 0}, {40, 45, 0, 0},
		{45, 55, 0, 1}, {54, 55, 0, 1}, {55, 55, 0, 0},
	} {
		f, err := e.At(tc.at)
		if err != nil || f.NextBoundary != tc.next || len(f.Video) != tc.video || len(f.Audio) != tc.audio {
			t.Fatalf("at %d: %+v %v", tc.at, f, err)
		}
	}
	f, _ := e.At(30)
	if f.Video[0].Ref.ClipID != "later" || f.Video[0].SourceTime != 200 {
		t.Fatal("adjacent cut used the expired clip")
	}
	for _, at := range []int64{-1, 56, math.MaxInt64} {
		if _, err := e.At(at); !errors.Is(err, ErrOutsideTimeline) {
			t.Fatalf("accepted out of range time %d", at)
		}
	}
}

func TestSnapshotIsolationAndDeterminism(t *testing.T) {
	p := fixture(t)
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	want, err := e.At(20)
	if err != nil {
		t.Fatal(err)
	}
	p.Assets[0].Path = "changed"
	p.Tracks[2].Clips[0].Transform.X = 40
	p.Tracks[3].Clips[0].Crop.X = 40
	for i := 0; i < 8; i++ {
		t.Run("reader", func(t *testing.T) {
			t.Parallel()
			f, err := e.At(20)
			if err != nil || !reflect.DeepEqual(f, want) {
				t.Fatal("snapshot changed")
			}
			f.Video[1].Transform.X = 80
			f.Video[2].Crop.X = 80
			f.Video[0].Asset.Path = "mutated result"
			f.Audio[0].Asset.ID = "mutated result"
		})
	}
}

func TestCompileValidationAndLegacy(t *testing.T) {
	p := fixture(t)
	p.Tracks[0].Clips[0].Start = 20
	if _, err := Compile(p); !errors.Is(err, project.ErrOverlap) {
		t.Fatalf("overlap: %v", err)
	}
	p = fixture(t)
	p.Tracks = p.Tracks[:2]
	p.Tracks[1].Clips = nil
	p.Version = project.LegacyVersion
	p.Assets[0].Range = ""
	before := p.Fingerprint()
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.At(10)
	if err != nil || len(f.Video) != 1 || len(f.Audio) != 1 || f.Video[0].Asset.Range != "" || p.Fingerprint() != before {
		t.Fatal("legacy composition changed source contract")
	}
	e, err = Compile(project.New())
	if err != nil {
		t.Fatal(err)
	}
	f, err = e.At(0)
	if err != nil || f.End != 0 || f.NextBoundary != 0 || len(f.Video) != 0 || len(f.Audio) != 0 {
		t.Fatal("empty project")
	}
}

func TestMaximumTimelineBoundary(t *testing.T) {
	p := fixture(t)
	p.Tracks = p.Tracks[:2]
	p.Tracks[1].Clips = nil
	p.Tracks[0].Clips = []project.Clip{{ID: "last", AssetID: "a", Start: math.MaxInt64 - 2, In: 1, Out: 3}}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.At(math.MaxInt64 - 1)
	if err != nil || f.NextBoundary != math.MaxInt64 || len(f.Video) != 1 || f.Video[0].SourceTime != 2 {
		t.Fatal("maximum timeline boundary")
	}
	f, err = e.At(math.MaxInt64)
	if err != nil || len(f.Video) != 0 || len(f.Audio) != 0 {
		t.Fatal("maximum exclusive end")
	}
}

func TestStorageOrderWithinTrackDoesNotChangeEvaluation(t *testing.T) {
	p := fixture(t)
	first, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Assets[0], p.Assets[1] = p.Assets[1], p.Assets[0]
	p.Tracks[0].Clips[0], p.Tracks[0].Clips[1] = p.Tracks[0].Clips[1], p.Tracks[0].Clips[0]
	second, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	for at := int64(0); at <= first.Duration(); at++ {
		a, errA := first.At(at)
		b, errB := second.At(at)
		if errA != nil || errB != nil || !reflect.DeepEqual(a, b) {
			t.Fatalf("storage order changed evaluation at %d", at)
		}
	}
}

func BenchmarkComposition(b *testing.B) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(b.TempDir(), "a.mp4"), Duration: 10000000, Width: 1080, Height: 1920, HasAudio: true}}
	for i := 2; i < 12; i++ {
		kind, prefix := project.VideoTrack, "V"
		if i%2 != 0 {
			kind, prefix = project.AudioTrack, "A"
		}
		p.Tracks = append(p.Tracks, project.Track{ID: fmt.Sprintf("%s%d", prefix, i/2+1), Kind: kind})
	}
	for i := range p.Tracks {
		for j := 0; j < 200; j++ {
			p.Tracks[i].Clips = append(p.Tracks[i].Clips, project.Clip{ID: fmt.Sprintf("clip-%d-%d", i, j), AssetID: "a", Start: int64(j) * 2000000, Out: 1000000})
		}
	}
	e, err := Compile(p)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("compile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := Compile(p); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("evaluate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			at := int64(i%200)*2000000 + 500000
			f, err := e.At(at)
			if err != nil || len(f.Video) != 6 || len(f.Audio) != 12 {
				b.Fatal("unexpected composition")
			}
		}
	})
}
