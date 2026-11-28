package media

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSDRPreviewGraphAndBackend(t *testing.T) {
	c := SourceColor{Family: ColorSDR709, Codec: "h264", PixelFormat: "yuv420p", Range: "tv", Chroma: "left"}
	req := StillRequest{Path: "screen.mp4", Width: 192, Height: 144, Range: "tv", Color: c, Canvas: CanvasSize{192, 144}, OutputWidth: 96, OutputHeight: 72}
	g, err := stillFilter(req)
	if err != nil || strings.Contains(g, "tonemap") || !strings.Contains(g, "in_color_matrix=bt709:in_range=tv:out_range=pc") || !strings.Contains(g, "in_h_chr_pos=0") {
		t.Fatalf("SDR graph: %s %v", g, err)
	}
	req.Color.Range, req.Color.Chroma = "pc", "center"
	g, err = stillFilter(req)
	if err != nil || !strings.Contains(g, "in_range=pc") || !strings.Contains(g, "in_h_chr_pos=128") {
		t.Fatalf("full range graph: %s %v", g, err)
	}
	engine := Default()
	engine.DecodeBackend = BackendCUDA
	if previewEngineForSource(engine, c).DecodeBackend != BackendCPU || previewEngineForSource(engine, SourceColor{}).DecodeBackend != BackendCUDA {
		t.Fatal("incorrect per-source decode policy")
	}
	req.Color.Family = ColorPQ
	if _, err := stillFilter(req); err == nil {
		t.Fatal("PQ preview accepted")
	}
}

func TestSDRPreviewRealMedia(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "sdr.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=0.5,geq=lum='if(lt(N,2),16,235)':cb=128:cr=128,setsar=1", "-c:v", "libx264", "-threads", "2", "-pix_fmt", "yuv420p", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv", "-chroma_sample_location", "left", path)
	e := Default()
	report, err := e.Inspect(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := report.Video()
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.ClassifySourceColor()
	if err != nil || c.Family != ColorSDR709 {
		t.Fatalf("fixture classification: %+v %v", c, err)
	}
	e.DecodeBackend = BackendCUDA
	still := NewStillDecoder(e)
	stream := NewStreamDecoder(ctx, e)
	defer stream.Close()
	req := StillRequest{Path: path, ClipID: "sdr", SourceEnd: 500000, Width: 192, Height: 144, Range: c.Range, Color: c, Canvas: CanvasSize{192, 144}, OutputWidth: 96, OutputHeight: 72}
	for _, at := range []int64{0, 100000, 200000, 300000} {
		req.Tick = at
		a, err := still.Decode(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		b, err := stream.Decode(ctx, req)
		if err != nil || !bytes.Equal(a.Pixels, b.Pixels) {
			t.Fatalf("still/stream parity at %d: %v", at, err)
		}
		for _, value := range a.Pixels[:3] {
			if at < 200000 && value > 3 || at >= 200000 && value < 252 {
				t.Fatalf("SDR range at %d: %v", at, a.Pixels[:4])
			}
		}
	}
	if stream.Stats().Opened != 1 {
		t.Fatalf("SDR decoder reopened: %+v", stream.Stats())
	}
	req.Color.Chroma = "center"
	if _, err := still.Decode(ctx, req); err == nil {
		t.Fatal("changed descriptor accepted")
	}
	req.Tick = 400000
	if _, err := stream.Decode(ctx, req); err == nil {
		t.Fatal("stream reused changed descriptor")
	}
	thumb, err := e.Thumbnail(ctx, path, .3, 96, 72)
	if err != nil || len(thumb) != 96*72*4 || thumb[0] < 252 {
		t.Fatalf("SDR thumbnail: %v", err)
	}
}
