package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
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

func TestRenderFullRangeCenterHLG(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "full.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=10:d=0.2,format=yuv420p10le,geq=lum='if(lt(X,32),0,1023)':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=full:chromaloc=1", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "pc", "-chroma_sample_location", "center", "-tag:v", "hvc1", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	engine := media.Default()
	report, err := engine.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := project.FromReport("a", src, report)
	if err != nil {
		t.Fatal(err)
	}
	v, err := report.Video()
	if err != nil || asset.Range != "pc" || v.ChromaLocation != "center" {
		t.Fatalf("fixture metadata: %+v %v", v, err)
	}
	p := project.New()
	p.Assets = []project.Asset{asset}
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: asset.ID, Out: asset.Duration}}
	prepared, err := Prepare(ctx, engine, p, "hlg", 30)
	if err != nil || len(prepared.Plan.Warnings) == 0 {
		t.Fatalf("normalization preflight: %v", err)
	}
	out := filepath.Join(dir, "normalized.mp4")
	if err := RenderVideo(ctx, engine, p, "hlg", 30, false, out); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", out, "-map", "0:v:0", "-frames:v", "1", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-").Output()
	if err != nil || len(raw) != 64*48*3 {
		t.Fatalf("normalized raw frame: %v (%d bytes)", err, len(raw))
	}
	black := binary.LittleEndian.Uint16(raw[2*(24*64+8):])
	white := binary.LittleEndian.Uint16(raw[2*(24*64+56):])
	if absCode(black, 64) > 12 || absCode(white, 940) > 12 {
		t.Fatalf("full-to-limited normalization: black %d white %d", black, white)
	}
}
