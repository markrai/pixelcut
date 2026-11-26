package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStreamDecoderReuseAndRetirement(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	path := filepath.Join(t.TempDir(), "stream.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=2:d=2,format=yuv420p10le,geq=lum='128+200*N':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := NewStreamDecoder(ctx, Default())
	defer d.Close()
	req := StillRequest{Path: path, ClipID: "one", Tick: 250000, SourceEnd: 2000000, Width: 64, Height: 48, Range: "tv", Canvas: CanvasSize{64, 48}, OutputWidth: 32, OutputHeight: 24}
	if err := d.Retain([]string{"one"}); err != nil {
		t.Fatal(err)
	}
	first, err := d.Decode(ctx, req)
	if err != nil || !first.Valid() {
		t.Fatalf("first streamed frame: %v", err)
	}
	stream := d.streams["one"]
	var last StillFrame
	for i := 0; i < 5; i++ {
		req.Tick += CompositeStep
		last, err = d.Decode(ctx, req)
		if err != nil || !last.Valid() || d.streams["one"] != stream {
			t.Fatalf("persistent stream not reused: %v", err)
		}
	}
	if last.Pixels[0] <= first.Pixels[0] {
		t.Fatal("stream did not advance source pictures")
	}
	req.Tick = 1500000
	if _, err = d.Decode(ctx, req); err != nil || d.streams["one"] == stream || stream.cmd.ProcessState == nil {
		t.Fatalf("discontinuity did not retire the old decoder: %v", err)
	}
	stream = d.streams["one"]
	if err = d.Retain(nil); err != nil || len(d.streams) != 0 || stream.cmd.ProcessState == nil {
		t.Fatal("inactive decoder was not reaped")
	}
	if err = d.Retain([]string{"a", "b", "c", "d", "e"}); err == nil {
		t.Fatal("excess simultaneous layers accepted")
	}
	cancel()
	if _, err = d.Decode(ctx, req); err == nil {
		t.Fatal("canceled run accepted a decode")
	}
}

func TestStreamDecoderRejectsInvalidRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := NewStreamDecoder(ctx, Default())
	defer d.Close()
	if _, err := d.Decode(ctx, StillRequest{}); err == nil {
		t.Fatal("missing clip identity accepted")
	}
	if err := d.Retain([]string{""}); err == nil {
		t.Fatal("missing active identity accepted")
	}
}
