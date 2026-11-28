package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestHLGDissolveSceneLinear(t *testing.T) {
	c := media.CanvasSize{Width: 6, Height: 4}
	full := media.Placement{Width: c.Width, Height: c.Height}
	black := solidLayer(full, 100, ycbcr10{64, 512, 512})
	white := solidLayer(full, 100, ycbcr10{940, 512, 512})
	dst := black444(c)
	d := dissolveFrame{index: 0, from: black.Pixels}
	for _, tc := range []struct {
		weight int64
		want   uint16
	}{{0, 64}, {500000, 828}, {1000000, 940}} {
		d.weight = tc.weight
		if err := compositeHLGDissolve(context.Background(), dst, c, []layerFrame{white}, &d); err != nil {
			t.Fatal(err)
		}
		want := solidLayer(full, 100, ycbcr10{tc.want, 512, 512})
		if !bytes.Equal(dst, want.Pixels) {
			t.Fatalf("wrong linear dissolve at %d: %+v", tc.weight, readPixel(dst, c.Width*c.Height*2, 0))
		}
	}
	d.weight = 500000
	overlay := solidLayer(media.Placement{DestX: 1, DestY: 1, Width: 2, Height: 2}, 50, ycbcr10{940, 512, 512})
	if err := compositeHLGDissolve(context.Background(), dst, c, []layerFrame{white, overlay}, &d); err != nil {
		t.Fatal(err)
	}
	if readPixel(dst, c.Width*c.Height*2, 0).Y != 828 || readPixel(dst, c.Width*c.Height*2, (c.Width+1)*2).Y != 894 {
		t.Fatal("transition and overlay were not composed together in linear light")
	}
	excursion := solidLayer(full, 100, ycbcr10{990, 32, 1000})
	d.from, d.weight = excursion.Pixels, 0
	if err := compositeHLGDissolve(context.Background(), dst, c, []layerFrame{white}, &d); err != nil || !bytes.Equal(dst, excursion.Pixels) {
		t.Fatalf("outgoing endpoint not exact: %v", err)
	}
	d.weight = -1
	if err := compositeHLGDissolve(context.Background(), dst, c, []layerFrame{white}, &d); err == nil {
		t.Fatal("invalid weight accepted")
	}
	d.weight = 500000
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := compositeHLGDissolve(ctx, dst, c, []layerFrame{white}, &d); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled transition: %v", err)
	}
}

func TestRenderHLGTransition(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_TRANSITION_GPU") == "1"
	engine := encoderSafetyEngine(t, physical)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()
	const width, height = 192, 144
	t.Logf("transition fixture=%dx%d physical=%t ffmpeg=%s ffprobe=%s", width, height, physical, engine.FFmpeg, engine.FFprobe)
	src := filepath.Join(dir, "source.mp4")
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", fmt.Sprintf("nullsrc=s=%dx%d:r=10:d=1,format=yuv420p10le,geq=lum='if(lt(N,2),280,if(lt(N,6),64,940))':cb=512:cr=512,setsar=1", width, height), "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=1", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	a, err := project.FromReport("a", src, report)
	if err != nil {
		t.Fatal(err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.CanvasWidth, p.CanvasHeight = width, height
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: "a", Out: 200000}, {ID: "next", AssetID: "a", Start: 200000, In: 600000, Out: 1000000}}
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
		t.Fatalf("incorrect transition duration: %+v", plan)
	}
	hardCut := filepath.Join(dir, "cut.mp4")
	if err := RenderProject(ctx, engine, p, "hlg", 30, hardCut); err != nil {
		t.Fatal(err)
	}
	audio, err := encoderLongAudioHash(ctx, engine, hardCut)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := matrixPreEncodeHashes(ctx, engine, prepared)
	if err != nil {
		t.Fatal(err)
	}
	modes := backendMatrixModes[:1]
	if physical {
		modes = backendMatrixModes[:]
		cuda := backendMatrixModes[1].engine(engine)
		got, err := matrixPreEncodeHashes(ctx, cuda, prepared)
		if err != nil || !slices.Equal(hashes, got) {
			t.Fatalf("transition CPU/CUDA pre-encode parity: %v", err)
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
		if err != nil || !bytes.Equal(audio, got) {
			t.Fatalf("%s transition changed decoded audio: %v", mode.name, err)
		}
		raw, err := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-hwaccel", "none", "-i", out, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-fps_mode", "passthrough", "-f", "rawvideo", "-").Output()
		const frameBytes = width * height * 3
		if err != nil || len(raw) != 18*frameBytes {
			t.Fatalf("%s decoded output: %d %v", mode.name, len(raw), err)
		}
		for _, tc := range []struct {
			frame, want int
		}{{5, 280}, {6, 64}, {9, 828}, {12, 940}, {17, 940}} {
			luma := int(binary.LittleEndian.Uint16(raw[tc.frame*frameBytes:]))
			if luma < tc.want-12 || luma > tc.want+12 {
				t.Fatalf("%s frame %d: luma=%d want=%d", mode.name, tc.frame, luma, tc.want)
			}
		}
		t.Logf("%s: outgoing handle, midpoint, endpoint, audio and HLG output passed", mode.name)
		requireEncoderNoStages(t, dir)
	}
	job, stop := context.WithCancel(ctx)
	defer stop()
	out := filepath.Join(dir, "cancelled.mp4")
	reached, done := false, false
	err = RenderProjectObserved(job, engine, q, "hlg", 30, out, func(v Progress) {
		done = done || v.Stage == StageDone
		if v.Stage == StageVideo && v.Completed >= 7 {
			reached = true
			stop()
		}
	})
	if !errors.Is(err, context.Canceled) || !reached || done {
		t.Fatalf("transition cancellation: %v reached=%t done=%t", err, reached, done)
	}
	requireEncoderMissing(t, out)
	requireEncoderNoStages(t, dir)
	if err := RenderProject(ctx, engine, q, "hlg", 30, filepath.Join(dir, "recovered.mp4")); err != nil {
		t.Fatalf("fresh export after cancellation: %v", err)
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(original, after) || q.Fingerprint() != fingerprint {
		t.Fatal("transition export changed source or project")
	}
	if _, err := composition.Compile(q); !errors.Is(err, composition.ErrTransitionRenderingUnavailable) {
		t.Fatalf("preview gate opened prematurely: %v", err)
	}
	requireEncoderNoStages(t, dir)
}
