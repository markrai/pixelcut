package composition

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type fakePCM struct {
	calls []media.PCMRequest
	err   error
	short bool
}

func (d *fakePCM) Retain(ids []string) error {
	if len(ids) > media.MaxAudioStreams {
		return errors.New("too many streams")
	}
	return nil
}

func (d *fakePCM) DecodePCM(_ context.Context, req media.PCMRequest) ([]int16, error) {
	d.calls = append(d.calls, req)
	if d.err != nil {
		return nil, d.err
	}
	n := req.Frames * media.PCMChannels
	if d.short {
		n--
	}
	samples := make([]int16, n)
	for i := range samples {
		samples[i] = 20000
	}
	return samples, nil
}

func TestMixPCMBoundariesPoliciesAndSaturation(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100000, Width: 8, Height: 8, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "video", AssetID: "a", Start: 10001, In: 2000, Out: 22000}}
	p.Tracks[1].Clips = []project.Clip{{ID: "audio", AssetID: "a", Start: 20000, In: 1000, Out: 31000}}
	p.Tracks[0].Hidden = true
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	d := &fakePCM{}
	block, err := MixPCM(context.Background(), e, d, 0, 0, 2400)
	if err != nil || len(block.Samples) != 4800 {
		t.Fatalf("mix: %v", err)
	}
	for _, tc := range []struct {
		frame int
		want  int16
	}{{0, 0}, {480, 0}, {481, 20000}, {959, 20000}, {960, 32767}, {1440, 32767}, {1441, 20000}, {2399, 20000}} {
		if block.Samples[tc.frame*2] != tc.want || block.Samples[tc.frame*2+1] != tc.want {
			t.Fatalf("frame %d: %d, want %d", tc.frame, block.Samples[tc.frame*2], tc.want)
		}
	}
	if block.Levels.Frames != 2400 || block.Levels.Peak != [2]uint32{32767, 32767} || block.Levels.Clipped != [2]int64{481, 481} {
		t.Fatalf("incorrect mixed block levels: %+v", block.Levels)
	}
	if len(d.calls) != 4 || d.calls[0].Offset != 481 || d.calls[0].Tick != 2019 || d.calls[2].ClipID != "audio" {
		t.Fatalf("wrong source mapping: %+v", d.calls)
	}
	p.Tracks[0].Clips[0].AudioDetached = true
	p.Tracks[1].Muted = true
	e, err = Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	d.calls = nil
	block, err = MixPCM(context.Background(), e, d, 0, 0, 2400)
	if err != nil || len(d.calls) != 0 {
		t.Fatalf("suppressed audio decoded: %v", err)
	}
	if block.Levels != (media.AudioLevels{Frames: 2400}) {
		t.Fatal("silence reported nonzero peaks or clipping")
	}
	for _, sample := range block.Samples {
		if sample != 0 {
			t.Fatal("suppressed audio produced sound")
		}
	}
}

func TestMixPCMFailureAndCancellation(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100000, Width: 8, Height: 8, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "video", AssetID: "a", Out: 100000}}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("missing audio")
	d := &fakePCM{err: failure}
	if b, err := MixPCM(context.Background(), e, d, 0, 0, 2400); !errors.Is(err, failure) || len(b.Samples) != 0 {
		t.Fatal("partial failed mix published")
	}
	d.err, d.short = nil, true
	if _, err := MixPCM(context.Background(), e, d, 0, 0, 2400); err == nil {
		t.Fatal("short block accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MixPCM(ctx, e, d, 0, 0, 2400); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestClockedSequenceSkipsObsoleteVideo(t *testing.T) {
	e := sequenceFixture(t)
	d := &sequenceDecoder{}
	var now int64
	var ticks []int64
	err := RenderClockedSequence(context.Background(), e, d, 0, func() int64 { return now }, func(_ context.Context, f CompositeFrame) error {
		ticks = append(ticks, f.Time)
		if f.Time > now {
			t.Fatal("video presented ahead of clock")
		}
		if len(ticks) == 1 {
			now = 400000
		} else {
			now = e.Duration()
		}
		return nil
	})
	if err != nil || len(ticks) != 3 || ticks[0] != 0 || ticks[1] != 400000 || ticks[2] != e.Duration() {
		t.Fatalf("clocked sequence: %v %v", ticks, err)
	}
}
