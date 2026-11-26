package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markrai/pixelcut/internal/mediacache"
)

func TestStillFrameIndexPredecessor(t *testing.T) {
	f := &FrameIndex{ticks: []int64{100, 350, 900}}
	for _, tc := range []struct {
		at, want int64
		ok       bool
	}{{0, 0, false}, {100, 100, true}, {349, 100, true}, {350, 350, true}, {899, 350, true}, {900, 900, true}, {1500, 900, true}} {
		got, ok := f.AtOrBefore(tc.at)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("at %d: %d %t", tc.at, got, ok)
		}
	}
}

func TestStillFilterUsesTransparentGeometry(t *testing.T) {
	req := StillRequest{Path: "a.mp4", Width: 160, Height: 120, Canvas: CanvasSize{160, 120}, OutputWidth: 37, OutputHeight: 29,
		Crop: &Crop{X: 8, Y: 12, Width: 144, Height: 96}, Transform: &Transform{CanvasWidth: 160, CanvasHeight: 120, Scale: 200, Rotation: 90, FlipHorizontal: true}}
	graph, err := stillFilter(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"crop=144:96:8:12", "transpose=clock,hflip", "crop=", "tonemap=mobius", "format=bgra", "scale=37:29"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("missing %q in %q", want, graph)
		}
	}
	if strings.Contains(graph, "pad=") {
		t.Fatal("layer was flattened over black")
	}
	req.OutputWidth = MaxStillSide + 1
	if _, err = stillFilter(req); err == nil {
		t.Fatal("unbounded output accepted")
	}
}

func TestStillCacheBudgetAndIsolation(t *testing.T) {
	d := NewStillDecoder(Default())
	frame := StillFrame{Width: 960, Height: 540, Pixels: make([]byte, 960*540*4)}
	for i := 0; i < 20; i++ {
		d.store(stillKey{source: mediacache.Source{Path: "a"}, tick: int64(i)}, frame)
		if d.bytes > stillCacheBytes || len(d.frames) > stillCacheEntries {
			t.Fatal("cache exceeded budget")
		}
	}
	frame.Pixels[0] = 99
	if d.frames[len(d.frames)-1].frame.Pixels[0] != 0 {
		t.Fatal("cache aliased caller buffer")
	}
	for i := 0; i < 100; i++ {
		d.store(stillKey{tick: int64(i)}, StillFrame{Width: 1, Height: 1, Pixels: make([]byte, 4)})
	}
	if len(d.frames) > stillCacheEntries {
		t.Fatal("small frames exceeded entry budget")
	}
}

func TestStillDecodeHLGAndCache(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	path := filepath.Join(t.TempDir(), "still.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=1:d=2,format=yuv420p10le,geq=lum='128+600*N':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
	d := NewStillDecoder(Default())
	req := StillRequest{Path: path, Tick: 500000, Width: 64, Height: 48, Range: "tv", Canvas: CanvasSize{64, 48}, OutputWidth: 32, OutputHeight: 24}
	ctx := context.Background()
	first, err := d.Decode(ctx, req)
	if err != nil || !first.Valid() {
		t.Fatalf("first frame: %v", err)
	}
	req.Tick = 1900000
	last, err := d.Decode(ctx, req)
	if err != nil || !last.Valid() {
		t.Fatalf("last frame: %v", err)
	}
	if last.Pixels[0] <= first.Pixels[0] {
		t.Fatal("timestamp selection did not reach brighter final frame")
	}
	want := last.Pixels[0]
	last.Pixels[0] = 0
	req.Tick = 1500000
	again, err := d.Decode(ctx, req)
	if err != nil || !again.Valid() || again.Pixels[0] != want || len(d.frames) != 2 || len(d.sources) != 1 {
		t.Fatalf("same indexed frame was not reused: %v", err)
	}
	req.Width = 62
	if _, err = d.Decode(ctx, req); err == nil {
		t.Fatal("changed source geometry accepted")
	}
}
