package projectexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestRenderOpaqueHLGProject(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=10:d=1,format=yuv420p10le,geq=lum='180+100*N':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
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
	p := project.New()
	p.Assets = []project.Asset{asset}
	p.CanvasWidth, p.CanvasHeight = 64, 48
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", Start: 100000, In: 100000, Out: 400000}, {ID: "tail", AssetID: "a", Start: 500000, In: 300000, Out: 400000}}
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "top", AssetID: "a", Start: 200000, In: 500000, Out: 600000, Transform: &media.Transform{CanvasWidth: 64, CanvasHeight: 48, Scale: 50}}}
	out := filepath.Join(dir, "output.mp4")
	if err := RenderVideo(ctx, engine, p, "hlg", 30, false, out); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", out, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	const frameBytes = 64 * 48 * 3
	if len(raw) != 18*frameBytes {
		t.Fatalf("raw output bytes %d want %d", len(raw), 18*frameBytes)
	}
	pixel := func(frame, x, y int) int {
		return int(binary.LittleEndian.Uint16(raw[frame*frameBytes+2*(y*64+x):]))
	}
	for _, tc := range []struct {
		frame, x, y, want int
	}{{0, 4, 4, 64}, {2, 32, 24, 64}, {3, 4, 4, 280}, {5, 4, 4, 280}, {6, 4, 4, 380}, {6, 32, 24, 680}, {8, 32, 24, 680}, {9, 32, 24, 480}, {12, 4, 4, 64}, {14, 32, 24, 64}, {15, 32, 24, 480}, {17, 32, 24, 480}} {
		if got := pixel(tc.frame, tc.x, tc.y); got < tc.want-12 || got > tc.want+12 {
			t.Fatalf("frame %d at %d,%d: luma %d want %d", tc.frame, tc.x, tc.y, got, tc.want)
		}
	}
	q, err := p.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "top"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	blendPath := filepath.Join(dir, "blend.mp4")
	if err := RenderVideo(ctx, engine, q, "hlg", 30, false, blendPath); err != nil {
		t.Fatal(err)
	}
	blended, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", blendPath, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-").Output()
	if err != nil || len(blended) != len(raw) {
		t.Fatalf("blended output: %v", err)
	}
	if got := int(binary.LittleEndian.Uint16(blended[6*frameBytes+2*(24*64+32):])); got < 575 || got > 599 {
		t.Fatalf("linear blend luma %d want 587 (not code-space average 530)", got)
	}
	if got := int(binary.LittleEndian.Uint16(blended[6*frameBytes+2*(4*64+4):])); got < 368 || got > 392 {
		t.Fatalf("uncovered base luma changed: %d", got)
	}
	current, err := os.ReadFile(src)
	if err != nil || sha256.Sum256(original) != sha256.Sum256(current) {
		t.Fatal("source changed during export")
	}
	published, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := RenderVideo(ctx, engine, p, "hlg", 30, false, out); err == nil {
		t.Fatal("existing destination overwritten")
	}
	after, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(published, after) {
		t.Fatal("existing output changed")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	missing := filepath.Join(dir, "cancelled.mp4")
	if err := RenderVideo(cancelled, engine, p, "hlg", 30, false, missing); err == nil {
		t.Fatal("cancelled export accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("cancelled output published")
	}
	broken := engine
	broken.FFmpeg = filepath.Join(dir, "missing-ffmpeg")
	failed := filepath.Join(dir, "failed.mp4")
	if err := RenderVideo(ctx, broken, p, "hlg", 30, false, failed); err == nil {
		t.Fatal("missing encoder accepted")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("failed output published")
	}
	files, err := filepath.Glob(filepath.Join(dir, ".pixelcut-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("staged output leaked: %v %v", files, err)
	}
	t.Log("HLG Main 10 project output: 18 frames, opaque overlay, black gaps, exact CFR duration, original source unchanged")
}
