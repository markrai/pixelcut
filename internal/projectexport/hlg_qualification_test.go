package projectexport

import (
	"context"
	"fmt"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

type hlgQualificationFixture struct {
	name   string
	canvas media.CanvasSize
	layers []layerFrame
}

func qualificationPattern(bounds media.Placement, opacity, seed int) layerFrame {
	n := bounds.Width * bounds.Height * 2
	layer := layerFrame{Pixels: make([]byte, 3*n), Bounds: bounds, Opacity: opacity}
	for y := 0; y < bounds.Height; y++ {
		for x := 0; x < bounds.Width; x++ {
			p := ycbcr10{
				Y:  uint16(64 + (x*37+y*17+seed*29)%877),
				Cb: uint16(64 + (x*13+y*53+seed*71)%897),
				Cr: uint16(64 + (x*61+y*7+seed*43)%897),
			}
			writePixel(layer.Pixels, n, (y*bounds.Width+x)*2, p)
		}
	}
	return layer
}

func hlgQualificationFixtures() []hlgQualificationFixture {
	c := media.CanvasSize{Width: 34, Height: 26}
	full := media.Placement{Width: c.Width, Height: c.Height}
	inset := media.Placement{DestX: 3, DestY: 5, Width: 19, Height: 13}
	corner := media.Placement{DestX: 25, DestY: 19, Width: 9, Height: 7}
	overlap := media.Placement{DestX: 9, DestY: 7, Width: 23, Height: 17}
	white := ycbcr10{940, 512, 512}
	excursion := ycbcr10{990, 32, 1000}
	return []hlgQualificationFixture{
		{"black-gap", c, nil},
		{"opaque-excursion", c, []layerFrame{solidLayer(inset, 100, excursion)}},
		{"white-half", c, []layerFrame{solidLayer(full, 50, white)}},
		{"two-half-white", c, []layerFrame{solidLayer(full, 50, white), solidLayer(inset, 50, white)}},
		{"zero-opacity", c, []layerFrame{qualificationPattern(full, 100, 1), solidLayer(inset, 0, white)}},
		{"opaque-over-mixed", c, []layerFrame{qualificationPattern(full, 50, 2), solidLayer(inset, 100, excursion)}},
		{"four-layers", c, []layerFrame{qualificationPattern(full, 100, 3), qualificationPattern(inset, 1, 4), qualificationPattern(overlap, 99, 5), qualificationPattern(corner, 50, 6)}},
		{"four-translucent", c, []layerFrame{qualificationPattern(full, 25, 7), qualificationPattern(inset, 50, 8), qualificationPattern(overlap, 75, 9), qualificationPattern(corner, 1, 10)}},
		{"disjoint-rectangles", c, []layerFrame{qualificationPattern(inset, 100, 11), qualificationPattern(corner, 50, 12)}},
	}
}

func qualificationPixelClass(layers []layerFrame, x, y int) (mixed bool, exact ycbcr10) {
	for i := len(layers) - 1; i >= 0; i-- {
		l := layers[i]
		b := l.Bounds
		if l.Opacity == 0 || x < b.DestX || y < b.DestY || x >= b.DestX+b.Width || y >= b.DestY+b.Height {
			continue
		}
		if l.Opacity != 100 {
			return true, ycbcr10{}
		}
		return false, readPixel(l.Pixels, b.Width*b.Height*2, ((y-b.DestY)*b.Width+x-b.DestX)*2)
	}
	return false, ycbcr10{64, 512, 512}
}

type hlgQualificationQuality struct {
	exactPixels, mixedPixels int
	maxMixedDelta            int
}

func compareHLGQualification(f hlgQualificationFixture, reference, candidate []byte) (hlgQualificationQuality, error) {
	var q hlgQualificationQuality
	c := f.canvas
	if c.Validate() != nil || len(reference) != c.Width*c.Height*6 || len(candidate) != len(reference) {
		return q, fmt.Errorf("invalid qualification frame")
	}
	n := c.Width * c.Height * 2
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			i := (y*c.Width + x) * 2
			r, p := readPixel(reference, n, i), readPixel(candidate, n, i)
			mixed, exact := qualificationPixelClass(f.layers, x, y)
			if !mixed {
				q.exactPixels++
				if r != exact || p != exact {
					return q, fmt.Errorf("exact coverage mismatch at %d,%d: reference %+v candidate %+v expected %+v", x, y, r, p, exact)
				}
				continue
			}
			q.mixedPixels++
			for _, v := range []ycbcr10{r, p} {
				if v.Y < 64 || v.Y > 940 || v.Cb < 64 || v.Cb > 960 || v.Cr < 64 || v.Cr > 960 {
					return q, fmt.Errorf("mixed sample outside legal limited range at %d,%d: %+v", x, y, v)
				}
			}
			d := max(absCode(r.Y, p.Y), absCode(r.Cb, p.Cb), absCode(r.Cr, p.Cr))
			q.maxMixedDelta = max(q.maxMixedDelta, d)
			if d > 1 {
				return q, fmt.Errorf("mixed sample exceeds one-code provisional limit at %d,%d: reference %+v candidate %+v", x, y, r, p)
			}
		}
	}
	return q, nil
}

func TestHLGQualificationReferenceMatrix(t *testing.T) {
	for _, f := range hlgQualificationFixtures() {
		t.Run(f.name, func(t *testing.T) {
			ref, again := black444(f.canvas), black444(f.canvas)
			for _, dst := range [][]byte{ref, again} {
				if err := compositeHLG(context.Background(), dst, f.canvas, f.layers); err != nil {
					t.Fatal(err)
				}
			}
			q, err := compareHLGQualification(f, ref, again)
			if err != nil || q.exactPixels+q.mixedPixels != f.canvas.Width*f.canvas.Height || q.maxMixedDelta != 0 {
				t.Fatalf("reference matrix: %+v %v", q, err)
			}
			at := func(x, y int) ycbcr10 {
				return readPixel(ref, f.canvas.Width*f.canvas.Height*2, (y*f.canvas.Width+x)*2)
			}
			switch f.name {
			case "white-half":
				if q.mixedPixels != f.canvas.Width*f.canvas.Height || at(0, 0) != (ycbcr10{828, 512, 512}) {
					t.Fatal("half white does not match analytic reference")
				}
			case "two-half-white":
				if at(0, 0) != (ycbcr10{828, 512, 512}) || at(3, 5) != (ycbcr10{894, 512, 512}) {
					t.Fatal("two-layer coverage does not match analytic reference")
				}
			case "four-layers", "opaque-over-mixed", "disjoint-rectangles":
				if q.mixedPixels == 0 || q.exactPixels == 0 {
					t.Fatal("fixture must cover exact and mixed pixels")
				}
			}
		})
	}
}

func TestHLGQualificationComparatorRejectsErrors(t *testing.T) {
	fixtures := hlgQualificationFixtures()
	for _, f := range []hlgQualificationFixture{fixtures[0], fixtures[2]} {
		ref := black444(f.canvas)
		if err := compositeHLG(context.Background(), ref, f.canvas, f.layers); err != nil {
			t.Fatal(err)
		}
		bad := append([]byte(nil), ref...)
		n := f.canvas.Width * f.canvas.Height * 2
		p := readPixel(bad, n, 0)
		p.Y++
		writePixel(bad, n, 0, p)
		_, err := compareHLGQualification(f, ref, bad)
		if f.name == "black-gap" {
			if err == nil {
				t.Fatal("one-code opaque/black error accepted")
			}
		} else {
			if err != nil {
				t.Fatalf("one-code mixed tolerance rejected: %v", err)
			}
			p.Y++
			writePixel(bad, n, 0, p)
			if _, err := compareHLGQualification(f, ref, bad); err == nil {
				t.Fatal("two-code mixed error accepted")
			}
			p.Cb = 63
			writePixel(bad, n, 0, p)
			if _, err := compareHLGQualification(f, ref, bad); err == nil {
				t.Fatal("illegal mixed range accepted")
			}
		}
		if _, err := compareHLGQualification(f, ref, bad[:len(bad)-1]); err == nil {
			t.Fatal("short frame accepted")
		}
	}
}

func BenchmarkHLGQualificationCPU(b *testing.B) {
	for _, c := range []media.CanvasSize{{Width: 640, Height: 360}, {Width: 1920, Height: 1080}, {Width: 3840, Height: 2160}} {
		for _, opacity := range []int{100, 50} {
			b.Run(fmt.Sprintf("%dx%d/opacity%d", c.Width, c.Height, opacity), func(b *testing.B) {
				full := media.Placement{Width: c.Width, Height: c.Height}
				layers := []layerFrame{qualificationPattern(full, 100, 1), qualificationPattern(full, opacity, 2)}
				dst := black444(c)
				b.ReportAllocs()
				b.SetBytes(int64(len(dst)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := compositeHLG(context.Background(), dst, c, layers); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
