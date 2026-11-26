package composition

import (
	"context"
	"math"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/project"
)

func TestVideoSamplesIncludeOffGridCuts(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 300000, Width: 8, Height: 8, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "short", AssetID: "a", Start: 15000, Out: 30000}, {ID: "long", AssetID: "a", Start: 105000, Out: 100000}}
	p.Tracks[1].Clips = []project.Clip{{ID: "audio", AssetID: "a", Start: 70000, Out: 10000}}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for at := int64(0); ; at = e.NextVideoSample(0, at) {
		got = append(got, at)
		if at == e.Duration() {
			break
		}
		if len(got) > 10 {
			t.Fatal("scheduler failed to advance")
		}
	}
	want := []int64{0, 15000, 45000, 100000, 105000, 200000, 205000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("samples %v want %v", got, want)
	}
	if e.VideoSampleAt(0, 44000) != 15000 || e.VideoSampleAt(0, 90000) != 45000 || e.VideoSampleAt(0, 106000) != 105000 {
		t.Fatal("catch-up skipped a current cut")
	}
	if at, ok := e.EditBoundary(45000, true); !ok || at != 70000 {
		t.Fatal("audio-only edit missing from navigation")
	}
	if at, ok := e.EditBoundary(105000, false); !ok || at != 80000 {
		t.Fatal("previous edit boundary incorrect")
	}
	p.Tracks[0].Hidden = true
	e, err = Compile(p)
	if err != nil || e.NextVideoSample(0, 0) != 100000 {
		t.Fatal("hidden video forced an off-grid sample")
	}
}

func TestFrameValidityAndOverflow(t *testing.T) {
	f := CompositeFrame{Time: 15000, ValidUntil: 45000}
	if f.CurrentAt(14999) || !f.CurrentAt(15000) || !f.CurrentAt(44999) || f.CurrentAt(45000) {
		t.Fatal("frame crossed its half-open edit interval")
	}
	f = CompositeFrame{Time: 0, ValidUntil: 1000000}
	if !f.CurrentAt(200000) || f.CurrentAt(200001) {
		t.Fatal("lateness budget not enforced")
	}
	f = CompositeFrame{Time: 100, ValidUntil: 100, End: true}
	if f.CurrentAt(99) || !f.CurrentAt(100) || f.CurrentAt(101) {
		t.Fatal("invalid final frame validity")
	}
	e := &Evaluator{duration: math.MaxInt64}
	if e.NextVideoSample(0, math.MaxInt64-1) != math.MaxInt64 {
		t.Fatal("sample overflow")
	}
}

func TestClockedSequenceUsesOffGridBoundary(t *testing.T) {
	e := sequenceFixture(t)
	e.videoBoundaries = append([]int64{50000}, e.videoBoundaries...)
	d := &sequenceDecoder{}
	var now int64
	var got []int64
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := RenderClockedSequence(ctx, e, d, 0, func() int64 { return now }, func(_ context.Context, frame CompositeFrame) error {
		got = append(got, frame.Time)
		if !frame.CurrentAt(now) {
			t.Fatal("obsolete frame delivered")
		}
		switch len(got) {
		case 1:
			now = 50000
		case 2:
			now = 300000
		default:
			now = e.Duration()
		}
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, []int64{0, 50000, 300000, 550000}) {
		t.Fatalf("sequence %v: %v", got, err)
	}
}
