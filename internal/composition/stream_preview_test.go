package composition

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type sequenceDecoder struct {
	solidStillDecoder
	active [][]string
}

func (d *sequenceDecoder) Retain(ids []string) error {
	d.active = append(d.active, append([]string(nil), ids...))
	return nil
}

func sequenceFixture(t *testing.T) *Evaluator {
	t.Helper()
	p := project.New()
	path := filepath.Join(t.TempDir(), "base.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	p.Assets = []project.Asset{{ID: "a", Path: path, Duration: 600000, Width: 8, Height: 8}}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: "a", Start: 100000, Out: 200000}, {ID: "last", AssetID: "a", Start: 400000, In: 200000, Out: 350000}}
	var err error
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "overlay", AssetID: "a", Start: 200000, In: 100000, Out: 200000}}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRenderSequenceTracksGapsAndEnd(t *testing.T) {
	e := sequenceFixture(t)
	d := &sequenceDecoder{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var ticks []int64
	var ends []bool
	var colors []byte
	err := RenderSequence(ctx, e, d, 0, func(_ context.Context, f CompositeFrame) error {
		if !f.Frame.Valid() {
			t.Fatal("invalid frame delivered")
		}
		ticks = append(ticks, f.Time)
		ends = append(ends, f.End)
		colors = append(colors, f.Frame.Pixels[0])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ticks, []int64{0, 100000, 200000, 300000, 400000, 500000, 550000}) {
		t.Fatalf("incorrect timeline sampling: %v", ticks)
	}
	want := [][]string{nil, {"first"}, {"first", "overlay"}, nil, {"last"}, {"last"}, nil}
	if !reflect.DeepEqual(d.active, want) {
		t.Fatalf("incorrect active stream lifecycle: %v", d.active)
	}
	if !reflect.DeepEqual(colors, []byte{0, 200, 200, 0, 200, 200, 0}) || !ends[len(ends)-1] {
		t.Fatalf("unexpected gaps or final frame: %v %v", colors, ends)
	}
	for _, end := range ends[:len(ends)-1] {
		if end {
			t.Fatal("premature final frame")
		}
	}
	if len(d.calls) != 5 || d.calls[2].ClipID != "overlay" || d.calls[2].Tick != 100000 || d.calls[4].Tick != 300000 {
		t.Fatalf("incorrect source mapping: %+v", d.calls)
	}
}

func TestRenderSequenceCancellationAndFailure(t *testing.T) {
	e := sequenceFixture(t)
	d := &sequenceDecoder{}
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	err := RenderSequence(ctx, e, d, 0, func(_ context.Context, f CompositeFrame) error {
		n++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || n != 1 {
		t.Fatalf("canceled sequence delivered extra frames: %d %v", n, err)
	}
	failure := errors.New("decoder unavailable")
	d.err = failure
	err = RenderSequence(context.Background(), e, d, 100000, func(_ context.Context, f CompositeFrame) error {
		t.Fatal("failed frame delivered")
		return nil
	})
	if !errors.Is(err, failure) {
		t.Fatalf("decode failure lost: %v", err)
	}
	if err = RenderSequence(context.Background(), e, d, e.Duration(), func(context.Context, CompositeFrame) error { return nil }); err == nil {
		t.Fatal("playback accepted final endpoint")
	}
	if NextCompositeTick(math.MaxInt64-1, math.MaxInt64) != math.MaxInt64 || NextCompositeTick(0, 999999) != media.CompositeStep {
		t.Fatal("sample arithmetic overflow or wrong step")
	}
}
