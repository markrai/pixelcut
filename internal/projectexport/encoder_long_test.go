package projectexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func encoderLongHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func encoderLongVideo(ctx context.Context, engine media.Engine, plan Plan, paths [2]string) (encoderMotionQuality, error) {
	q := encoderMotionQuality{minPSNR: [3]float64{100, 100, 100}}
	var readers [2]io.ReadCloser
	var processes [2]*exportProcess
	for i, path := range paths {
		p := newExportProcess(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-hwaccel", "none", "-c:v", "hevc", "-i", path, "-map", "0:v:0", "-an", "-sn", "-dn", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
		defer p.close()
		r, err := p.cmd.StdoutPipe()
		if err != nil {
			return q, err
		}
		p.pipe = r
		if err := p.cmd.Start(); err != nil {
			return q, err
		}
		readers[i], processes[i] = r, p
	}
	y := plan.Contract.Canvas.Width * plan.Contract.Canvas.Height * 2
	bounds := [4]int{0, y, y + y/4, y + y/2}
	a, b := make([]byte, bounds[3]), make([]byte, bounds[3])
	var squared, samples [3]uint64
	for n := int64(0); n < plan.VideoFrames; n++ {
		if err := ctx.Err(); err != nil {
			return q, err
		}
		if _, err := io.ReadFull(readers[0], a); err != nil {
			return q, err
		}
		if _, err := io.ReadFull(readers[1], b); err != nil {
			return q, err
		}
		for j := 0; j < 3; j++ {
			d, largest, err := motionPlaneError(a[bounds[j]:bounds[j+1]], b[bounds[j]:bounds[j+1]])
			if err != nil {
				return q, err
			}
			count := uint64((bounds[j+1] - bounds[j]) / 2)
			squared[j] += d
			samples[j] += count
			q.minPSNR[j] = min(q.minPSNR[j], motionPSNR(d, count))
			q.maxDelta[j] = max(q.maxDelta[j], largest)
		}
	}
	var extra [1]byte
	for i, r := range readers {
		if n, err := r.Read(extra[:]); n != 0 || err != io.EOF {
			return q, errors.New("extra or incomplete long-export video")
		}
		if err := processes[i].wait(); err != nil {
			return q, err
		}
	}
	for j := range q.meanPSNR {
		q.meanPSNR[j] = motionPSNR(squared[j], samples[j])
	}
	return q, ctx.Err()
}

func encoderLongAudioHash(ctx context.Context, engine media.Engine, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-i", path, "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "hash", "-hash", "sha256", "-")
	cmd.WaitDelay = 2 * time.Second
	return cmd.Output()
}

func TestProjectNVENCLongQualification(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_NVENC_LONG") != "1" {
		t.Skip("physical long NVENC project qualification is opt-in")
	}
	engine := encoderSafetyEngine(t, true)
	spec := encoderSafetySpec(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	dir := t.TempDir()
	src := os.Getenv("PIXELCUT_NVENC_LONG_SOURCE")
	if src == "" {
		t.Fatal("set PIXELCUT_NVENC_LONG_SOURCE to a 10-60 second supported HLG clip with audio")
	}
	src, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateProjectExportSource(); err != nil {
		t.Fatal(err)
	}
	a, err := project.FromReport("a", src, report)
	if err != nil || !a.HasAudio || a.Duration < 10000000 || a.Duration > 60000000 || a.Width > 4096 || a.Height > 4096 || int64(a.Width)*int64(a.Height) > MaxExportPixels {
		t.Fatalf("long fixture requires bounded 10-60s HLG video with audio: %v", err)
	}
	original, err := encoderLongHash(src)
	if err != nil {
		t.Fatal(err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.CanvasWidth, p.CanvasHeight = a.Width, a.Height
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: a.ID, Start: 100000, Out: 10000000}}
	plan, err := Build(p, "hlg", 30)
	if err != nil || !plan.Contract.Audio || plan.VideoFrames != 303 {
		t.Fatalf("unexpected long project plan: %v", err)
	}
	t.Logf("long project pid=%d ffmpeg=%s ffprobe=%s device=%d qp=%d canvas=%dx%d frames=%d audio_samples=%d source_sha256=%x", os.Getpid(), engine.FFmpeg, engine.FFprobe, spec.device, spec.qp, a.Width, a.Height, plan.VideoFrames, plan.OutputAudioFrames, original)
	for _, tool := range []string{engine.FFmpeg, engine.FFprobe} {
		cmd := exec.CommandContext(ctx, tool, "-version")
		cmd.WaitDelay = time.Second
		var log errorTail
		cmd.Stdout, cmd.Stderr = &log, &log
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s", log.data)
	}
	var times [2][]time.Duration
	var paths [2]string
	for round := 0; round < 3; round++ {
		order := [2]int{0, 1}
		if round%2 == 1 {
			order[0], order[1] = order[1], order[0]
		}
		for _, index := range order {
			selected := videoEncoderSpec{}
			if index == 1 {
				selected = spec
			}
			out := filepath.Join(dir, fmt.Sprintf("round-%d-%d.mp4", round, index))
			runtime.GC()
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			baseHeap, peakHeap := mem.HeapAlloc, mem.HeapAlloc
			last := time.Now()
			start := last
			var events []Progress
			err := renderProjectWithEncoder(ctx, engine, p, "hlg", 30, true, false, out, func(v Progress) {
				events = append(events, v)
				if time.Since(last) >= 250*time.Millisecond || v.Stage == StageDone {
					runtime.ReadMemStats(&mem)
					peakHeap = max(peakHeap, mem.HeapAlloc)
					last = time.Now()
				}
			}, selected)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkExportProgress(events, plan); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(out)
			if err != nil {
				t.Fatal(err)
			}
			times[index] = append(times[index], elapsed)
			t.Logf("round=%d backend=%s wall=%s fps=%.2f bytes=%d go_heap_start=%d go_heap_sampled_peak=%d", round+1, selected.name(), elapsed, float64(plan.VideoFrames)/elapsed.Seconds(), info.Size(), baseHeap, peakHeap)
			if round == 2 {
				paths[index] = out
			} else if err := os.Remove(out); err != nil {
				t.Fatal(err)
			}
			requireEncoderNoStages(t, dir)
		}
	}
	q, err := encoderLongVideo(ctx, engine, plan, paths)
	if err != nil {
		t.Fatal(err)
	}
	h0, err := encoderLongAudioHash(ctx, engine, paths[0])
	if err != nil {
		t.Fatal(err)
	}
	h1, err := encoderLongAudioHash(ctx, engine, paths[1])
	if err != nil || !bytes.Equal(h0, h1) {
		t.Fatalf("long export audio differed: %v", err)
	}
	after, err := encoderLongHash(src)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("long source changed")
	}
	for _, ts := range times {
		slices.Sort(ts)
	}
	t.Logf("long project cpu_wall=%v gpu_wall=%v median_speedup=%.3f decoded_audio_hash=%s cpu_vs_gpu_min_psnr_yuv=%v pooled_psnr_yuv=%v max_delta_yuv=%v", times[0], times[1], float64(times[0][1])/float64(times[1][1]), bytes.TrimSpace(h0), q.minPSNR, q.meanPSNR, q.maxDelta)
	requireEncoderNoStages(t, dir)
}
