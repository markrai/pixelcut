package composition

import (
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestPCMMixerGainAndRoundOnce(t *testing.T) {
	var m PCMMixer
	if err := m.Reset(3); err != nil {
		t.Fatal(err)
	}
	a := []int16{1, -1, 30000, -30000, 20000, -20000}
	b := []int16{1, -1, -30000, 30000, 20000, -20000}
	out := make([]int16, 6)
	if err := m.Add(a, 150); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(b, 50); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(out); err != nil {
		t.Fatal(err)
	}
	want := []int16{2, -2, 30000, -30000, 32767, -32768}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("mixed %v want %v", out, want)
	}
	if err := m.Reset(3); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(b, 50); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(a, 150); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(out); err != nil || !reflect.DeepEqual(out, want) {
		t.Fatal("mix depends on order")
	}
	if err := m.Reset(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Add([]int16{1, -1}, 50); err != nil {
		t.Fatal(err)
	}
	if err := m.Add([]int16{1, -1}, 50); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(out[:2]); err != nil || out[0] != 1 || out[1] != -1 {
		t.Fatal("per-input rounding instead of final rounding")
	}
	if err := m.Reset(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Add([]int16{1, -1}, 50); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(out[:2]); err != nil || out[0] != 1 || out[1] != -1 {
		t.Fatal("half-ties not rounded symmetrically")
	}
	if err := m.Reset(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(out[:2]); err != nil || out[0] != 0 || out[1] != 0 {
		t.Fatal("reset leaked previous samples")
	}
}

func TestPCMMixerBounds(t *testing.T) {
	var m PCMMixer
	if m.Reset(0) == nil || m.Reset(media.PCMBlockFrames+1) == nil || m.Add([]int16{1, 2}, 100) == nil {
		t.Fatal("invalid mix initialized")
	}
	if err := m.Reset(1); err != nil {
		t.Fatal(err)
	}
	if m.Add([]int16{1}, 100) == nil || m.Add([]int16{1, 2}, -1) == nil || m.Add([]int16{1, 2}, 401) == nil || m.Finish([]int16{1}) == nil {
		t.Fatal("invalid contribution accepted")
	}
	for i := 0; i < media.MaxAudioStreams; i++ {
		if err := m.Add([]int16{32767, -32768}, 400); err != nil {
			t.Fatal(err)
		}
	}
	if m.Add([]int16{1, 2}, 100) == nil {
		t.Fatal("decoder contribution cap exceeded")
	}
	out := make([]int16, 2)
	if err := m.Finish(out); err != nil || out[0] != 32767 || out[1] != -32768 {
		t.Fatal("maximum gain saturation")
	}
}
