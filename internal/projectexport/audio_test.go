package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type testAudio struct {
	next  map[string]int64
	opens map[string]int
}

func (d *testAudio) Retain(ids map[string]bool) error {
	for id := range d.next {
		if !ids[id] {
			delete(d.next, id)
		}
	}
	return nil
}

func (d *testAudio) Read(a composition.AudioContribution, pos int64, frames int) ([]int16, error) {
	id := a.Ref.ClipID
	next, ok := d.next[id]
	if !ok {
		first, _, _, err := audioInterval(a)
		if err != nil {
			return nil, err
		}
		next = first
		d.opens[id]++
	}
	if next != pos {
		return nil, errors.New("audio stream discontinuity")
	}
	d.next[id] = pos + int64(frames)
	samples := make([]int16, frames*2)
	value := int16(20000)
	if id == "audio" {
		value = -10000
	}
	for i := range samples {
		samples[i] = value
	}
	return samples, nil
}

func TestOfflineSampleIntervals(t *testing.T) {
	for _, tc := range []struct {
		start, in, duration, first, end, source int64
	}{
		{0, 0, 100000, 0, 4800, 0},
		{10001, 20000, 100000, 481, 5281, 960},
		{10001, 0, 100000, 481, 5281, 0},
		{1, 1, 1, 1, 1, 1},
		{100000, 200001, 100001, 4800, 9601, 9600},
	} {
		a := composition.AudioContribution{Contribution: composition.Contribution{Timeline: project.Interval{Start: tc.start, End: tc.start + tc.duration}, SourceInterval: project.Interval{Start: tc.in, End: tc.in + tc.duration}}}
		first, end, source, err := audioInterval(a)
		if err != nil || first != tc.first || end != tc.end || source != tc.source {
			t.Fatalf("interval %+v: %d %d %d %v", tc, first, end, source, err)
		}
	}
}

func TestOfflineMixExactCutsAndSilence(t *testing.T) {
	plan, err := Build(exportFixture(t), "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	d := &testAudio{next: make(map[string]int64), opens: make(map[string]int)}
	if err := mixAudio(context.Background(), plan, d, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != int(plan.OutputAudioFrames)*4 || len(d.next) != 0 {
		t.Fatalf("incorrect PCM size or retirement: %d", out.Len())
	}
	for _, id := range []string{"base", "top", "audio"} {
		if d.opens[id] != 1 {
			t.Fatalf("clip %s opened %d times", id, d.opens[id])
		}
	}
	for _, tc := range []struct {
		at   int
		want int16
	}{{0, 0}, {480, 0}, {481, 20000}, {2399, 20000}, {2400, 32767}, {3359, 32767}, {3360, 20000}, {5280, 20000}, {5281, 0}, {7199, 0}, {7200, -10000}, {8160, -10000}, {8161, 0}, {9599, 0}} {
		for ch := 0; ch < 2; ch++ {
			got := int16(binary.LittleEndian.Uint16(out.Bytes()[4*tc.at+2*ch:]))
			if got != tc.want {
				t.Fatalf("frame %d channel %d: %d want %d", tc.at, ch, got, tc.want)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mixAudio(ctx, plan, d, &out); err == nil {
		t.Fatal("cancelled mix accepted")
	}
	plan.Spans[1].Audio = make([]composition.AudioContribution, media.MaxAudioStreams+1)
	if err := validateAudio(plan); err == nil {
		t.Fatal("excess simultaneous audio accepted")
	}
}

func TestProjectOutputRequiresExactAudioPresentationInterval(t *testing.T) {
	plan, err := Build(exportFixture(t), "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	v := plan.Contract.ExpectedVideo()
	zero, count := int64(0), plan.OutputAudioFrames
	v.Profile, v.FrameCount, v.Duration, v.AvgFrameRate, v.StartPTS = "Main 10", "6", "0.200000", "30/1", &zero
	a := media.Stream{CodecType: "audio", CodecName: "aac", SampleRate: "48000", Channels: 2, TimeBase: "1/48000", StartPTS: &zero, DurationTS: &count}
	r := media.Report{Streams: []media.Stream{v, a}}
	if err := verifyProjectOutput(plan, r); err != nil {
		t.Fatal(err)
	}
	count--
	if err := verifyProjectOutput(plan, r); err == nil {
		t.Fatal("one-sample-short audio accepted")
	}
	count++
	r.Streams[1].StartPTS = nil
	if err := verifyProjectOutput(plan, r); err == nil {
		t.Fatal("missing audio start accepted")
	}
	r.Streams[1] = a
	r.Streams[0].StartPTS = nil
	if err := verifyProjectOutput(plan, r); err == nil {
		t.Fatal("missing video start accepted")
	}
}

func TestOfflineLevelsMatchPreviewAndCountPadding(t *testing.T) {
	p := exportFixture(t)
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var measured media.AudioLevels
	d := &testAudio{next: make(map[string]int64), opens: make(map[string]int)}
	err = mixAudioObserved(context.Background(), plan, d, &out, func(v media.AudioLevels) {
		if v.Frames <= measured.Frames || v.Frames*4 != int64(out.Len()) {
			t.Fatal("meter advanced without a complete PCM write")
		}
		measured = v
	})
	if err != nil {
		t.Fatal(err)
	}
	if measured.Frames != plan.OutputAudioFrames || measured.Peak != [2]uint32{32767, 32767} || measured.Clipped != [2]int64{960, 960} {
		t.Fatalf("incorrect export levels: %+v", measured)
	}
	e, err := composition.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	var preview media.AudioLevels
	decoder := parityPCM{reads: make(map[string]int)}
	for pos := int64(0); pos < plan.ContentAudioFrames; {
		n := int(min(int64(1379), plan.ContentAudioFrames-pos))
		b, err := composition.MixPCM(context.Background(), e, decoder, 0, pos, n)
		if err != nil {
			t.Fatal(err)
		}
		preview.Merge(b.Levels)
		pos += int64(n)
	}
	preview.Frames += plan.OutputAudioFrames - plan.ContentAudioFrames
	if preview != measured {
		t.Fatalf("preview/export levels differ: %+v != %+v", preview, measured)
	}
	var reports int
	d = &testAudio{next: make(map[string]int64), opens: make(map[string]int)}
	if err := mixAudioObserved(context.Background(), plan, d, rejectPCMWrite{}, func(media.AudioLevels) { reports++ }); err == nil || reports != 0 {
		t.Fatal("failed PCM write reported levels")
	}
}

type rejectPCMWrite struct{}

func (rejectPCMWrite) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
