package projectexport

import (
	"encoding/binary"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestOpaqueBlackAndOddPlacement(t *testing.T) {
	c := media.CanvasSize{Width: 8, Height: 6}
	frame := black444(c)
	for i := 0; i < c.Width*c.Height*3; i++ {
		want := uint16(512)
		if i < c.Width*c.Height {
			want = 64
		}
		if got := binary.LittleEndian.Uint16(frame[2*i:]); got != want {
			t.Fatalf("black sample %d = %d want %d", i, got, want)
		}
	}
	b := media.Placement{DestX: 3, DestY: 1, Width: 3, Height: 3}
	src := make([]byte, 3*3*6)
	for i := 0; i < len(src); i += 2 {
		binary.LittleEndian.PutUint16(src[i:], 700)
	}
	if err := copyOpaque(frame, src, c, b); err != nil {
		t.Fatal(err)
	}
	for plane := 0; plane < 3; plane++ {
		for y := 0; y < c.Height; y++ {
			for x := 0; x < c.Width; x++ {
				want := uint16(512)
				if plane == 0 {
					want = 64
				}
				if x >= 3 && x < 6 && y >= 1 && y < 4 {
					want = 700
				}
				i := 2 * (plane*c.Width*c.Height + y*c.Width + x)
				if got := binary.LittleEndian.Uint16(frame[i:]); got != want {
					t.Fatalf("plane %d at %d,%d = %d want %d", plane, x, y, got, want)
				}
			}
		}
	}
}

func TestOpaqueRejectsAudioAndTranslucency(t *testing.T) {
	p := exportFixture(t)
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOpaque(plan, false); err == nil {
		t.Fatal("enabled audio silently discarded")
	}
	if err := validateOpaque(plan, true); err == nil {
		t.Fatal("partial opacity silently discarded")
	}
	p.Tracks[2].Clips[0].Opacity = nil
	plan, err = Build(p, "hlg", 30)
	if err != nil || validateOpaque(plan, true) != nil {
		t.Fatalf("opaque project rejected: %v", err)
	}
}
