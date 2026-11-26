package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type parityPCM struct {
	reads map[string]int
}

func (d parityPCM) Retain(_ []string) error { return nil }

func (d parityPCM) DecodePCM(_ context.Context, req media.PCMRequest) ([]int16, error) {
	d.reads[req.ClipID]++
	v := int16(20000)
	if req.ClipID == "audio" {
		v = -10000
	}
	out := make([]int16, req.Frames*media.PCMChannels)
	for i := range out {
		out[i] = v
	}
	return out, nil
}

func TestPreviewAndOfflineGainParity(t *testing.T) {
	p := exportFixture(t)
	for _, tc := range []struct {
		ref  project.ClipRef
		gain int
	}{
		{project.ClipRef{TrackID: "V1", ClipID: "base"}, 75},
		{project.ClipRef{TrackID: "V2", ClipID: "top"}, 25},
		{project.ClipRef{TrackID: "A1", ClipID: "audio"}, 0},
	} {
		var err error
		p, err = p.SetClipGain(tc.ref, tc.gain)
		if err != nil {
			t.Fatal(err)
		}
	}
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var offline bytes.Buffer
	d := &testAudio{next: make(map[string]int64), opens: make(map[string]int)}
	if err := mixAudio(ctx, plan, d, &offline); err != nil {
		t.Fatal(err)
	}
	e, err := composition.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	preview := parityPCM{reads: make(map[string]int)}
	for pos := int64(0); pos < plan.ContentAudioFrames; {
		n := int(min(int64(media.PCMBlockFrames), plan.ContentAudioFrames-pos))
		block, err := composition.MixPCM(ctx, e, preview, 0, pos, n)
		if err != nil {
			t.Fatal(err)
		}
		for i, want := range block.Samples {
			got := int16(binary.LittleEndian.Uint16(offline.Bytes()[pos*4+int64(i*2):]))
			if got != want {
				t.Fatalf("preview/export mismatch at sample %d: %d != %d", pos*2+int64(i), want, got)
			}
		}
		pos += int64(n)
	}
	if d.opens["audio"] != 0 || preview.reads["audio"] != 0 {
		t.Fatal("zero-gain clip decoded")
	}
	for _, tc := range []struct {
		at   int
		want int16
	}{{480, 0}, {481, 15000}, {2400, 20000}, {3360, 15000}, {5281, 0}, {7200, 0}} {
		if got := int16(binary.LittleEndian.Uint16(offline.Bytes()[tc.at*4:])); got != tc.want {
			t.Fatalf("gain at %d: %d want %d", tc.at, got, tc.want)
		}
	}
	p, err = p.SetClipGain(project.ClipRef{TrackID: "V1", ClipID: "base"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.SetClipGain(project.ClipRef{TrackID: "V2", ClipID: "top"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = Build(p, "hlg", 30)
	if err != nil || plan.Contract.Audio {
		t.Fatalf("all zero gain audio produced a mux contribution: %v", err)
	}
}

func TestMutedClipOfflinePlanPreservesVideo(t *testing.T) {
	p := exportFixture(t)
	var err error
	for _, ref := range []project.ClipRef{
		{TrackID: "V1", ClipID: "base"},
		{TrackID: "V2", ClipID: "top"},
		{TrackID: "A1", ClipID: "audio"},
	} {
		p, err = p.SetClipGain(ref, 175)
		if err != nil {
			t.Fatal(err)
		}
		p, err = p.SetClipMuted(ref, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	plan, err := Build(p, "hlg", 30)
	if err != nil || plan.Contract.Audio || plan.VideoFrames != 6 {
		t.Fatalf("mute affected video or retained audio: %+v %v", plan, err)
	}
	if len(plan.Spans[2].Layers) != 2 {
		t.Fatal("mute removed visible layers")
	}
}
