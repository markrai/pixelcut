package composition

import (
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestMeterDistinguishesFullScaleAndClipping(t *testing.T) {
	var m PCMMixer
	if err := m.Reset(4); err != nil {
		t.Fatal(err)
	}
	a := []int16{32767, -32768, 30000, -30000, 100, -100, 20000, -20000}
	b := []int16{0, 0, 10000, -10000, 0, 0, -20000, 20000}
	if err := m.Add(a, 100); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(b, 100); err != nil {
		t.Fatal(err)
	}
	var levels media.AudioLevels
	out := make([]int16, 8)
	if err := m.FinishObserved(out, &levels); err != nil {
		t.Fatal(err)
	}
	want := []int16{32767, -32768, 32767, -32768, 100, -100, 0, 0}
	if !reflect.DeepEqual(out, want) || levels.Frames != 4 || levels.Peak != [2]uint32{32767, 32768} || levels.Clipped != [2]int64{1, 1} {
		t.Fatalf("bad samples/levels: %v %+v", out, levels)
	}
	if err := m.Reset(1); err != nil {
		t.Fatal(err)
	}
	if err := m.FinishObserved(out[:2], &levels); err != nil || levels.Frames != 5 || levels.ClipCount() != 2 {
		t.Fatal("silence reset aggregate meter")
	}
	if err := m.Reset(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Add([]int16{20000, -20000}, 200); err != nil {
		t.Fatal(err)
	}
	if err := m.Add([]int16{-20000, 20000}, 200); err != nil {
		t.Fatal(err)
	}
	var canceled media.AudioLevels
	if err := m.FinishObserved(out[:2], &canceled); err != nil || canceled != (media.AudioLevels{Frames: 1}) {
		t.Fatal("meter counted pre-sum clipping")
	}
}
