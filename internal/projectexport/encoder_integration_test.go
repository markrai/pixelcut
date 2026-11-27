package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestProjectEncoderSelection(t *testing.T) {
	c := media.CanvasSize{Width: 1920, Height: 1080}
	args, err := (videoEncoderSpec{}).args(c, 30, "output.mp4")
	if err != nil || !slices.Equal(args, hlgVideoEncoderArgs(c, 30, "output.mp4")) {
		t.Fatalf("default CPU command changed: %v", err)
	}
	gpu, err := (videoEncoderSpec{nvenc: true, device: 2, qp: 18}).args(c, 30, "output.mp4")
	if err != nil || !slices.Contains(gpu, "hevc_nvenc") || slices.Contains(gpu, "libx265") || gpu[slices.Index(gpu, "-qp")+1] != "18" || gpu[slices.Index(gpu, "-gpu")+1] != "2" {
		t.Fatalf("NVENC command: %v %v", gpu, err)
	}
	for _, spec := range []videoEncoderSpec{{qp: 16}, {device: 1}, {nvenc: true, device: -1, qp: 16}, {nvenc: true, qp: 52}} {
		if _, err := spec.args(c, 30, "output.mp4"); err == nil {
			t.Fatalf("invalid encoder spec accepted: %+v", spec)
		}
	}
}

func TestProjectNVENCIntegration(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_NVENC_PROJECT") != "1" {
		t.Skip("physical NVENC project qualification is opt-in")
	}
	device, qp := 0, 16
	for _, option := range []struct {
		key string
		dst *int
		max int
	}{{"PIXELCUT_NVENC_DEVICE", &device, 31}, {"PIXELCUT_NVENC_PROJECT_QP", &qp, 51}} {
		if value := os.Getenv(option.key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > option.max {
				t.Fatalf("invalid %s", option.key)
			}
			*option.dst = n
		}
	}
	engine := media.Default()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	for _, tool := range []*string{&engine.FFmpeg, &engine.FFprobe} {
		path, err := exec.LookPath(*tool)
		if err != nil {
			t.Fatal(err)
		}
		*tool, err = filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, *tool, "-version")
		var log errorTail
		cmd.Stdout, cmd.Stderr = &log, &log
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s\n%s", *tool, log.data)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=1,format=yuv420p10le,geq=lum='180+50*N':cb=512:cr=512,setsar=1", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=1", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
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
		t.Fatalf("source fixture: %v", err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.CanvasWidth, p.CanvasHeight = 192, 144
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", Start: 100000, In: 100000, Out: 400000}}
	p.Tracks[1].Clips = []project.Clip{{ID: "audio", AssetID: "a", Start: 450000, In: 100000, Out: 200000}}
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "top", AssetID: "a", Start: 200000, In: 500000, Out: 600000, Transform: &media.Transform{CanvasWidth: 192, CanvasHeight: 144, Scale: 50}}}
	p, err = p.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "top"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Build(p, "hlg", 30)
	if err != nil || !plan.Contract.Audio {
		t.Fatalf("project plan: %v", err)
	}
	var video, audio [2][]byte
	for i, spec := range []videoEncoderSpec{{}, {nvenc: true, device: device, qp: qp}} {
		out := filepath.Join(dir, strconv.Itoa(i)+".mp4")
		var events []Progress
		start := time.Now()
		if err := renderProjectWithEncoder(ctx, engine, p, "hlg", 30, true, false, out, func(v Progress) { events = append(events, v) }, spec); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		if err := checkExportProgress(events, plan); err != nil {
			t.Fatal(err)
		}
		if spec.nvenc {
			found := false
			for _, event := range events {
				for _, warning := range event.Warnings {
					found = found || strings.Contains(warning, "Qualification-only NVENC")
				}
			}
			if !found {
				t.Fatal("NVENC qualification warning missing")
			}
		}
		report, err := engine.Inspect(ctx, out)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyProjectOutput(plan, report); err != nil {
			t.Fatal(err)
		}
		if err := verifyPublishedTiming(ctx, engine, plan, out, report); err != nil {
			t.Fatal(err)
		}
		video[i], err = exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-hwaccel", "none", "-i", out, "-map", "0:v:0", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "yuv420p10le", "-fps_mode", "passthrough", "-f", "rawvideo", "-").Output()
		if err != nil || int64(len(video[i])) != plan.VideoFrames*192*144*3 {
			t.Fatalf("decoded video: %d bytes %v", len(video[i]), err)
		}
		audio[i], err = exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-i", out, "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "s16le", "-").Output()
		if err != nil || int64(len(audio[i])) < plan.OutputAudioFrames*4 {
			t.Fatalf("decoded audio: %d bytes %v", len(audio[i]), err)
		}
		info, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("backend=%s qp=%d device=%d frames=%d audio_samples=%d bytes=%d project_wall=%s", spec.name(), spec.qp, spec.device, plan.VideoFrames, plan.OutputAudioFrames, info.Size(), elapsed)
	}
	if !bytes.Equal(audio[0], audio[1]) {
		t.Fatal("encoder selection changed decoded AAC")
	}
	const frameBytes = 192 * 144 * 3
	for _, raw := range video {
		for _, tc := range []struct {
			frame, x, y, want int
		}{{0, 4, 4, 64}, {3, 4, 4, 230}, {6, 4, 4, 280}, {9, 4, 4, 330}, {12, 4, 4, 64}} {
			got := int(binary.LittleEndian.Uint16(raw[tc.frame*frameBytes+2*(tc.y*192+tc.x):]))
			if got < tc.want-12 || got > tc.want+12 {
				t.Fatalf("frame %d luma %d want %d", tc.frame, got, tc.want)
			}
		}
	}
	bounds := [4]int{0, 192 * 144 * 2, 192 * 144 * 5 / 2, frameBytes}
	var minPSNR = [3]float64{100, 100, 100}
	var maxDelta [3]int
	for n := 0; n < int(plan.VideoFrames); n++ {
		for j := 0; j < 3; j++ {
			lo, hi := n*frameBytes+bounds[j], n*frameBytes+bounds[j+1]
			squared, delta, err := motionPlaneError(video[0][lo:hi], video[1][lo:hi])
			if err != nil {
				t.Fatal(err)
			}
			minPSNR[j] = min(minPSNR[j], motionPSNR(squared, uint64((hi-lo)/2)))
			maxDelta[j] = max(maxDelta[j], delta)
		}
	}
	t.Logf("CPU-vs-NVENC min_psnr_yuv=%v max_delta_yuv=%v", minPSNR, maxDelta)
	for _, value := range minPSNR {
		if value < 40 {
			t.Fatal("integrated encoder fixture failed provisional comparative fidelity screen")
		}
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("source changed")
	}
	files, err := filepath.Glob(filepath.Join(dir, ".pixelcut-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("staged output leaked: %v %v", files, err)
	}
}
