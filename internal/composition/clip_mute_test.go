package composition

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestClipMuteSuppressesOnlyAudioAndSkipsAudition(t *testing.T) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 100000, Width: 8, Height: 8, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "v", AssetID: "a", Out: 100000, AudioMuted: true}}
	p.Tracks[1].Clips = []project.Clip{{ID: "a1", AssetID: "a", Out: 50000, AudioMuted: true}, {ID: "a2", AssetID: "a", Start: 50000, Out: 50000}}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.At(0)
	if err != nil || len(f.Video) != 1 || len(f.Audio) != 0 {
		t.Fatal("clip mute suppressed video or leaked audio")
	}
	if ref, at, ok := e.AuditionTarget("V1", 0, true); !ok || at != 0 || ref.ClipID != "v" {
		t.Fatal("muted video not available for audition")
	}
	if _, _, ok := e.AudioTarget("A1", 0, false); ok {
		t.Fatal("muted audio audition accepted")
	}
	if a, at, ok := e.AudioTarget("A1", 0, true); !ok || at != 50000 || a.Ref.ClipID != "a2" {
		t.Fatal("muted clip blocked later audible audio")
	}
	d := &fakePCM{}
	block, err := MixPCM(context.Background(), e, d, 0, 0, 2400)
	if err != nil || len(d.calls) != 0 {
		t.Fatal("muted contributions decoded")
	}
	for _, s := range block.Samples {
		if s != 0 {
			t.Fatal("muted PCM nonzero")
		}
	}
}
