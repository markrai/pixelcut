package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"os/exec"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

func TestHLGRangeAndChromaNormalization(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	const w, h = 8, 4
	const n = w * h
	input := make([]byte, n*3)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(input[2*i:], []uint16{0, 256, 512, 1023}[i%4])
	}
	for i := 0; i < n/4; i++ {
		binary.LittleEndian.PutUint16(input[2*n+2*i:], []uint16{128, 384, 640, 896}[i%4])
		binary.LittleEndian.PutUint16(input[2*n+n/2+2*i:], 512)
	}
	p := exportFixture(t)
	p.Assets[0].Width, p.Assets[0].Height = w, h
	p.Assets[0].Range = "pc"
	p.CanvasWidth, p.CanvasHeight = w, h
	p.Tracks[2].Clips = nil
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	layer := plan.Spans[1].Layers[0]
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	normalize := func(chroma string) []byte {
		t.Helper()
		graph, err := layerFilter(layer, Source{Asset: p.Assets[0], Chroma: chroma}, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "rawvideo", "-pixel_format", "yuv420p10le", "-video_size", "8x4", "-i", "pipe:0", "-vf", graph, "-frames:v", "1", "-c:v", "rawvideo", "-pix_fmt", "yuv444p10le", "-threads", "1", "-f", "rawvideo", "-")
		cmd.Stdin = bytes.NewReader(input)
		raw, err := cmd.Output()
		if err != nil || len(raw) != n*6 {
			t.Fatalf("normalize %s: %v (%d bytes)", chroma, err, len(raw))
		}
		return raw
	}
	left, center := normalize("left"), normalize("center")
	for _, raw := range [][]byte{left, center} {
		for i, want := range []uint16{64, 283, 502, 940} {
			if got := readPixel(raw, n*2, 2*i); absCode(got.Y, want) > 1 || got.Cr != 512 {
				t.Fatalf("range normalization at %d: %+v want Y=%d Cr=512", i, got, want)
			}
		}
	}
	if a, b := readPixel(left, n*2, 6), readPixel(center, n*2, 6); a.Cb <= b.Cb+30 {
		t.Fatalf("chroma siting was not converted: left=%+v center=%+v", a, b)
	}
}
