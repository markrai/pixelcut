package projectexport

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestHLGReferenceValues(t *testing.T) {
	for _, tc := range []struct {
		code, linear float64
	}{{0, 0}, {0.5, 1.0 / 12}, {0.7385492680658274, 0.25}, {0.8716434713446153, 0.5}, {1, 1}} {
		if got := hlgToLinear(tc.code); math.Abs(got-tc.linear) > 1e-7 {
			t.Fatalf("inverse HLG(%f) = %.9f want %.9f", tc.code, got, tc.linear)
		}
		if got := linearToHLG(tc.linear); math.Abs(got-tc.code) > 1e-7 {
			t.Fatalf("HLG(%f) = %.9f want %.9f", tc.linear, got, tc.code)
		}
	}
	for _, tc := range []struct {
		rgb  linearRGB
		want ycbcr10
	}{{linearRGB{}, ycbcr10{64, 512, 512}}, {linearRGB{1, 1, 1}, ycbcr10{940, 512, 512}}, {linearRGB{1, 0, 0}, ycbcr10{294, 387, 960}}, {linearRGB{0.5, 0, 0.5}, ycbcr10{310, 793, 871}}} {
		if got := encodeHLG(tc.rgb); got != tc.want {
			t.Fatalf("encode %+v = %+v want %+v", tc.rgb, got, tc.want)
		}
	}
	for _, p := range []ycbcr10{{64, 512, 512}, {502, 512, 512}, {940, 512, 512}, {400, 570, 420}, {680, 460, 550}} {
		if got := encodeHLG(decodeHLG(p)); got != p {
			t.Fatalf("in-gamut round trip %+v = %+v", p, got)
		}
	}
}

func solidLayer(b media.Placement, opacity int, p ycbcr10) layerFrame {
	n := b.Width * b.Height * 2
	f := layerFrame{Pixels: make([]byte, n*3), Bounds: b, Opacity: opacity}
	for i := 0; i < n; i += 2 {
		writePixel(f.Pixels, n, i, p)
	}
	return f
}

func TestHLGCompositionCoverageAndOrder(t *testing.T) {
	c := media.CanvasSize{Width: 6, Height: 4}
	full := media.Placement{Width: 6, Height: 4}
	partial := media.Placement{DestX: 1, DestY: 1, Width: 3, Height: 2}
	white := ycbcr10{940, 512, 512}
	black := ycbcr10{64, 512, 512}
	f := black444(c)
	compose := func(layers ...layerFrame) {
		t.Helper()
		copy(f, black444(c))
		if err := compositeHLG(context.Background(), f, c, layers); err != nil {
			t.Fatal(err)
		}
	}
	at := func(x, y int) ycbcr10 { return readPixel(f, c.Width*c.Height*2, (y*c.Width+x)*2) }
	compose(solidLayer(partial, 50, white))
	if at(1, 1) != (ycbcr10{828, 512, 512}) || at(0, 0) != black || at(4, 1) != black {
		t.Fatal("linear-light blend or odd rectangle coverage incorrect")
	}
	compose(solidLayer(full, 50, white), solidLayer(partial, 50, white))
	if at(1, 1).Y != 894 || at(0, 0).Y != 828 {
		t.Fatal("multilayer coverage was quantized or blended in code space")
	}
	compose(solidLayer(full, 50, white), solidLayer(partial, 50, black))
	if at(1, 1).Y != 711 || at(0, 0).Y != 828 {
		t.Fatal("source-over order incorrect")
	}
	red, blue := ycbcr10{294, 387, 960}, ycbcr10{116, 960, 476}
	compose(solidLayer(full, 100, red), solidLayer(partial, 50, blue))
	got := at(1, 1)
	if absCode(got.Y, 310) > 1 || absCode(got.Cb, 793) > 1 || absCode(got.Cr, 871) > 1 || at(0, 0) != red {
		t.Fatalf("chromatic blend: %+v", got)
	}
	opaque := ycbcr10{990, 32, 1000}
	compose(solidLayer(full, 50, white), solidLayer(full, 100, opaque))
	if at(1, 1) != opaque {
		t.Fatal("opaque top layer did not bypass mixed-path clipping")
	}
	compose(solidLayer(full, 100, red), solidLayer(partial, 0, white))
	if !bytes.Equal(f, solidLayer(full, 100, red).Pixels) {
		t.Fatal("zero opacity changed opaque frame")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := compositeHLG(ctx, f, c, []layerFrame{solidLayer(full, 50, white)}); err == nil {
		t.Fatal("cancelled blend accepted")
	}
	bad := solidLayer(partial, 50, white)
	bad.Pixels = bad.Pixels[:1]
	if err := compositeHLG(context.Background(), f, c, []layerFrame{bad}); err == nil {
		t.Fatal("invalid buffer accepted")
	}
}

func absCode(a, b uint16) int {
	return max(int(a)-int(b), int(b)-int(a))
}

func BenchmarkHLGBlend(b *testing.B) {
	c := media.CanvasSize{Width: 640, Height: 360}
	full := media.Placement{Width: c.Width, Height: c.Height}
	dst := black444(c)
	layers := []layerFrame{solidLayer(full, 100, ycbcr10{400, 570, 420}), solidLayer(full, 50, ycbcr10{680, 460, 550})}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := compositeHLG(context.Background(), dst, c, layers); err != nil {
			b.Fatal(err)
		}
	}
}
