package media

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func sampleStream() Stream {
	return Stream{CodecType: "video", CodecName: "hevc", PixelFormat: "yuv420p10le", Width: 320, Height: 240, Primaries: "bt2020", Transfer: "arib-std-b67", Matrix: "bt2020nc", Range: "tv", ChromaLocation: "left", SampleAspectRatio: "1:1", Duration: "2.0"}
}

func TestColorContract(t *testing.T) {
	v := sampleStream()
	if err := v.ValidateHLG(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Stream){func(s *Stream) { s.Transfer = "bt709" }, func(s *Stream) { s.Range = "" }, func(s *Stream) { s.PixelFormat = "yuv420p" }, func(s *Stream) { s.SideData = []map[string]any{{"rotation": 90}} }, func(s *Stream) { s.SampleAspectRatio = "2:1" }} {
		b := v
		change(&b)
		if b.ValidateHLG() == nil {
			t.Fatalf("accepted %+v", b)
		}
	}
	b := v
	b.Range = "pc"
	if verifyColor(v, b, v.Width, v.Height) == nil {
		t.Fatal("accepted changed range")
	}
}

func TestEditValidation(t *testing.T) {
	v := sampleStream()
	for _, c := range []Crop{{1, 0, 100, 100}, {0, 0, 321, 100}, {240, 0, 100, 100}, {0, 0, 0, 100}, {-2, 0, 100, 100}} {
		if c.Validate(v) == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
	if err := (Crop{20, 20, 200, 160}).Validate(v); err != nil {
		t.Fatal(err)
	}
	if (Edit{Trim: true, Start: 1, Duration: 2}).validate(v) == nil {
		t.Fatal("accepted out-of-bounds trim")
	}
}

func runFF(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("ffmpeg", args...)
	b, err := cmd.Output()
	if err != nil {
		t.Fatal(commandError("ffmpeg", err))
	}
	return b
}

func TestHLGMediaPipeline(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	ctx := context.Background()
	e := Default()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2", "-vf", "format=yuv420p10le,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-c:a", "aac", "-tag:v", "hvc1", src)
	report, err := e.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	v, err := report.Video()
	if err != nil {
		t.Fatal(err)
	}
	if err = v.ValidateHLG(); err != nil {
		t.Fatalf("%v: %+v", err, v)
	}
	copied := filepath.Join(dir, "copy.mp4")
	if err = e.Remux(ctx, src, copied); err != nil {
		t.Fatal(err)
	}
	hashes := func(p string) []byte {
		return runFF(t, "-v", "error", "-i", p, "-map", "0:v:0", "-f", "framehash", "-")
	}
	if !bytes.Equal(hashes(src), hashes(copied)) {
		t.Fatal("remux changed decoded frame hashes or timestamps")
	}
	if err = e.Remux(ctx, src, copied); err == nil {
		t.Fatal("overwrote existing output")
	}
	cropped := filepath.Join(dir, "crop.mp4")
	if err = e.Render(ctx, src, cropped, Edit{Crop: &Crop{20, 20, 200, 160}}); err != nil {
		t.Fatal(err)
	}
	trimmed := filepath.Join(dir, "trim.mp4")
	if err = e.Render(ctx, src, trimmed, Edit{Trim: true, Start: 0.5, Duration: 1}); err != nil {
		t.Fatal(err)
	}
	r, err := e.Inspect(ctx, trimmed)
	if err != nil {
		t.Fatal(err)
	}
	tv, err := r.Video()
	if err != nil {
		t.Fatal(err)
	}
	d, err := strconv.ParseFloat(tv.Duration, 64)
	if err != nil || d < 0.95 || d > 1.05 {
		t.Fatalf("unexpected duration %q", tv.Duration)
	}
	bad := filepath.Join(dir, "bad.mp4")
	if err = e.Render(ctx, src, bad, Edit{Crop: &Crop{1, 0, 100, 100}}); err == nil {
		t.Fatal("accepted invalid crop")
	}
	if _, err = os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("invalid edit published output")
	}
}
