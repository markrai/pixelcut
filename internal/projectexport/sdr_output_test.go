package projectexport

import (
	"context"
	"math"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestSDROutputToneMappingAndComposition(t *testing.T) {
	for _, tc := range []struct {
		in, want uint16
	}{{64, 64}, {721, 824}, {940, 940}} {
		got := mapHLGToSDR(ycbcr10{tc.in, 512, 512})
		if absCode(got.Y, tc.want) > 3 || got.Cb != 512 || got.Cr != 512 {
			t.Fatalf("neutral HLG %d: %+v want %d", tc.in, got, tc.want)
		}
	}
	last := 0.0
	for i := 0; i <= 1000; i++ {
		got := sdrToneLuminance(float64(i) / 1000)
		if got < last || got > 1+1e-12 || math.IsNaN(got) {
			t.Fatal("invalid tone curve")
		}
		last = got
	}
	for _, rgb := range []linearRGB{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} {
		v := mapHLGToSDR(encodeHLG(rgb))
		if v.Y < 64 || v.Y > 940 || v.Cb < 64 || v.Cb > 960 || v.Cr < 64 || v.Cr > 960 {
			t.Fatalf("out of nominal SDR gamut: %+v", v)
		}
	}
	c := media.CanvasSize{Width: 6, Height: 4}
	b := media.Placement{Width: c.Width, Height: c.Height}
	black := solidLayer(b, 100, ycbcr10{64, 512, 512})
	white := solidLayer(b, 50, ycbcr10{940, 512, 512})
	dst := black444(c)
	ctx := context.Background()
	if err := compositeColor(ctx, dst, c, []layerFrame{black, white}, nil, decodeSDR709, encodeSDR709); err != nil || absCode(readPixel(dst, c.Width*c.Height*2, 0).Y, 682) > 2 {
		t.Fatalf("SDR linear opacity: %v", err)
	}
	white.Opacity = 100
	d := dissolveFrame{index: 0, from: black.Pixels, weight: 500000}
	if err := compositeColor(ctx, dst, c, []layerFrame{white}, &d, decodeSDR709, encodeSDR709); err != nil || absCode(readPixel(dst, c.Width*c.Height*2, 0).Y, 682) > 2 {
		t.Fatalf("SDR linear dissolve: %v", err)
	}
}
