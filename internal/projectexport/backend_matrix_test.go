package projectexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type backendMatrixMode struct {
	name   string
	decode media.Backend
	encode media.Backend
}

var backendMatrixModes = [...]backendMatrixMode{
	{"cpu-x265", media.BackendCPU, media.BackendCPU},
	{"cuda-x265", media.BackendCUDA, media.BackendCPU},
	{"cpu-nvenc", media.BackendCPU, media.BackendNVENC},
	{"cuda-nvenc", media.BackendCUDA, media.BackendNVENC},
}

var backendMatrixOrders = [...][4]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {1, 0, 3, 2}}

func (m backendMatrixMode) engine(base media.Engine) media.Engine {
	base.DecodeBackend, base.EncodeBackend = m.decode, m.encode
	return base
}

func TestBackendMatrixSchedule(t *testing.T) {
	for i, m := range backendMatrixModes {
		engine := m.engine(media.Default())
		spec, err := projectEncoderSpec(engine)
		if err != nil || spec.nvenc != (i >= 2) || (engine.DecodeBackend == media.BackendCUDA) != (i%2 == 1) {
			t.Fatalf("matrix mode %s: %+v %v", m.name, spec, err)
		}
	}
	for _, order := range backendMatrixOrders {
		sorted := order
		slices.Sort(sorted[:])
		if sorted != ([4]int{0, 1, 2, 3}) {
			t.Fatalf("invalid matrix order %v", order)
		}
	}
}

func matrixPreEncodeHashes(ctx context.Context, engine media.Engine, prepared Prepared) ([][32]byte, error) {
	plan := prepared.Plan
	sources := make(map[string]Source, len(prepared.Sources))
	for _, s := range prepared.Sources {
		sources[s.Asset.ID] = s
	}
	black := black444(plan.Contract.Canvas)
	frame := make([]byte, len(black))
	var indexes renderIndexes
	hashes := make([][32]byte, 0, len(plan.Spans))
	for _, span := range plan.Spans {
		h := sha256.New()
		w := &progressWriter{dst: h, unitBytes: int64(len(frame)), total: span.EndFrame - span.FirstFrame}
		if err := renderSpan(ctx, engine, &indexes, sources, plan, span, w, black, frame); err != nil {
			return nil, fmt.Errorf("pre-encode span [%d,%d): %w", span.Start, span.End, err)
		}
		if w.written != (span.EndFrame-span.FirstFrame)*int64(len(frame)) {
			return nil, errors.New("incorrect pre-encode byte count")
		}
		hashes = append(hashes, [32]byte(h.Sum(nil)))
	}
	if err := prepared.VerifySources(ctx); err != nil {
		return nil, err
	}
	return hashes, nil
}

func matrixVerifyProgress(events []Progress, plan Plan, mode backendMatrixMode) error {
	if err := checkExportProgress(events, plan); err != nil {
		return err
	}
	var cuda, nvenc bool
	for _, event := range events {
		for _, warning := range event.Warnings {
			cuda = cuda || strings.Contains(warning, "Experimental CUDA video decoding requested")
			nvenc = nvenc || strings.Contains(warning, "Experimental NVENC encoder requested")
		}
	}
	if cuda != (mode.decode == media.BackendCUDA) || nvenc != (mode.encode == media.BackendNVENC) {
		return fmt.Errorf("backend warnings do not match %s: CUDA=%t NVENC=%t", mode.name, cuda, nvenc)
	}
	return nil
}

func matrixVerifyOutput(t *testing.T, ctx context.Context, engine media.Engine, plan Plan, path string) {
	t.Helper()
	report, err := engine.Inspect(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyProjectOutput(plan, report); err != nil {
		t.Fatal(err)
	}
	if err := verifyPublishedTiming(ctx, engine, plan, path, report); err != nil {
		t.Fatal(err)
	}
}

func matrixCompareOutputs(t *testing.T, ctx context.Context, engine media.Engine, plan Plan, paths [4]string) {
	t.Helper()
	audio, err := encoderLongAudioHash(ctx, engine, paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(paths); i++ {
		got, err := encoderLongAudioHash(ctx, engine, paths[i])
		if err != nil || !bytes.Equal(audio, got) {
			t.Fatalf("decoded audio changed in %s: %v", backendMatrixModes[i].name, err)
		}
	}
	t.Logf("all four decoded audio hashes match: %s", bytes.TrimSpace(audio))
	for _, pair := range [][2]int{{0, 1}, {2, 3}, {0, 2}, {1, 3}} {
		q, err := encoderLongVideo(ctx, engine, plan, [2]string{paths[pair[0]], paths[pair[1]]})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("compare=%s/%s min_psnr_yuv=%v pooled_psnr_yuv=%v max_delta_yuv=%v", backendMatrixModes[pair[0]].name, backendMatrixModes[pair[1]].name, q.minPSNR, q.meanPSNR, q.maxDelta)
		if backendMatrixModes[pair[0]].encode == backendMatrixModes[pair[1]].encode && q.maxDelta != ([3]int{}) {
			t.Errorf("decoded pixels changed with decoding backend for the same encoder: %v", q.maxDelta)
		}
	}
}

func matrixRunCase(t *testing.T, ctx context.Context, base media.Engine, p project.Project, src, dir string, rounds int) {
	t.Helper()
	original, err := encoderLongHash(src)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := p.Fingerprint()
	prepared, err := Prepare(ctx, base, p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.Plan
	if !plan.Contract.Audio || plan.VideoFrames < 1 || plan.VideoFrames > 303 {
		t.Fatal("invalid matrix workload")
	}
	for _, m := range backendMatrixModes {
		if _, err := Prepare(ctx, m.engine(base), p, "hlg", 30); err != nil {
			t.Fatalf("%s preflight: %v", m.name, err)
		}
	}
	t.Logf("matrix canvas=%dx%d frames=%d audio_samples=%d source_sha256=%x", plan.Contract.Canvas.Width, plan.Contract.Canvas.Height, plan.VideoFrames, plan.OutputAudioFrames, original)
	cpu, err := matrixPreEncodeHashes(ctx, backendMatrixModes[0].engine(base), prepared)
	if err != nil {
		t.Fatal(err)
	}
	gpu, err := matrixPreEncodeHashes(ctx, backendMatrixModes[1].engine(base), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cpu, gpu) {
		t.Fatal("CUDA pre-encode composition differs from CPU")
	}
	t.Logf("pre-encode exact SHA256 parity: %d frames across %d spans", plan.VideoFrames, len(cpu))
	var paths [4]string
	var times [4][]time.Duration
	for round := 0; round < rounds; round++ {
		for _, index := range backendMatrixOrders[round] {
			mode := backendMatrixModes[index]
			engine := mode.engine(base)
			out := filepath.Join(dir, fmt.Sprintf("round-%d-%s.mp4", round+1, mode.name))
			runtime.GC()
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			peak := mem.HeapAlloc
			var resources *matrixResourceSession
			if rounds == 3 && os.Getenv("PIXELCUT_QUALIFY_GPU_RESOURCES") == "1" {
				var err error
				resources, err = startMatrixResources(ctx)
				if err != nil {
					t.Fatalf("resource baseline for %s: %v", mode.name, err)
				}
			}
			start := time.Now()
			last := start
			t.Logf("start=%s round=%d mode=%s", start.Format(time.RFC3339Nano), round+1, mode.name)
			var events []Progress
			renderErr := RenderProjectObserved(ctx, engine, p, "hlg", 30, out, func(v Progress) {
				events = append(events, v)
				if time.Since(last) >= 250*time.Millisecond || v.Stage == StageDone {
					runtime.ReadMemStats(&mem)
					peak = max(peak, mem.HeapAlloc)
					last = time.Now()
				}
			})
			elapsed := time.Since(start)
			if resources != nil {
				summary, err := resources.stop()
				t.Logf("resources round=%d mode=%s %s", round+1, mode.name, summary)
				if err != nil {
					t.Errorf("resource evidence incomplete for %s: %v", mode.name, err)
				}
			}
			if renderErr != nil {
				t.Fatalf("%s round %d: %v", mode.name, round+1, renderErr)
			}
			if err := matrixVerifyProgress(events, plan, mode); err != nil {
				t.Fatal(err)
			}
			matrixVerifyOutput(t, ctx, base, plan, out)
			info, err := os.Stat(out)
			if err != nil {
				t.Fatal(err)
			}
			times[index] = append(times[index], elapsed)
			t.Logf("end=%s round=%d mode=%s wall=%s fps=%.2f bytes=%d go_heap_sampled_peak=%d", start.Add(elapsed).Format(time.RFC3339Nano), round+1, mode.name, elapsed, float64(plan.VideoFrames)/elapsed.Seconds(), info.Size(), peak)
			requireEncoderNoStages(t, dir)
			if round == rounds-1 {
				paths[index] = out
			} else if err := os.Remove(out); err != nil {
				t.Fatal(err)
			}
		}
	}
	matrixCompareOutputs(t, ctx, base, plan, paths)
	for _, ts := range times {
		slices.Sort(ts)
	}
	for i, ts := range times {
		t.Logf("mode=%s samples=%v median_wall=%s speedup_vs_cpu=%.3f", backendMatrixModes[i].name, ts, ts[len(ts)/2], float64(times[0][len(times[0])/2])/float64(ts[len(ts)/2]))
	}
	after, err := encoderLongHash(src)
	if err != nil || !bytes.Equal(original, after) || p.Fingerprint() != fingerprint {
		t.Fatal("matrix source or project changed")
	}
	requireEncoderNoStages(t, dir)
}

func TestIntegratedGPUBackendMatrix(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_GPU_MATRIX") != "1" {
		t.Skip("physical integrated GPU matrix is opt-in")
	}
	if os.Getenv("PIXELCUT_QUALIFY_GPU_RESOURCES") == "1" && runtime.GOOS != "windows" {
		t.Fatal("resource qualification requires Windows")
	}
	base := encoderSafetyEngine(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	t.Logf("matrix pid=%d ffmpeg=%s ffprobe=%s public NVENC device=0 QP=16", os.Getpid(), base.FFmpeg, base.FFprobe)
	for _, tool := range []string{base.FFmpeg, base.FFprobe} {
		cmd := exec.CommandContext(ctx, tool, "-version")
		cmd.WaitDelay = time.Second
		var log errorTail
		cmd.Stdout, cmd.Stderr = &log, &log
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s", log.data)
	}
	t.Run("vfr-multilayer", func(t *testing.T) {
		dir := t.TempDir()
		p, src := cudaQualificationFixture(t, ctx, base, dir)
		matrixRunCase(t, ctx, base, p, src, dir, 1)
		combined := backendMatrixModes[3].engine(base)
		job, stop := context.WithCancel(ctx)
		defer stop()
		out := filepath.Join(dir, "cancelled.mp4")
		reached, done := false, false
		err := RenderProjectObserved(job, combined, p, "hlg", 30, out, func(v Progress) {
			done = done || v.Stage == StageDone
			if v.Stage == StageVideo && v.Completed > 0 {
				reached = true
				stop()
			}
		})
		if err == nil || !reached || done || ctx.Err() != nil {
			t.Fatalf("combined cancellation reached=%t done=%t err=%v outer=%v", reached, done, err, ctx.Err())
		}
		requireEncoderMissing(t, out)
		requireEncoderNoStages(t, dir)
		out = filepath.Join(dir, "recovered.mp4")
		if err := RenderProject(ctx, combined, p, "hlg", 30, out); err != nil {
			t.Fatalf("combined fresh export after cancellation: %v", err)
		}
		plan, err := Build(p, "hlg", 30)
		if err != nil {
			t.Fatal(err)
		}
		matrixVerifyOutput(t, ctx, base, plan, out)
		requireEncoderNoStages(t, dir)
		t.Log("combined cancellation and fresh verified export passed")
	})
	if t.Failed() {
		t.Fatal("synthetic matrix failed; do not benchmark real source")
	}
	t.Run("real-hlg", func(t *testing.T) {
		src := os.Getenv("PIXELCUT_GPU_MATRIX_SOURCE")
		if src == "" {
			t.Fatal("set PIXELCUT_GPU_MATRIX_SOURCE to a supported 10-60 second HEVC Main 10 HLG clip with audio")
		}
		src, err := filepath.Abs(src)
		if err != nil {
			t.Fatal(err)
		}
		report, err := base.Inspect(ctx, src)
		if err != nil {
			t.Fatal(err)
		}
		if err := report.ValidateProjectExportSource(); err != nil {
			t.Fatal(err)
		}
		a, err := project.FromReport("a", src, report)
		if err != nil || !a.HasAudio || a.Duration < 10000000 || a.Duration > 60000000 || a.Width > 4096 || a.Height > 4096 || int64(a.Width)*int64(a.Height) > MaxExportPixels {
			t.Fatalf("matrix requires bounded 10-60s HLG source with audio: %v", err)
		}
		p := project.New()
		p.Assets = []project.Asset{a}
		p.CanvasWidth, p.CanvasHeight = a.Width, a.Height
		p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: a.ID, Start: 100000, Out: 10000000}}
		matrixRunCase(t, ctx, base, p, src, t.TempDir(), 3)
	})
}
