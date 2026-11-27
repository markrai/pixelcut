package projectexport

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func encoderSafetyEngine(t *testing.T, physical bool) media.Engine {
	t.Helper()
	engine := media.Default()
	for _, tool := range []*string{&engine.FFmpeg, &engine.FFprobe} {
		path, err := exec.LookPath(*tool)
		if err != nil {
			if physical {
				t.Fatal(err)
			}
			t.Skip(err)
		}
		*tool, err = filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	return engine
}

func encoderSafetyOption(t *testing.T, key string, fallback, limit int) int {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > limit {
		t.Fatalf("invalid %s", key)
	}
	return n
}

func encoderSafetySpec(t *testing.T) videoEncoderSpec {
	t.Helper()
	return videoEncoderSpec{nvenc: true, device: encoderSafetyOption(t, "PIXELCUT_NVENC_DEVICE", 0, 31), qp: encoderSafetyOption(t, "PIXELCUT_NVENC_PROJECT_QP", 16, 51)}
}

func encoderSafetyFixture(t *testing.T, ctx context.Context, engine media.Engine, dir string) (project.Project, string, []byte) {
	t.Helper()
	src := filepath.Join(dir, "source.mp4")
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=30:d=0.5,format=yuv420p10le,geq=lum='180+20*N':cb=512:cr=512,setsar=1", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=0.5", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
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
	if err != nil || !a.HasAudio {
		t.Fatalf("fixture import: %v", err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.CanvasWidth, p.CanvasHeight = a.Width, a.Height
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: a.ID, Out: 400000}}
	return p, src, original
}

func requireEncoderNoStages(t *testing.T, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ".pixelcut-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("staged output leaked: %v %v", files, err)
	}
}

func requireEncoderMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected output at %s: %v", path, err)
	}
}

func TestProjectEncoderRuntimeSafety(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_NVENC_SAFETY") == "1"
	engine := encoderSafetyEngine(t, physical)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	p, src, original := encoderSafetyFixture(t, ctx, engine, t.TempDir())
	specs := []videoEncoderSpec{{}}
	if physical {
		specs = append(specs, encoderSafetySpec(t))
	}
	for _, spec := range specs {
		t.Run(spec.name(), func(t *testing.T) {
			render := func(ctx context.Context, output string, observe Observer) error {
				return renderProjectWithEncoder(ctx, engine, p, "hlg", 30, true, false, output, observe, spec)
			}
			for _, stage := range []Stage{StagePreflight, StageVideo, StageVideoWait, StageAudio, StageAudioWait, StageVerify, StagePublish} {
				t.Run("cancel-"+string(stage), func(t *testing.T) {
					dir := t.TempDir()
					out := filepath.Join(dir, "cancelled.mp4")
					job, stop := context.WithCancel(ctx)
					defer stop()
					reached, done := false, false
					start := time.Now()
					err := render(job, out, func(v Progress) {
						done = done || v.Stage == StageDone
						if v.Stage == stage && ((stage != StageVideo && stage != StageAudio) || v.Completed > 0) {
							reached = true
							stop()
						}
					})
					if err == nil || !reached || done || ctx.Err() != nil {
						t.Fatalf("cancellation stage=%s reached=%t done=%t err=%v outer=%v", stage, reached, done, err, ctx.Err())
					}
					requireEncoderMissing(t, out)
					requireEncoderNoStages(t, dir)
					t.Logf("cancelled at %s; elapsed=%s error=%v", stage, time.Since(start), err)
				})
			}
			t.Run("existing-and-late-destination", func(t *testing.T) {
				dir := t.TempDir()
				out := filepath.Join(dir, "existing.mp4")
				sentinel := []byte("keep original destination")
				if err := os.WriteFile(out, sentinel, 0600); err != nil {
					t.Fatal(err)
				}
				if err := render(ctx, out, nil); err == nil {
					t.Fatal("existing output accepted")
				}
				if got, err := os.ReadFile(out); err != nil || !bytes.Equal(got, sentinel) {
					t.Fatal("existing output changed")
				}
				if err := os.Remove(out); err != nil {
					t.Fatal(err)
				}
				var inject error
				reached, done := false, false
				err := render(ctx, out, func(v Progress) {
					done = done || v.Stage == StageDone
					if v.Stage == StagePublish {
						reached = true
						inject = os.WriteFile(out, sentinel, 0600)
					}
				})
				if err == nil || inject != nil || !reached || done {
					t.Fatalf("late collision: %v injection=%v reached=%t done=%t", err, inject, reached, done)
				}
				if got, err := os.ReadFile(out); err != nil || !bytes.Equal(got, sentinel) {
					t.Fatal("late destination changed")
				}
				requireEncoderNoStages(t, dir)
			})
			t.Run("corrupt-staged-video", func(t *testing.T) {
				dir := t.TempDir()
				out := filepath.Join(dir, "corrupt.mp4")
				reached, done := false, false
				var inject error
				err := render(ctx, out, func(v Progress) {
					done = done || v.Stage == StageDone
					if v.Stage != StageVerify || reached {
						return
					}
					reached = true
					files, err := filepath.Glob(filepath.Join(dir, ".pixelcut-*"))
					if err != nil || len(files) != 1 {
						inject = fmt.Errorf("expected one staged video, found %v: %v", files, err)
						return
					}
					inject = os.Truncate(files[0], 0)
				})
				if err == nil || inject != nil || !reached || done {
					t.Fatalf("corrupted stage: %v injection=%v reached=%t done=%t", err, inject, reached, done)
				}
				requireEncoderMissing(t, out)
				requireEncoderNoStages(t, dir)
			})
			t.Run("source-mutation", func(t *testing.T) {
				dir := t.TempDir()
				out := filepath.Join(dir, "changed.mp4")
				copyPath := filepath.Join(dir, "copy.mp4")
				if err := os.WriteFile(copyPath, original, 0600); err != nil {
					t.Fatal(err)
				}
				q := p.Clone()
				q.Assets[0].Path = copyPath
				reached, done := false, false
				var inject error
				err := renderProjectWithEncoder(ctx, engine, q, "hlg", 30, true, false, out, func(v Progress) {
					done = done || v.Stage == StageDone
					if v.Stage == StagePublish {
						reached = true
						inject = os.WriteFile(copyPath, append(append([]byte(nil), original...), 0), 0600)
					}
				}, spec)
				if err == nil || inject != nil || !reached || done || !strings.Contains(err.Error(), "changed") {
					t.Fatalf("source mutation: %v injection=%v reached=%t done=%t", err, inject, reached, done)
				}
				requireEncoderMissing(t, out)
				requireEncoderNoStages(t, dir)
			})
			if spec.nvenc {
				t.Run("unavailable-device", func(t *testing.T) {
					bad := spec
					bad.device = encoderSafetyOption(t, "PIXELCUT_NVENC_UNAVAILABLE_DEVICE", 31, 31)
					if bad.device == spec.device {
						t.Fatal("unavailable device must differ from qualified device")
					}
					dir := t.TempDir()
					out := filepath.Join(dir, "unavailable.mp4")
					done := false
					err := renderProjectWithEncoder(ctx, engine, p, "hlg", 30, true, false, out, func(v Progress) {
						done = done || v.Stage == StageDone
					}, bad)
					if err == nil || done || ctx.Err() != nil || !strings.Contains(err.Error(), "NVENC") {
						t.Fatalf("unavailable device unexpectedly accepted or wrong error: %v done=%t", err, done)
					}
					t.Logf("unavailable device=%d error=%v", bad.device, err)
					requireEncoderMissing(t, out)
					requireEncoderNoStages(t, dir)
				})
			}
			t.Run("fresh-export-after-faults", func(t *testing.T) {
				dir := t.TempDir()
				out := filepath.Join(dir, "recovered.mp4")
				if err := render(ctx, out, nil); err != nil {
					t.Fatal(err)
				}
				report, err := engine.Inspect(ctx, out)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := Build(p, "hlg", 30)
				if err != nil {
					t.Fatal(err)
				}
				if err := verifyProjectOutput(plan, report); err != nil {
					t.Fatal(err)
				}
				if err := verifyPublishedTiming(ctx, engine, plan, out, report); err != nil {
					t.Fatal(err)
				}
				requireEncoderNoStages(t, dir)
			})
		})
	}
	if after, err := os.ReadFile(src); err != nil || !bytes.Equal(after, original) {
		t.Fatal("original fixture changed")
	}
}
