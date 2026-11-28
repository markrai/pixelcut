package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestRenderMixedHLG(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_MIXED_EXPORT_GPU") == "1"
	engine := encoderSafetyEngine(t, physical)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()
	hlg := filepath.Join(dir, "hlg.mp4")
	sdr := filepath.Join(dir, "sdr.mp4")
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=0.6,format=yuv420p10le,geq=lum=64:cb=512:cr=512,setsar=1", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=0.6", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", hlg)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("HLG fixture: %v %s", err, data)
	}
	cmd = exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=0.4,format=yuv420p,geq=lum=235:cb=128:cr=128,setsar=1,setparams=range=limited:color_primaries=bt709:color_trc=bt709:colorspace=bt709", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=0.4", "-c:v", "libx264", "-threads", "2", "-pix_fmt", "yuv420p", "-x264-params", "colorprim=bt709:transfer=bt709:colormatrix=bt709:range=tv:chromaloc=0", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv", "-chroma_sample_location", "left", "-movflags", "+write_colr", "-c:a", "aac", sdr)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("SDR fixture: %v %s", err, data)
	}
	p := project.New()
	p.CanvasWidth, p.CanvasHeight = 192, 144
	original := make(map[string][]byte)
	for _, path := range []string{hlg, sdr} {
		report, err := engine.Inspect(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		a, err := project.FromImportReport(filepath.Base(path), path, report)
		if err != nil || !a.HasAudio || a.ImportOnly() || a.ColorDefaults != 0 {
			t.Fatalf("fixture import: %+v %v", a, err)
		}
		p.Assets = append(p.Assets, a)
		original[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: p.Assets[0].ID, Out: 200000}, {ID: "next", AssetID: p.Assets[1].ID, Start: 200000, Out: 400000}}
	q, err := p.SetVideoTransition(project.ClipRef{TrackID: "V1", ClipID: "next"}, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first", Duration: 200000})
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := q.Fingerprint()
	prepared, err := Prepare(ctx, engine, q, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.Plan
	if plan.VideoFrames != 18 || plan.OutputAudioFrames != 28800 {
		t.Fatalf("mixed output timing: %+v", plan)
	}
	hardCut := filepath.Join(dir, "cut.mp4")
	if err := RenderProject(ctx, engine, p, "hlg", 30, hardCut); err != nil {
		t.Fatal(err)
	}
	audio, err := encoderLongAudioHash(ctx, engine, hardCut)
	if err != nil {
		t.Fatal(err)
	}
	modes := backendMatrixModes[:1]
	if physical {
		modes = backendMatrixModes[:]
		cpu, err := matrixPreEncodeHashes(ctx, backendMatrixModes[0].engine(engine), prepared)
		if err != nil {
			t.Fatal(err)
		}
		gpu, err := matrixPreEncodeHashes(ctx, backendMatrixModes[1].engine(engine), prepared)
		if err != nil || !slices.Equal(cpu, gpu) {
			t.Fatalf("mixed CPU/CUDA pre-encode parity: %v", err)
		}
	}
	for _, mode := range modes {
		out := filepath.Join(dir, mode.name+".mp4")
		var events []Progress
		if err := RenderProjectObserved(ctx, mode.engine(engine), q, "hlg", 30, out, func(v Progress) { events = append(events, v) }); err != nil {
			t.Fatalf("%s: %v", mode.name, err)
		}
		if err := matrixVerifyProgress(events, plan, mode); err != nil {
			t.Fatal(err)
		}
		matrixVerifyOutput(t, ctx, engine, plan, out)
		got, err := encoderLongAudioHash(ctx, engine, out)
		if err != nil || !bytes.Equal(got, audio) {
			t.Fatalf("%s audio differs: %v", mode.name, err)
		}
		raw, err := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-hwaccel", "none", "-i", out, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-fps_mode", "passthrough", "-f", "rawvideo", "-").Output()
		const frameBytes = 192 * 144 * 3
		if err != nil || len(raw) != 18*frameBytes {
			t.Fatalf("%s output: %d bytes %v", mode.name, len(raw), err)
		}
		for _, tc := range []struct {
			frame, want int
		}{{5, 64}, {6, 64}, {9, 596}, {12, 721}, {17, 721}} {
			y := int(binary.LittleEndian.Uint16(raw[tc.frame*frameBytes:]))
			if y < tc.want-12 || y > tc.want+12 {
				t.Fatalf("%s frame %d: luma=%d want=%d", mode.name, tc.frame, y, tc.want)
			}
		}
		t.Logf("%s: mixed HLG transition, reference white, timing and audio passed", mode.name)
		requireEncoderNoStages(t, dir)
	}
	overlay := p.Clone()
	overlay.Tracks[0].Clips = overlay.Tracks[0].Clips[:1]
	overlay, _, err = overlay.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	overlay.Tracks[2].Clips = []project.Clip{{ID: "overlay", AssetID: p.Assets[1].ID, Out: 200000, AudioMuted: true, Crop: &media.Crop{X: 8, Y: 8, Width: 176, Height: 128}, Transform: &media.Transform{CanvasWidth: 192, CanvasHeight: 144, Scale: 50}}}
	overlay, err = overlay.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "overlay"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	overlayOut := filepath.Join(dir, "overlay.mp4")
	if err := RenderProject(ctx, engine, overlay, "hlg", 30, overlayOut); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-i", overlayOut, "-map", "0:v:0", "-frames:v", "1", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-").Output()
	if err != nil || len(raw) != 192*144*3 {
		t.Fatalf("overlay frame: %d bytes %v", len(raw), err)
	}
	for _, tc := range []struct {
		x, y int
		want uint16
	}{{8, 8, 64}, {96, 72, 596}} {
		got := binary.LittleEndian.Uint16(raw[2*(tc.y*192+tc.x):])
		if absCode(got, tc.want) > 12 {
			t.Fatalf("mixed overlay at %d,%d: %d want %d", tc.x, tc.y, got, tc.want)
		}
	}
	requireEncoderNoStages(t, dir)
	for path, before := range original {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("mixed export changed source")
		}
	}
	if q.Fingerprint() != fingerprint {
		t.Fatal("mixed export changed project")
	}
}
