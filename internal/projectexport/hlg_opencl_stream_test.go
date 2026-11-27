package projectexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

const openCLStripHeight = 120
const openCLStreamLogicalFrames = 2

type openCLStripBatch struct {
	canvas media.CanvasSize
	cases  []hlgQualificationFixture
	count  int
}

func newOpenCLStripBatch(width, height, opacity int) (openCLStripBatch, error) {
	if width < 2 || width > 4096 || width%2 != 0 || height < openCLStripHeight || height > 2160 || height%openCLStripHeight != 0 || (opacity != 50 && opacity != 100) {
		return openCLStripBatch{}, errors.New("unsupported strip workload")
	}
	c := media.CanvasSize{Width: width, Height: openCLStripHeight}
	full := media.Placement{Width: c.Width, Height: c.Height}
	inset := media.Placement{DestX: 3, DestY: 5, Width: c.Width - 6, Height: c.Height - 10}
	if inset.Width <= 0 {
		return openCLStripBatch{}, errors.New("strip too narrow")
	}
	b := openCLStripBatch{canvas: c, count: height / c.Height * openCLStreamLogicalFrames}
	for i := 0; i < 2; i++ {
		b.cases = append(b.cases, hlgQualificationFixture{
			name:   fmt.Sprintf("strip-%d", i),
			canvas: c,
			layers: []layerFrame{qualificationPattern(full, 100, i+1), qualificationPattern(inset, opacity, i+3)},
		})
	}
	return b, nil
}

type openCLStripInput struct {
	ctx         context.Context
	batch       openCLStripBatch
	atlas       []byte
	index, used int
}

func (r *openCLStripInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.used == len(r.atlas) {
		if r.index == r.batch.count {
			return 0, io.EOF
		}
		f := r.batch.cases[r.index%len(r.batch.cases)]
		atlas, err := packHLGOpenCLAtlasInto(f.canvas, f.layers, r.atlas)
		if err != nil {
			return 0, err
		}
		r.atlas, r.used = atlas, 0
		r.index++
	}
	n := copy(p, r.atlas[r.used:])
	r.used += n
	return n, nil
}

type openCLStripOutput struct {
	ctx         context.Context
	batch       openCLStripBatch
	packed, raw []byte
	used, index int
	check       func(int, []byte) error
	first       time.Time
	digests     [][32]byte
}

func (w *openCLStripOutput) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	total := 0
	for len(p) > 0 {
		if w.index == w.batch.count {
			return total, errors.New("extra OpenCL strip")
		}
		n := copy(w.packed[w.used:], p)
		total += n
		w.used += n
		p = p[n:]
		if w.used != len(w.packed) {
			continue
		}
		raw, err := unpackHLGOpenCLInto(w.batch.canvas, w.packed, w.raw, uint16(100+w.index))
		if err != nil {
			return total, err
		}
		w.raw = raw
		if w.index == 0 {
			w.first = time.Now()
		}
		w.digests = append(w.digests, sha256.Sum256(raw))
		if w.check != nil {
			if err := w.check(w.index, raw); err != nil {
				return total, err
			}
		}
		w.index++
		w.used = 0
	}
	return total, nil
}

type openCLStripTiming struct {
	total, first time.Duration
	digests      [][32]byte
}

func runOpenCLStripBatch(ctx context.Context, ffmpeg, dir, device string, batch openCLStripBatch, check func(int, []byte) error) (openCLStripTiming, string, error) {
	var result openCLStripTiming
	if err := ctx.Err(); err != nil {
		return result, "", err
	}
	args, err := openCLCompositeBatchArgs(batch.canvas, device, batch.count)
	if err != nil {
		return result, "", err
	}
	if len(batch.cases) == 0 {
		return result, "", errors.New("empty OpenCL workload")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	input := &openCLStripInput{ctx: ctx, batch: batch}
	output := &openCLStripOutput{ctx: ctx, batch: batch, packed: make([]byte, batch.canvas.Width*batch.canvas.Height*8), check: check}
	log := openCLCapture{limit: 128 * 1024}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, output, &log
	start := time.Now()
	err = cmd.Run()
	result.total = time.Since(start)
	if !output.first.IsZero() {
		result.first = output.first.Sub(start)
	}
	if ctx.Err() != nil {
		return result, string(log.data), ctx.Err()
	}
	if err != nil {
		return result, string(log.data), fmt.Errorf("OpenCL strip batch: %w", err)
	}
	if input.index != batch.count || input.used != len(input.atlas) || output.index != batch.count || output.used != 0 || log.overflow {
		return result, string(log.data), errors.New("incomplete OpenCL strip batch or diagnostics")
	}
	result.digests = output.digests
	return result, string(log.data), nil
}

func TestHLGOpenCLStripProtocol(t *testing.T) {
	batch, err := newOpenCLStripBatch(8, 120, 50)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := &openCLStripInput{ctx: ctx, batch: batch}
	size := batch.canvas.Width * batch.canvas.Height * 8 * MaxVideoLayers
	first := make([]byte, size)
	second := make([]byte, size)
	if _, err := io.ReadFull(input, first); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(input, second); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("strip inputs did not change")
	}
	if n, err := input.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("input termination: %d %v", n, err)
	}
	outSize := size / MaxVideoLayers
	frame := make([]byte, outSize)
	for i := 0; i < len(frame); i += 8 {
		binary.LittleEndian.PutUint16(frame[i:], 64)
		binary.LittleEndian.PutUint16(frame[i+2:], 512)
		binary.LittleEndian.PutUint16(frame[i+4:], 512)
		binary.LittleEndian.PutUint16(frame[i+6:], 100)
	}
	output := &openCLStripOutput{ctx: ctx, batch: batch, packed: make([]byte, outSize)}
	if _, err := output.Write(frame[:13]); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write(frame[13:]); err != nil || output.index != 1 {
		t.Fatalf("fragmented frame: %v", err)
	}
	if _, err := output.Write(frame); err == nil {
		t.Fatal("stale strip marker accepted")
	}
	output = &openCLStripOutput{ctx: ctx, batch: batch, packed: make([]byte, outSize)}
	if _, err := output.Write(frame); err != nil {
		t.Fatal(err)
	}
	for i := 6; i < len(frame); i += 8 {
		binary.LittleEndian.PutUint16(frame[i:], 101)
	}
	if _, err := output.Write(frame); err != nil || output.index != batch.count {
		t.Fatalf("second strip: %v", err)
	}
	if _, err := output.Write(frame); err == nil {
		t.Fatal("extra strip accepted")
	}
	cancel()
	if _, err := input.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled input: %v", err)
	}
	if _, _, err := runOpenCLStripBatch(ctx, "not-a-real-executable", "", "0.0", batch, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled batch started: %v", err)
	}
	if _, err := newOpenCLStripBatch(3840, 2159, 50); err == nil {
		t.Fatal("incomplete strip accepted")
	}
	if _, err := openCLCompositeBatchArgs(batch.canvas, "0.0", 121); err == nil {
		t.Fatal("unbounded batch accepted")
	}
}

func cpuOpenCLStripTiming(ctx context.Context, batch openCLStripBatch) (time.Duration, error) {
	dst := black444(batch.canvas)
	start := time.Now()
	for i := 0; i < batch.count; i++ {
		if err := compositeHLG(ctx, dst, batch.canvas, batch.cases[i%len(batch.cases)].layers); err != nil {
			return 0, err
		}
		_ = sha256.Sum256(dst)
	}
	return time.Since(start), nil
}

func medianOpenCLDuration(values []time.Duration) time.Duration {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}

func TestHLGOpenCLStreamQualification(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_OPENCL_STREAM") != "1" {
		t.Skip("physical OpenCL stream qualification is opt-in")
	}
	device := os.Getenv("PIXELCUT_OPENCL_DEVICE")
	if device == "" {
		device = "0.0"
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffmpeg, err = filepath.Abs(ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hlg.cl"), []byte(hlgOpenCLKernel), 0600); err != nil {
		t.Fatal(err)
	}
	version := exec.CommandContext(ctx, ffmpeg, "-version")
	version.WaitDelay = time.Second
	identity := openCLCapture{limit: 128 * 1024}
	version.Stdout, version.Stderr = &identity, &identity
	if err := version.Run(); err != nil || identity.overflow {
		t.Fatalf("FFmpeg identity: %v", err)
	}
	t.Logf("FFmpeg %s\n%s\nOpenCL device %s", ffmpeg, identity.data, device)
	for _, size := range []media.CanvasSize{{Width: 640, Height: 360}, {Width: 1920, Height: 1080}, {Width: 3840, Height: 2160}} {
		for _, opacity := range []int{100, 50} {
			t.Run(fmt.Sprintf("%dx%d/opacity%d", size.Width, size.Height, opacity), func(t *testing.T) {
				batch, err := newOpenCLStripBatch(size.Width, size.Height, opacity)
				if err != nil {
					t.Fatal(err)
				}
				refs := make([][]byte, len(batch.cases))
				for i, f := range batch.cases {
					refs[i] = black444(f.canvas)
					if err := compositeHLG(ctx, refs[i], f.canvas, f.layers); err != nil {
						t.Fatal(err)
					}
				}
				maxDelta, exact, mixed := 0, 0, 0
				verify, log, err := runOpenCLStripBatch(ctx, ffmpeg, dir, device, batch, func(i int, raw []byte) error {
					j := i % len(batch.cases)
					q, err := compareHLGQualification(batch.cases[j], refs[j], raw)
					maxDelta = max(maxDelta, q.maxMixedDelta)
					exact += q.exactPixels
					mixed += q.mixedPixels
					return err
				})
				if err != nil {
					t.Fatalf("fidelity: %v\n%s", err, log)
				}
				if exact+mixed != size.Width*size.Height*openCLStreamLogicalFrames {
					t.Fatal("coverage count mismatch")
				}
				t.Logf("device log:\n%s", log)
				var cpu, gpu, first []time.Duration
				for round := 0; round < 3; round++ {
					for _, hardware := range []bool{round%2 == 1, round%2 != 1} {
						if hardware {
							r, log, err := runOpenCLStripBatch(ctx, ffmpeg, dir, device, batch, nil)
							if err != nil || !slices.Equal(r.digests, verify.digests) {
								t.Fatalf("timed GPU batch not repeatable: %v\n%s", err, log)
							}
							gpu, first = append(gpu, r.total), append(first, r.first)
						} else {
							d, err := cpuOpenCLStripTiming(ctx, batch)
							if err != nil {
								t.Fatal(err)
							}
							cpu = append(cpu, d)
						}
					}
				}
				t.Logf("logical_frames=%d strips=%d exact=%d mixed=%d max_mixed_delta=%d cpu=%v gpu=%v first_strip=%v cpu_median=%s gpu_median=%s atlas_bytes=%d output_bytes=%d", openCLStreamLogicalFrames, batch.count, exact, mixed, maxDelta, cpu, gpu, first, medianOpenCLDuration(cpu), medianOpenCLDuration(gpu), batch.canvas.Width*batch.canvas.Height*8*MaxVideoLayers, batch.canvas.Width*batch.canvas.Height*8)
			})
		}
	}
	t.Run("cancel-and-restart", func(t *testing.T) {
		batch, err := newOpenCLStripBatch(640, 360, 50)
		if err != nil {
			t.Fatal(err)
		}
		child, stop := context.WithCancel(ctx)
		defer stop()
		seen := false
		_, log, err := runOpenCLStripBatch(child, ffmpeg, dir, device, batch, func(_ int, _ []byte) error {
			seen = true
			stop()
			return context.Canceled
		})
		if !seen || !errors.Is(err, context.Canceled) {
			t.Fatalf("active cancellation: %v\n%s", err, log)
		}
		r, log, err := runOpenCLStripBatch(ctx, ffmpeg, dir, device, batch, nil)
		if err != nil || len(r.digests) != batch.count {
			t.Fatalf("restart: %v\n%s", err, log)
		}
	})
}
