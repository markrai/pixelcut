package projectexport

import (
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestExportQualificationVFRAllRates(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "vfr.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=10:d=0.5,format=yuv420p10le,geq=lum=180+100*N:cb=512:cr=512,setsar=1", "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000:duration=0.5", "-vf", "select=not(eq(n\\,2))", "-fps_mode", "passthrough", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("VFR fixture: %v %s", err, data)
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
	index, err := engine.ExportFrames(ctx, src)
	if err != nil || index.Count() != 4 {
		t.Fatalf("VFR source frame count: %v", err)
	}
	for _, tc := range []struct {
		tick int64
		want int
	}{{0, 0}, {99999, 0}, {100000, 1}, {299999, 1}, {300000, 2}, {400000, 3}} {
		if got, ok := index.OrdinalAtOrBefore(tc.tick); !ok || got != tc.want {
			t.Fatalf("VFR source ordinal at %d: %d want %d", tc.tick, got, tc.want)
		}
	}
	p := project.New()
	p.Assets = []project.Asset{asset}
	p.Tracks[0].Clips = []project.Clip{{ID: "vfr", AssetID: "a", Start: 10001, In: 50000, Out: 450001}}
	for _, fps := range []int{24, 25, 30, 50, 60} {
		t.Run(fmt.Sprint(fps), func(t *testing.T) {
			plan, err := Build(p, "hlg", fps)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, fmt.Sprintf("out-%d.mp4", fps))
			if err := RenderProject(ctx, engine, p, "hlg", fps, out); err != nil {
				t.Fatal(err)
			}
			raw, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", out, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-").Output()
			const frameBytes = 64 * 48 * 3
			if err != nil || int64(len(raw)) != plan.VideoFrames*frameBytes {
				t.Fatalf("decode VFR export: %d bytes %v", len(raw), err)
			}
			for n := int64(0); n < plan.VideoFrames; n++ {
				tick, ok := plan.FrameTick(n)
				if !ok {
					t.Fatal("missing output tick")
				}
				want := uint16(64)
				if tick >= 10001 {
					sourceTick := tick - 10001 + 50000
					switch {
					case sourceTick < 100000:
						want = 180
					case sourceTick < 300000:
						want = 280
					case sourceTick < 400000:
						want = 480
					default:
						want = 580
					}
				}
				got := binary.LittleEndian.Uint16(raw[n*frameBytes+2*(24*64+32):])
				if absCode(got, want) > 12 {
					t.Fatalf("fps %d frame %d tick %d luma %d want %d", fps, n, tick, got, want)
				}
			}
			report, err := engine.Inspect(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyProjectOutput(plan, report); err != nil {
				t.Fatal(err)
			}
			t.Logf("VFR predecessor samples, exact CFR grid and AAC presentation: %d frames / %d samples", plan.VideoFrames, plan.OutputAudioFrames)
		})
	}
}

func BenchmarkHLGBlend4K(b *testing.B) {
	c := media.CanvasSize{Width: 3840, Height: 2160}
	full := media.Placement{Width: c.Width, Height: c.Height}
	dst := black444(c)
	layers := []layerFrame{solidLayer(full, 100, ycbcr10{400, 570, 420}), solidLayer(full, 50, ycbcr10{680, 460, 550})}
	b.SetBytes(int64(len(dst)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := compositeHLG(context.Background(), dst, c, layers); err != nil {
			b.Fatal(err)
		}
	}
}
