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
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

const encoderQualificationFrames = 12

func nvencQualificationArgs(c media.CanvasSize, fps, device int, name string) ([]string, error) {
	contract := media.ExportContract{Color: "hlg", Canvas: c, FPS: fps}
	if contract.Validate() != nil || c.Width > 4096 || c.Height > 4096 || c.Width%2 != 0 || c.Height%2 != 0 || int64(c.Width)*int64(c.Height) > MaxExportPixels || device < 0 || device > 31 {
		return nil, errors.New("invalid NVENC qualification contract or device")
	}
	args := hlgVideoEncoderInput(c, fps)
	args = append(args, "-c:v", "hevc_nvenc", "-gpu", strconv.Itoa(device), "-profile:v", "main10", "-preset", "p6", "-tune", "hq", "-rc", "constqp", "-qp", "16", "-pix_fmt", "p010le")
	return append(args, hlgVideoEncoderOutput(name)...), nil
}

func TestNVENCQualificationArguments(t *testing.T) {
	c := media.CanvasSize{Width: 1920, Height: 1080}
	cpu := hlgVideoEncoderArgs(c, 30, "out.mp4")
	gpu, err := nvencQualificationArgs(c, 30, 2, "out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	input, output := hlgVideoEncoderInput(c, 30), hlgVideoEncoderOutput("out.mp4")
	for _, args := range [][]string{cpu, gpu} {
		if !slices.Equal(args[:len(input)], input) || !slices.Equal(args[len(args)-len(output):], output) {
			t.Fatal("encoder contract boundary changed")
		}
	}
	if !slices.Contains(cpu, "libx265") || slices.Contains(cpu, "hevc_nvenc") || !slices.Contains(gpu, "hevc_nvenc") || slices.Contains(gpu, "libx265") || !slices.Contains(gpu, "p010le") {
		t.Fatal("encoder selection changed")
	}
	if _, err := nvencQualificationArgs(c, 30, -1, "out.mp4"); err == nil {
		t.Fatal("implicit device accepted")
	}
	if _, err := nvencQualificationArgs(c, 29, 0, "out.mp4"); err == nil {
		t.Fatal("unsupported frame rate accepted")
	}
	if _, err := nvencQualificationArgs(media.CanvasSize{Width: 8192, Height: 2160}, 30, 0, "out.mp4"); err == nil {
		t.Fatal("oversized canvas accepted")
	}
}

func encoderFixtureCode(n int) ycbcr10 {
	return ycbcr10{uint16(140 + n*60), uint16(420 + n*13), uint16(590 - n*11)}
}

func fillEncoderFixture(frame []byte, n int) {
	plane := len(frame) / 3
	p := encoderFixtureCode(n)
	for j, v := range []uint16{p.Y, p.Cb, p.Cr} {
		for i := j * plane; i < (j+1)*plane; i += 2 {
			binary.LittleEndian.PutUint16(frame[i:], v)
		}
	}
}

func encodeQualificationFixture(ctx context.Context, engine media.Engine, c media.CanvasSize, args []string) (time.Duration, error) {
	p := newExportProcess(ctx, engine.FFmpeg, args...)
	defer p.close()
	stdin, err := p.cmd.StdinPipe()
	if err != nil {
		return 0, err
	}
	p.pipe = stdin
	frame := make([]byte, c.Width*c.Height*6)
	start := time.Now()
	if err := p.cmd.Start(); err != nil {
		return 0, err
	}
	for n := 0; n < encoderQualificationFrames; n++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		fillEncoderFixture(frame, n)
		if wrote, err := stdin.Write(frame); err != nil {
			return 0, err
		} else if wrote != len(frame) {
			return 0, io.ErrShortWrite
		}
	}
	if err := stdin.Close(); err != nil {
		return 0, err
	}
	if err := p.wait(); err != nil {
		return 0, err
	}
	return time.Since(start), ctx.Err()
}

type encoderFixtureQuality struct {
	maxDelta [3]int
	minPSNR  [3]float64
}

func checkEncoderFixturePlane(raw []byte, want uint16) (int, float64, error) {
	if len(raw) == 0 || len(raw)%2 != 0 {
		return 0, 0, errors.New("invalid decoded plane")
	}
	var squares uint64
	var largest int
	for i := 0; i < len(raw); i += 2 {
		v := binary.LittleEndian.Uint16(raw[i:])
		if v > 1023 {
			return 0, 0, errors.New("decoded sample exceeds 10-bit range")
		}
		d := absCode(v, want)
		largest = max(largest, d)
		squares += uint64(d * d)
	}
	psnr := 100.0
	if squares != 0 {
		psnr = min(100, 10*math.Log10(1023*1023/(float64(squares)/float64(len(raw)/2))))
	}
	return largest, psnr, nil
}

func verifyEncoderFixture(ctx context.Context, engine media.Engine, plan Plan, path string) (encoderFixtureQuality, error) {
	q := encoderFixtureQuality{minPSNR: [3]float64{100, 100, 100}}
	report, err := engine.Inspect(ctx, path)
	if err != nil {
		return q, err
	}
	if err := verifyVideoOutput(plan, report); err != nil {
		return q, err
	}
	if err := verifyPublishedTiming(ctx, engine, plan, path, report); err != nil {
		return q, err
	}
	c := plan.Contract.Canvas
	p := newExportProcess(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-hwaccel", "none", "-c:v", "hevc", "-i", path, "-map", "0:v:0", "-an", "-sn", "-dn", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-fps_mode", "passthrough", "-f", "rawvideo", "pipe:1")
	defer p.close()
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return q, err
	}
	p.pipe = stdout
	if err := p.cmd.Start(); err != nil {
		return q, err
	}
	frame := make([]byte, c.Width*c.Height*3)
	y := c.Width * c.Height * 2
	bounds := [4]int{0, y, y + y/4, len(frame)}
	for n := 0; n < encoderQualificationFrames; n++ {
		if err := ctx.Err(); err != nil {
			return q, err
		}
		if _, err := io.ReadFull(stdout, frame); err != nil {
			return q, err
		}
		code := encoderFixtureCode(n)
		for j, want := range []uint16{code.Y, code.Cb, code.Cr} {
			d, psnr, err := checkEncoderFixturePlane(frame[bounds[j]:bounds[j+1]], want)
			if err != nil {
				return q, err
			}
			q.maxDelta[j] = max(q.maxDelta[j], d)
			q.minPSNR[j] = min(q.minPSNR[j], psnr)
		}
	}
	var extra [1]byte
	if n, err := stdout.Read(extra[:]); n != 0 || err != io.EOF {
		return q, errors.New("extra or incomplete decoded encoder fixture")
	}
	if err := p.wait(); err != nil {
		return q, err
	}
	for j, d := range q.maxDelta {
		if d > 12 || q.minPSNR[j] < 40 {
			return q, fmt.Errorf("flat-color encoder fixture exceeds provisional limits: plane %d max delta %d PSNR %.3f", j, d, q.minPSNR[j])
		}
	}
	return q, ctx.Err()
}

func TestEncoderFixturePlane(t *testing.T) {
	raw := []byte{0, 2, 0, 2}
	if d, p, err := checkEncoderFixturePlane(raw, 512); err != nil || d != 0 || p != 100 {
		t.Fatalf("exact plane: %d %.3f %v", d, p, err)
	}
	raw[0] = 12
	if d, _, err := checkEncoderFixturePlane(raw, 512); err != nil || d != 12 {
		t.Fatalf("lossy plane: %d %v", d, err)
	}
	raw[1] = 4
	if _, _, err := checkEncoderFixturePlane(raw, 512); err == nil {
		t.Fatal("invalid 10-bit code accepted")
	}
}

func qualifyEncoder(t *testing.T, ctx context.Context, engine media.Engine, c media.CanvasSize, fps int, backend string, device int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "encoded.mp4")
	args := hlgVideoEncoderArgs(c, fps, path)
	if backend == "nvenc" {
		var err error
		args, err = nvencQualificationArgs(c, fps, device, path)
		if err != nil {
			t.Fatal(err)
		}
	}
	plan := Plan{Contract: media.ExportContract{Color: "hlg", Canvas: c, FPS: fps}, VideoFrames: encoderQualificationFrames}
	elapsed, err := encodeQualificationFixture(ctx, engine, c, args)
	if err != nil {
		t.Fatal(err)
	}
	q, err := verifyEncoderFixture(ctx, engine, plan, path)
	if err != nil {
		t.Fatalf("%s quality %+v: %v", backend, q, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("backend=%s device=%d width=%d height=%d fps=%d frames=%d max_delta_yuv=%v min_psnr_yuv=%v encode_feed_wall=%s bytes=%d", backend, device, c.Width, c.Height, fps, encoderQualificationFrames, q.maxDelta, q.minPSNR, elapsed, info.Size())
}

func TestHLGEncoderQualification(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_NVENC") == "1"
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			if physical {
				t.Fatal(err)
			}
			t.Skip(name + " unavailable")
		}
	}
	engine := media.Default()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t.Run("cpu-reference", func(t *testing.T) {
		qualifyEncoder(t, ctx, engine, media.CanvasSize{Width: 64, Height: 48}, 30, "cpu", 0)
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
	for _, tool := range []string{engine.FFmpeg, engine.FFprobe} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		version := exec.CommandContext(ctx, path, "-version")
		version.WaitDelay = time.Second
		var log errorTail
		version.Stdout, version.Stderr = &log, &log
		if err := version.Run(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s\n%s", path, log.data)
	}
	for _, tc := range []struct {
		c   media.CanvasSize
		fps int
	}{
		{media.CanvasSize{Width: 640, Height: 360}, 24},
		{media.CanvasSize{Width: 1920, Height: 1080}, 30},
		{media.CanvasSize{Width: 1080, Height: 1920}, 60},
		{media.CanvasSize{Width: 3840, Height: 2160}, 30},
	} {
		t.Run(fmt.Sprintf("%dx%d-%d", tc.c.Width, tc.c.Height, tc.fps), func(t *testing.T) {
			for _, backend := range []string{"cpu", "nvenc"} {
				t.Run(backend, func(t *testing.T) {
					qualifyEncoder(t, ctx, engine, tc.c, tc.fps, backend, device)
				})
			}
		})
	}
}
