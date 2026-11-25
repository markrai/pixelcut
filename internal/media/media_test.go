package media

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/markrai/pixelcut/internal/mediacache"
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
	thumb,err:=e.Thumbnail(ctx,src,0.5,48,32)
	if err!=nil || len(thumb)!=48*32*4 {t.Fatalf("thumbnail: %d bytes: %v",len(thumb),err)}
	if _,err:=e.Thumbnail(ctx,src,0.5,47,32);err==nil {t.Fatal("accepted odd thumbnail width")}
	cacheDir:=filepath.Join(dir,"cache")
	cache,err:=mediacache.Open(cacheDir)
	if err!=nil {t.Fatal(err)}
	pixels,err:=e.CachedThumbnail(ctx,cache,src,500000,48,32)
	if err!=nil || !bytes.Equal(pixels,thumb) {t.Fatal("cached thumbnail differs")}
	peaks,err:=e.CachedWaveform(ctx,cache,src,2000000)
	if err!=nil || peaks.Peak(0,2000000)==0 {t.Fatalf("cached waveform: %v",err)}
	frames,err:=e.CachedFrames(ctx,cache,src)
	if err!=nil || frames.Count()!=48 {t.Fatalf("cached frames: %v",err)}
	metadata,err:=e.CachedImportReport(ctx,cache,src)
	if err!=nil || metadata.Format!=nil || len(metadata.Streams)!=len(report.Streams) {t.Fatalf("cached metadata: %v",err)}
	if cache.Stats().Entries!=4 {t.Fatalf("cache population: %+v",cache.Stats())}
	cache,err=mediacache.Open(cacheDir)
	if err!=nil {t.Fatal(err)}
	pixels,err=e.CachedThumbnail(ctx,cache,src,500000,48,32)
	if err!=nil || !bytes.Equal(pixels,thumb) {t.Fatal("reopened thumbnail differs")}
	again,err:=e.CachedWaveform(ctx,cache,src,2000000)
	if err!=nil || again.Peak(0,2000000)!=peaks.Peak(0,2000000) {t.Fatal("reopened waveform differs")}
	index,err:=e.CachedFrames(ctx,cache,src)
	if err!=nil || index.Count()!=frames.Count() {t.Fatal("reopened frame index differs")}
	metadata,err=e.CachedImportReport(ctx,cache,src)
	if err!=nil {t.Fatal(err)}
	mv,err:=metadata.Video()
	if err!=nil || mv.ValidateHLG()!=nil {t.Fatal("reopened metadata invalid")}
	if cache.Stats().Entries!=4 {t.Fatal("cache keys changed across reopening")}

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

func TestPixelPortraitPipeline(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	ctx := context.Background()
	e := Default()
	dir := t.TempDir()
	base := filepath.Join(dir, "base.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2", "-vf", "format=yuv420p10le,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-c:a", "aac", "-tag:v", "hvc1", base)
	sub := filepath.Join(dir, "metadata.srt")
	if err := os.WriteFile(sub, []byte("1\n00:00:00,000 --> 00:00:01,900\nSynthetic metadata\n"), 0600); err != nil {
		t.Fatal(err)
	}
	mux := filepath.Join(dir, "mux.mp4")
	runFF(t, "-v", "error", "-display_rotation:v:0", "-90", "-i", base, "-i", sub, "-map", "0:v", "-map", "0:a", "-map", "1:s", "-c", "copy", "-c:s", "mov_text", "-metadata:s:s:0", "handler_name=MetaHandle", "-metadata", "location=+00.0000+000.0000/", mux)
	b, err := os.ReadFile(mux)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"tx3g", "mett"}, {"sbtl", "meta"}} {
		if bytes.Count(b, []byte(pair[0])) != 1 {
			t.Fatalf("unexpected fixture sample entry or handler %q", pair[0])
		}
		b = bytes.Replace(b, []byte(pair[0]), []byte(pair[1]), 1)
	}
	src := filepath.Join(dir, "pixel.mp4")
	if err = os.WriteFile(src, b, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := e.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	v, err := r.Video()
	if err != nil {
		t.Fatal(err)
	}
	rotation, err := v.DisplayRotation()
	if err != nil || rotation != -90 || len(r.Streams) != 3 || !r.Streams[2].pixelMetadata() {
		t.Fatalf("invalid fixture rotation=%d err=%v streams=%+v", rotation, err, r.Streams)
	}
	if tags, ok := r.Format["tags"].(map[string]any); !ok || tags["location"] == nil {
		t.Fatal("fixture missing location metadata")
	}
	copyPath := filepath.Join(dir, "copy.mp4")
	if err = e.Remux(ctx, src, copyPath); err != nil {
		t.Fatal(err)
	}
	hashes := func(p string) []byte {
		return runFF(t, "-v", "error", "-i", p, "-map", "0:v:0", "-map", "0:a:0", "-f", "framehash", "-")
	}
	if !bytes.Equal(hashes(src), hashes(copyPath)) {
		t.Fatal("remux changed decoded audio/video frames or timestamps")
	}
	for _, tc := range []struct {
		name string
		edit Edit
		w, h int
	}{{"trim", Edit{Trim: true, Start: 0.5, Duration: 1}, 240, 320}, {"crop", Edit{Crop: &Crop{20, 20, 200, 280}}, 200, 280}, {"combined", Edit{Trim: true, Start: 0.5, Duration: 1, Crop: &Crop{20, 20, 200, 280}}, 200, 280}, {"canvas", Edit{Transform:&Transform{CanvasWidth:320,CanvasHeight:240,Scale:75,X:20,Y:-10},Crop:&Crop{20,20,200,280}},320,240}, {"zoom", Edit{Transform:&Transform{CanvasWidth:240,CanvasHeight:320,Scale:150,X:10,Y:20}},240,320}, {"rotate", Edit{Transform:&Transform{CanvasWidth:320,CanvasHeight:240,Scale:100,Rotation:90}},320,240}, {"rotate-flip-crop", Edit{Transform:&Transform{CanvasWidth:280,CanvasHeight:200,Scale:100,Rotation:270,FlipHorizontal:true},Crop:&Crop{20,20,200,280}},280,200}} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(dir, tc.name+".mp4")
			if err := e.Render(ctx, src, out, tc.edit); err != nil {
				t.Fatal(err)
			}
			r, err := e.Inspect(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			v, err := r.Video()
			if err != nil || v.Width != tc.w || v.Height != tc.h || len(r.Streams) != 2 {
				t.Fatalf("unexpected output: %+v %v", r, err)
			}
			if tc.edit.Trim {
				d, err := strconv.ParseFloat(v.Duration, 64)
				if err != nil || d < 0.95 || d > 1.05 {
					t.Fatalf("unexpected trimmed duration %q", v.Duration)
				}
			}
		})
	}
	if err = e.Render(ctx, src, filepath.Join(dir, "bad.mp4"), Edit{Crop: &Crop{0, 0, 300, 200}}); err == nil {
		t.Fatal("accepted crop outside displayed width")
	}
	if err = e.Remux(ctx, mux, filepath.Join(dir, "unknown.mp4")); err == nil {
		t.Fatal("silently discarded an unsupported subtitle stream")
	}
}
