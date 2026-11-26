package composition

import (
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestDetachedAudioCompositionAndTarget(t *testing.T) {
	p := fixture(t)
	p.Tracks[1].Clips = nil
	ref := project.ClipRef{TrackID: "V1", ClipID: "early"}
	q, err := p.DetachAudio(ref, "A1", "detached")
	if err != nil {
		t.Fatal(err)
	}
	e, err := Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.At(12)
	if err != nil || len(f.Video) != 1 || len(f.Audio) != 1 || f.Audio[0].Linked || f.Audio[0].Ref.ClipID != "detached" {
		t.Fatalf("duplicate or missing audio: %+v %v", f, err)
	}
	a, at, ok := e.AudioTarget("A1", 0, true)
	if !ok || at != 10 || a.SourceTime != 100 {
		t.Fatal("audio gap-start mapping")
	}
	if _, _, ok := e.AudioTarget("A1", 0, false); ok {
		t.Fatal("audio seek jumped gap")
	}
	a, at, ok = e.AudioTarget("A1", 19, false)
	if !ok || at != 19 || a.SourceTime != 109 {
		t.Fatal("audio source seek mapping")
	}
	if _, _, ok := e.AudioTarget("A1", 30, true); ok {
		t.Fatal("audio wrapped past final clip")
	}
	if _, _, ok := e.AudioTarget("V1", 12, true); ok {
		t.Fatal("video track accepted as independent audio")
	}
}
