package projectexport

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestRenderNativeSDR(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_SDR_EXPORT_GPU") == "1"
	engine := encoderSafetyEngine(t, physical)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()
	hlg := filepath.Join(dir, "hlg.mp4")
	sdr := filepath.Join(dir, "sdr.mp4")
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=0.6,format=yuv420p10le,geq=lum=721:cb=512:cr=512,setsar=1", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=0.6", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", hlg)
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
	native := p.Clone()
	native.Tracks[0].Clips = []project.Clip{{ID: "native", AssetID: p.Assets[1].ID, Out: 400000}}
	fingerprint := q.Fingerprint()
	modes := backendMatrixModes[:1]
	if physical {
		modes = backendMatrixModes[:]
	}
	type sample struct{ frame, want int }
	for _, tc := range []struct {
		name    string
		project project.Project
		frames  []sample
	}{
		{"native", native, []sample{{0, 235}, {11, 235}}},
		{"cut", p, []sample{{0, 206}, {5, 206}, {6, 235}, {17, 235}}},
		{"dissolve", q, []sample{{0, 206}, {6, 206}, {9, 221}, {12, 235}, {17, 235}}},
	} {
		plan, err := Build(tc.project, media.ExportColorSDR, 30)
		if err != nil {
			t.Fatal(err)
		}
		var audio []byte
		for _, mode := range modes {
			out := filepath.Join(dir, tc.name+"-"+mode.name+".mp4")
			var events []Progress
			if err := RenderProjectObserved(ctx, mode.engine(engine), tc.project, media.ExportColorSDR, 30, out, func(v Progress) { events = append(events, v) }); err != nil {
				t.Fatalf("%s/%s: %v", tc.name, mode.name, err)
			}
			if err := matrixVerifyProgress(events, plan, mode); err != nil {
				t.Fatal(err)
			}
			matrixVerifyOutput(t, ctx, engine, plan, out)
			got, err := encoderLongAudioHash(ctx, engine, out)
			if err != nil || (audio != nil && !bytes.Equal(got, audio)) {
				t.Fatalf("audio parity: %v", err)
			}
			audio = got
			raw, err := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-i", out, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p", "-fps_mode", "passthrough", "-f", "rawvideo", "-").Output()
			const frameBytes = 192 * 144 * 3 / 2
			if err != nil || int64(len(raw)) != plan.VideoFrames*frameBytes {
				t.Fatalf("SDR output: %d bytes %v", len(raw), err)
			}
			for _, sample := range tc.frames {
				y := int(raw[sample.frame*frameBytes])
				if y < sample.want-4 || y > sample.want+4 {
					t.Fatalf("%s/%s frame %d: luma=%d want=%d", tc.name, mode.name, sample.frame, y, sample.want)
				}
			}
			t.Logf("%s/%s: native SDR output, timing and audio passed", tc.name, mode.name)
			requireEncoderNoStages(t, dir)
		}
	}
	for path, before := range original {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("SDR export changed source")
		}
	}
	if q.Fingerprint() != fingerprint {
		t.Fatal("SDR export changed project")
	}
}
