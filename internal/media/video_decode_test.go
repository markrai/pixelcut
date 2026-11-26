package media

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestVideoDecodePipelinePolicy(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{1920, 1080}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	for _, mode := range []Backend{"", BackendCPU, BackendAuto} {
		e := Default()
		e.DecodeBackend = mode
		args, graph, err := e.VideoDecodePipeline(v, "scale=64:48", false)
		if err != nil || len(args) != 0 || graph != "scale=64:48" {
			t.Fatalf("CPU/auto changed the reference: %v %q %v", args, graph, err)
		}
	}
	e := Default()
	e.DecodeBackend = BackendCUDA
	args, graph, err := e.VideoDecodePipeline(v, "scale=64:48", false)
	if err != nil || !slices.Equal(args, []string{"-noautorotate", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda", "-c:v", "hevc"}) || graph != cudaDownloadFilter+",scale=64:48" {
		t.Fatalf("CUDA pipeline: %v %q %v", args, graph, err)
	}
	for _, bad := range []Stream{{}, {CodecName: "h264"}, {CodecName: "hevc", Profile: "Main"}} {
		if _, _, err := e.VideoDecodePipeline(bad, "", false); err == nil {
			t.Fatal("unsupported forced CUDA source accepted")
		}
	}
	e.DecodeBackend = "unknown"
	if _, _, err := e.VideoDecodePipeline(v, "", false); err == nil {
		t.Fatal("unknown backend accepted")
	}
}

func TestExperimentalCUDACompositePixels(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_CUDA") != "1" {
		t.Skip("set PIXELCUT_QUALIFY_CUDA=1 on a CUDA workstation")
	}
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "source.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=1,format=yuv420p10le,geq=lum='64+mod(17*X+13*Y+31*N,877)':cb='64+mod(7*X+11*Y+13*N,897)':cr='64+mod(9*X+19*Y+17*N,897)',setsar=1", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
	cpu, gpu := Default(), Default()
	gpu.DecodeBackend = BackendCUDA
	cpuStill, gpuStill := NewStillDecoder(cpu), NewStillDecoder(gpu)
	cpuStream, gpuStream := NewStreamDecoder(ctx, cpu), NewStreamDecoder(ctx, gpu)
	defer cpuStream.Close()
	defer gpuStream.Close()
	req := StillRequest{Path: path, ClipID: "one", SourceEnd: 1000000, Width: 192, Height: 144, Range: "tv", Canvas: CanvasSize{192, 144}, OutputWidth: 96, OutputHeight: 72, Crop: &Crop{X: 8, Y: 8, Width: 176, Height: 128}}
	for _, tick := range []int64{0, 100000, 200000, 650000, 750000} {
		req.Tick = tick
		c, err := cpuStill.Decode(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		g, err := gpuStill.Decode(ctx, req)
		if err != nil || !bytes.Equal(c.Pixels, g.Pixels) {
			t.Fatalf("CUDA still differs at %d: %v", tick, err)
		}
		c, err = cpuStream.Decode(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		g, err = gpuStream.Decode(ctx, req)
		if err != nil || !bytes.Equal(c.Pixels, g.Pixels) {
			t.Fatalf("CUDA streaming differs at %d: %v", tick, err)
		}
	}
	gpuStream.Close()
	stats := gpuStream.Stats()
	if stats.Opened != 2 || stats.Opened != stats.Retired || stats.Peak != 1 {
		t.Fatalf("CUDA stream lifecycle: %+v", stats)
	}
	t.Logf("CUDA processed still and streaming parity; stats=%+v", stats)
}

func TestExperimentalCUDAInvalidBackendReport(t *testing.T) {
	e := Default()
	e.DecodeBackend = "unknown"
	if _, _, err := e.VideoDecodePipeline(Stream{}, "", false); err == nil || !strings.Contains(err.Error(), "unknown decode backend") {
		t.Fatal("invalid backend not rejected")
	}
}
