package projectexport

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
)

const encoderMotionMaxBytes int64 = 1 << 30
const encoderMotionLosslessQP = -2

type encoderMotionCase struct {
	name   string
	canvas media.CanvasSize
	frames int
	source string
}

func (c encoderMotionCase) frameBytes() int {
	return c.canvas.Width * c.canvas.Height * 3
}

func (c encoderMotionCase) validate() error {
	w, h := c.canvas.Width, c.canvas.Height
	if c.canvas.Validate() != nil || w > 4096 || h > 4096 || w%2 != 0 || h%2 != 0 || int64(w)*int64(h) > MaxExportPixels || c.frames < 2 || c.frames > 120 || int64(c.frameBytes())*int64(c.frames) > encoderMotionMaxBytes {
		return errors.New("invalid or oversized encoder motion workload")
	}
	return nil
}

func encoderMotionArgs(c encoderMotionCase, raw, output string, qp, device int) ([]string, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	base := hlgVideoEncoderArgs(c.canvas, 30, output)
	if qp == encoderMotionLosslessQP {
		index := slices.Index(base, "-x265-params")
		if index < 0 || index+1 >= len(base) {
			return nil, errors.New("missing lossless control encoder options")
		}
		base[index+1] += ":lossless=1"
	} else if qp != -1 {
		if qp < 0 || qp > 51 {
			return nil, errors.New("invalid NVENC QP")
		}
		var err error
		base, err = nvencQualificationArgs(c.canvas, 30, device, output)
		if err != nil {
			return nil, err
		}
		base[slices.Index(base, "-qp")+1] = strconv.Itoa(qp)
	}
	codec := slices.Index(base, "-c:v")
	if codec < 0 {
		return nil, errors.New("missing encoder boundary")
	}
	args := []string{"-v", "error", "-xerror", "-nostdin", "-filter_threads", "1", "-f", "rawvideo", "-pixel_format", "yuv420p10le", "-video_size", fmt.Sprintf("%dx%d", c.canvas.Width, c.canvas.Height), "-framerate", "30", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-i", raw, "-map", "0:v:0", "-frames:v", strconv.Itoa(c.frames), "-vf", "setsar=1"}
	return append(args, base[codec:]...), nil
}

func prepareEncoderMotionReference(ctx context.Context, engine media.Engine, c encoderMotionCase, raw string) error {
	if err := c.validate(); err != nil {
		return err
	}
	input := []string{"-v", "error", "-xerror", "-nostdin", "-filter_threads", "1"}
	var filters []string
	if c.source == "" {
		input = append(input, "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=30,format=yuv444p10le", c.canvas.Width, c.canvas.Height))
	} else {
		identity, err := mediacache.Identify(c.source)
		if err != nil {
			return err
		}
		report, err := engine.Inspect(ctx, identity.Path)
		if err != nil {
			return err
		}
		if err := report.ValidateProjectExportSource(); err != nil {
			return err
		}
		v, err := report.Video()
		if err != nil {
			return err
		}
		if v.CodecName != "hevc" || v.Profile != "Main 10" {
			return errors.New("real encoder fixture requires HEVC Main 10")
		}
		display, err := v.DisplayGeometry()
		if err != nil || display.Width != c.canvas.Width || display.Height != c.canvas.Height {
			return errors.New("real fixture geometry changed")
		}
		rotation, err := v.DisplayRotation()
		if err != nil {
			return err
		}
		input = append(input, "-noautorotate", "-hwaccel", "none", "-c:v", "hevc", "-i", identity.Path)
		r := "limited"
		if v.Range == "pc" {
			r = "full"
		}
		filters = append(filters, fmt.Sprintf("zscale=rangein=%s:range=limited:chromalin=%s:matrixin=2020_ncl:matrix=2020_ncl:primariesin=2020:primaries=2020:transferin=arib-std-b67:transfer=arib-std-b67:filter=lanczos", r, v.ChromaLocation), "format=yuv444p10le")
		switch rotation {
		case -90:
			filters = append(filters, "transpose=clock")
		case 90:
			filters = append(filters, "transpose=cclock")
		case 180:
			filters = append(filters, "hflip", "vflip")
		}
	}
	shared := hlgVideoEncoderInput(c.canvas, 30)
	filters = append(filters, shared[slices.Index(shared, "-vf")+1])
	args := append(input, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", strings.Join(filters, ","), "-fps_mode", "passthrough", "-frames:v", strconv.Itoa(c.frames), "-c:v", "rawvideo", "-pix_fmt", "yuv420p10le", "-threads", "1", "-f", "rawvideo", "-n", raw)
	p := newExportProcess(ctx, engine.FFmpeg, args...)
	defer p.close()
	if err := p.cmd.Start(); err != nil {
		return err
	}
	if err := p.wait(); err != nil {
		return err
	}
	info, err := os.Stat(raw)
	if err != nil {
		return err
	}
	if info.Size() != int64(c.frameBytes())*int64(c.frames) {
		return errors.New("reference has incorrect raw frame count")
	}
	return ctx.Err()
}

type encoderMotionQuality struct {
	minPSNR  [3]float64
	meanPSNR [3]float64
	maxDelta [3]int
}

func motionPlaneError(reference, candidate []byte) (uint64, int, error) {
	if len(reference) == 0 || len(reference) != len(candidate) || len(reference)%2 != 0 {
		return 0, 0, errors.New("invalid motion plane extent")
	}
	var squared uint64
	var largest int
	for i := 0; i < len(reference); i += 2 {
		a, b := binary.LittleEndian.Uint16(reference[i:]), binary.LittleEndian.Uint16(candidate[i:])
		if a > 1023 || b > 1023 {
			return 0, 0, errors.New("motion sample exceeds 10-bit range")
		}
		d := absCode(a, b)
		squared += uint64(d * d)
		largest = max(largest, d)
	}
	return squared, largest, nil
}

func motionPSNR(squared, samples uint64) float64 {
	if squared == 0 {
		return 100
	}
	return min(100, 10*math.Log10(1023*1023/(float64(squared)/float64(samples))))
}

func verifyEncoderMotion(ctx context.Context, engine media.Engine, c encoderMotionCase, raw, output string) (encoderMotionQuality, error) {
	q := encoderMotionQuality{minPSNR: [3]float64{100, 100, 100}}
	plan := Plan{Contract: media.ExportContract{Color: "hlg", Canvas: c.canvas, FPS: 30}, VideoFrames: int64(c.frames)}
	report, err := engine.Inspect(ctx, output)
	if err != nil {
		return q, err
	}
	if err := verifyVideoOutput(plan, report); err != nil {
		return q, err
	}
	if err := verifyPublishedTiming(ctx, engine, plan, output, report); err != nil {
		return q, err
	}
	ref, err := os.Open(raw)
	if err != nil {
		return q, err
	}
	defer ref.Close()
	p := newExportProcess(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-hwaccel", "none", "-c:v", "hevc", "-i", output, "-map", "0:v:0", "-an", "-sn", "-dn", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
	defer p.close()
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return q, err
	}
	p.pipe = stdout
	if err := p.cmd.Start(); err != nil {
		return q, err
	}
	a, b := make([]byte, c.frameBytes()), make([]byte, c.frameBytes())
	y := c.canvas.Width * c.canvas.Height * 2
	bounds := [4]int{0, y, y + y/4, len(a)}
	var squared, samples [3]uint64
	for n := 0; n < c.frames; n++ {
		if err := ctx.Err(); err != nil {
			return q, err
		}
		if _, err := io.ReadFull(ref, a); err != nil {
			return q, err
		}
		if _, err := io.ReadFull(stdout, b); err != nil {
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
			q.maxDelta[j] = max(q.maxDelta[j], largest)
			q.minPSNR[j] = min(q.minPSNR[j], motionPSNR(d, count))
		}
	}
	var extra [1]byte
	for _, r := range []io.Reader{ref, stdout} {
		if n, err := r.Read(extra[:]); n != 0 || err != io.EOF {
			return q, errors.New("unexpected extra or incomplete encoder motion frame data")
		}
	}
	if err := p.wait(); err != nil {
		return q, err
	}
	for j := range q.meanPSNR {
		q.meanPSNR[j] = motionPSNR(squared[j], samples[j])
	}
	return q, ctx.Err()
}

type encoderMotionResult struct {
	qp      int
	quality encoderMotionQuality
	bytes   int64
	elapsed time.Duration
}

func measureEncoderMotion(ctx context.Context, engine media.Engine, c encoderMotionCase, raw, output string, qp, device int) (encoderMotionResult, error) {
	r := encoderMotionResult{qp: qp}
	args, err := encoderMotionArgs(c, raw, output, qp, device)
	if err != nil {
		return r, err
	}
	defer os.Remove(output)
	p := newExportProcess(ctx, engine.FFmpeg, args...)
	defer p.close()
	start := time.Now()
	if err := p.cmd.Start(); err != nil {
		return r, err
	}
	if err := p.wait(); err != nil {
		return r, err
	}
	r.elapsed = time.Since(start)
	r.quality, err = verifyEncoderMotion(ctx, engine, c, raw, output)
	if err != nil {
		return r, err
	}
	info, err := os.Stat(output)
	if err != nil {
		return r, err
	}
	r.bytes = info.Size()
	return r, nil
}

func comparableEncoderMotion(cpu, gpu encoderMotionQuality) bool {
	for j := range cpu.meanPSNR {
		if gpu.meanPSNR[j] < cpu.meanPSNR[j]-1 || gpu.minPSNR[j] < cpu.minPSNR[j]-1 {
			return false
		}
	}
	return true
}

func TestEncoderMotionMetric(t *testing.T) {
	a := []byte{0, 2, 64, 0}
	b := []byte{1, 2, 62, 0}
	d, largest, err := motionPlaneError(a, b)
	if err != nil || d != 5 || largest != 2 || motionPSNR(0, 2) != 100 || motionPSNR(d, 2) >= 100 {
		t.Fatalf("motion metric: %d %d %v", d, largest, err)
	}
	b[1] = 4
	if _, _, err := motionPlaneError(a, b); err == nil {
		t.Fatal("out-of-range sample accepted")
	}
	cpu := encoderMotionQuality{minPSNR: [3]float64{40, 40, 40}, meanPSNR: [3]float64{42, 42, 42}}
	gpu := cpu
	gpu.minPSNR[2] = 38.9
	if comparableEncoderMotion(cpu, gpu) || !comparableEncoderMotion(cpu, cpu) {
		t.Fatal("quality selection ignored a worst-frame chroma regression")
	}
	c := encoderMotionCase{canvas: media.CanvasSize{Width: 3840, Height: 2160}, frames: 120}
	if c.validate() == nil {
		t.Fatal("oversized reference accepted")
	}
	c.frames = 40
	args, err := encoderMotionArgs(c, "raw.yuv", "out.mp4", 20, 0)
	if err != nil || args[slices.Index(args, "-qp")+1] != "20" || args[slices.Index(args, "-pixel_format")+1] != "yuv420p10le" {
		t.Fatalf("motion encoder arguments: %v %v", args, err)
	}
}

func TestEncoderMotionInputColorContract(t *testing.T) {
	c := encoderMotionCase{canvas: media.CanvasSize{Width: 64, Height: 48}, frames: 12}
	for _, qp := range []int{encoderMotionLosslessQP, -1, 16} {
		args, err := encoderMotionArgs(c, "reference.yuv", "output.mp4", qp, 0)
		if err != nil {
			t.Fatal(err)
		}
		input := slices.Index(args, "-i")
		if input < 0 {
			t.Fatal("missing raw input")
		}
		for _, pair := range [][2]string{{"-color_primaries", "bt2020"}, {"-color_trc", "arib-std-b67"}, {"-colorspace", "bt2020nc"}, {"-color_range", "tv"}, {"-chroma_sample_location", "left"}} {
			index := slices.Index(args[:input], pair[0])
			if index < 0 || index+1 >= input || args[index+1] != pair[1] {
				t.Fatalf("QP %d missing input color option %v", qp, pair)
			}
		}
		if qp == encoderMotionLosslessQP {
			index := slices.Index(args, "-x265-params")
			if index < 0 || !strings.Contains(args[index+1], ":lossless=1") || !slices.Contains(args, "libx265") {
				t.Fatal("lossless CPU control not selected")
			}
		}
	}
}

func logEncoderMotion(t *testing.T, c encoderMotionCase, r encoderMotionResult) {
	t.Helper()
	t.Logf("case=%s qp=%d frames=%d bytes=%d container_kbps=%.1f wall=%s fps=%.2f min_psnr_yuv=%v aggregate_psnr_yuv=%v max_delta_yuv=%v", c.name, r.qp, c.frames, r.bytes, float64(r.bytes)*8*30/float64(c.frames)/1000, r.elapsed, float64(c.frames)/r.elapsed.Seconds(), r.quality.minPSNR, r.quality.meanPSNR, r.quality.maxDelta)
}

func qualifyEncoderMotion(t *testing.T, ctx context.Context, engine media.Engine, c encoderMotionCase, device int, physical bool) {
	t.Helper()
	dir := t.TempDir()
	raw, output := filepath.Join(dir, "reference.yuv"), filepath.Join(dir, "encoded.mp4")
	var identity mediacache.Source
	if c.source != "" {
		var err error
		identity, err = mediacache.Identify(c.source)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := prepareEncoderMotionReference(ctx, engine, c, raw); err != nil {
		t.Fatal(err)
	}
	if c.source != "" && !sameEncoderMotionSource(c.source, identity) {
		t.Fatal("source changed during reference generation")
	}
	control, err := measureEncoderMotion(ctx, engine, c, raw, output, encoderMotionLosslessQP, device)
	if err != nil {
		t.Fatalf("lossless reference control: %v", err)
	}
	logEncoderMotion(t, c, control)
	if control.quality.maxDelta != ([3]int{}) || control.quality.minPSNR != ([3]float64{100, 100, 100}) {
		t.Fatalf("lossless reference control changed samples: %+v", control.quality)
	}
	cpu, err := measureEncoderMotion(ctx, engine, c, raw, output, -1, device)
	if err != nil {
		t.Fatal(err)
	}
	logEncoderMotion(t, c, cpu)
	if !physical {
		return
	}
	var chosen *encoderMotionResult
	for _, qp := range []int{12, 16, 20, 24} {
		r, err := measureEncoderMotion(ctx, engine, c, raw, output, qp, device)
		if err != nil {
			t.Fatal(err)
		}
		logEncoderMotion(t, c, r)
		if comparableEncoderMotion(cpu.quality, r.quality) && (chosen == nil || r.bytes < chosen.bytes) {
			chosen = &r
		}
	}
	if chosen == nil {
		t.Error("no NVENC QP met the provisional comparative quality screen")
		return
	}
	var cpuTimes, gpuTimes []time.Duration
	for round := 0; round < 3; round++ {
		order := [2]int{-1, chosen.qp}
		if round%2 == 1 {
			order[0], order[1] = order[1], order[0]
		}
		for _, qp := range order {
			r, err := measureEncoderMotion(ctx, engine, c, raw, output, qp, device)
			if err != nil {
				t.Fatal(err)
			}
			logEncoderMotion(t, c, r)
			if qp == -1 {
				cpuTimes = append(cpuTimes, r.elapsed)
				if !comparableEncoderMotion(cpu.quality, r.quality) || !comparableEncoderMotion(r.quality, cpu.quality) || !comparableEncoderMotion(r.quality, chosen.quality) {
					t.Error("CPU timing run invalidated the reference quality match")
				}
			} else {
				gpuTimes = append(gpuTimes, r.elapsed)
				if !comparableEncoderMotion(cpu.quality, r.quality) {
					t.Error("timed NVENC run failed comparative quality screen")
				}
			}
		}
	}
	if c.source != "" && !sameEncoderMotionSource(c.source, identity) {
		t.Fatal("source changed during encoder comparison")
	}
	slices.Sort(cpuTimes)
	slices.Sort(gpuTimes)
	t.Logf("case=%s chosen_qp=%d cpu_wall=%v gpu_wall=%v median_speedup=%.3f sweep_size_ratio=%.3f", c.name, chosen.qp, cpuTimes, gpuTimes, float64(cpuTimes[1])/float64(gpuTimes[1]), float64(chosen.bytes)/float64(cpu.bytes))
}

func sameEncoderMotionSource(path string, before mediacache.Source) bool {
	now, err := mediacache.Identify(path)
	return err == nil && now == before
}

func TestHLGEncoderMotionQualification(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_NVENC_MOTION") == "1"
	engine := media.Default()
	for _, tool := range []string{engine.FFmpeg, engine.FFprobe} {
		if _, err := exec.LookPath(tool); err != nil {
			if physical {
				t.Fatal(err)
			}
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	t.Run("cpu-smoke", func(t *testing.T) {
		qualifyEncoderMotion(t, ctx, engine, encoderMotionCase{name: "smoke", canvas: media.CanvasSize{Width: 64, Height: 48}, frames: 12}, 0, false)
	})
	if !physical {
		return
	}
	device := 0
	if value := os.Getenv("PIXELCUT_NVENC_DEVICE"); value != "" {
		var err error
		device, err = strconv.Atoi(value)
		if err != nil || device < 0 || device > 31 {
			t.Fatal("invalid PIXELCUT_NVENC_DEVICE")
		}
	}
	for _, tool := range []*string{&engine.FFmpeg, &engine.FFprobe} {
		path, err := exec.LookPath(*tool)
		if err != nil {
			t.Fatal(err)
		}
		*tool, err = filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		version := exec.CommandContext(ctx, *tool, "-version")
		version.WaitDelay = time.Second
		var log errorTail
		version.Stdout, version.Stderr = &log, &log
		if err := version.Run(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s\n%s", *tool, log.data)
	}
	t.Logf("NVENC device=%d", device)
	cases := []encoderMotionCase{
		{name: "motion-360p", canvas: media.CanvasSize{Width: 640, Height: 360}, frames: 120},
		{name: "motion-1080p", canvas: media.CanvasSize{Width: 1920, Height: 1080}, frames: 90},
		{name: "motion-4k", canvas: media.CanvasSize{Width: 3840, Height: 2160}, frames: 40},
	}
	if source := os.Getenv("PIXELCUT_NVENC_MOTION_SOURCE"); source != "" {
		report, err := engine.Inspect(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		if err := report.ValidateProjectExportSource(); err != nil {
			t.Fatal(err)
		}
		v, err := report.Video()
		if err != nil {
			t.Fatal(err)
		}
		v, err = v.DisplayGeometry()
		if err != nil {
			t.Fatal(err)
		}
		c := encoderMotionCase{name: "real-hlg", canvas: media.CanvasSize{Width: v.Width, Height: v.Height}, frames: 60, source: source}
		if v.Width > 4096 || v.Height > 4096 || v.Width < 2 || v.Height < 2 {
			t.Fatal("unsupported real source dimensions")
		}
		c.frames = min(c.frames, int(encoderMotionMaxBytes/int64(c.frameBytes())))
		if err := c.validate(); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			qualifyEncoderMotion(t, ctx, engine, c, device, true)
		})
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
	}
}
