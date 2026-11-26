package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreviewScaleCandidatePolicy(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{1920, 1080}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	req := StillRequest{Path: "source.mp4", Width: 1920, Height: 1080, Canvas: CanvasSize{1920, 1080}, OutputWidth: 960, OutputHeight: 540}
	e := Default()
	e.DecodeBackend = BackendCUDA
	_, graph, err := e.previewCUDAPipeline(v, req, "select=eq(n\\,2)", "reference", true)
	if err != nil || !strings.HasPrefix(graph, "scale_cuda=w=960:h=540:format=p010le:interp_algo=lanczos:passthrough=0,"+cudaDownloadFilter) || !strings.HasSuffix(graph, previewToneMapGraph) {
		t.Fatalf("candidate graph: %q %v", graph, err)
	}
	_, graph, err = e.previewCUDAPipeline(v, req, "select=eq(n\\,2)", "reference", false)
	if err != nil || graph != cudaDownloadFilter+",select=eq(n\\,2),reference" {
		t.Fatalf("reference graph changed: %q %v", graph, err)
	}
	req.Crop = &Crop{X: 0, Y: 0, Width: 960, Height: 540}
	if _, err := previewScaleCandidate(v, req); err == nil {
		t.Fatal("crop accepted by candidate")
	}
	req.Crop = nil
	req.OutputWidth = 958
	if _, err := previewScaleCandidate(v, req); err == nil {
		t.Fatal("aspect change accepted by candidate")
	}
}

func TestPreviewScaleQualityWorstFrame(t *testing.T) {
	var a previewQualityAccumulator
	c := []byte{10, 20, 30, 255, 10, 20, 30, 255}
	if err := a.add(c, c); err != nil || a.quality.ExactFrames != 1 || !a.quality.withinLimits() {
		t.Fatalf("identical: %+v %v", a.quality, err)
	}
	g := []byte{10, 20, 50, 255, 10, 20, 30, 255}
	if err := a.add(c, g); err != nil || a.quality.MaxChannelDelta != 20 || a.quality.withinLimits() {
		t.Fatalf("large local error accepted: %+v %v", a.quality, err)
	}
	g[3] = 0
	if err := a.add(c, g); err == nil {
		t.Fatal("nonopaque output accepted")
	}
}

func TestPreviewScaleRejectsInvalidRequest(t *testing.T) {
	r, err := Default().CheckCUDAPreviewScale(context.Background(), "missing.mp4", 0, 121, 960, 540)
	if err == nil || r.Status != "failed" || r.Failure == "" {
		t.Fatalf("invalid request accepted: %+v %v", r, err)
	}
}

func TestExperimentalCUDAPreviewScale(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_CUDA_PREVIEW_SCALE") != "1" {
		t.Skip("set PIXELCUT_QUALIFY_CUDA_PREVIEW_SCALE=1 on a CUDA workstation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "source.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=1,format=yuv420p10le,geq=lum='128+2*X+2*Y+8*N':cb='400+X':cr='400+Y',setsar=1", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
	cpu, gpu := Default(), Default()
	gpu.DecodeBackend = BackendCUDA
	report, err := gpu.CheckCUDAPreviewScale(ctx, path, 0, 10, 96, 72)
	if err != nil {
		t.Fatalf("Lanczos qualification: %+v %v", report, err)
	}
	t.Logf("Lanczos qualification: %+v", report)
	req := StillRequest{Path: path, ClipID: "one", SourceEnd: 1000000, Width: 192, Height: 144, Range: "tv", Canvas: CanvasSize{192, 144}, OutputWidth: 96, OutputHeight: 72}
	cpuStill, gpuStill := NewStillDecoder(cpu), NewStillDecoder(gpu)
	gpuStill.previewScale = true
	cpuStream, gpuStream := NewStreamDecoder(ctx, cpu), NewStreamDecoder(ctx, gpu)
	gpuStream.indexer.previewScale = true
	defer cpuStream.Close()
	defer gpuStream.Close()
	var stillQuality, streamQuality previewQualityAccumulator
	var cpuTime, gpuTime time.Duration
	for _, tick := range []int64{0, 100000, 200000, 650000, 750000} {
		req.Tick = tick
		c, err := cpuStill.Decode(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		g, err := gpuStill.Decode(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if err := stillQuality.add(c.Pixels, g.Pixels); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		c, err = cpuStream.Decode(ctx, req)
		cpuTime += time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		started = time.Now()
		g, err = gpuStream.Decode(ctx, req)
		gpuTime += time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		if err := streamQuality.add(c.Pixels, g.Pixels); err != nil {
			t.Fatal(err)
		}
	}
	gpuStream.Close()
	stats := gpuStream.Stats()
	t.Logf("still=%+v stream=%+v cpu stream=%s gpu stream=%s stats=%+v", stillQuality.quality, streamQuality.quality, cpuTime, gpuTime, stats)
	if !stillQuality.quality.withinLimits() || !streamQuality.quality.withinLimits() {
		t.Fatal("candidate preview quality exceeds limits")
	}
	if stats.Opened != 2 || stats.Opened != stats.Retired || stats.Peak != 1 {
		t.Fatalf("candidate stream lifecycle: %+v", stats)
	}
}

func TestPreviewScaleDiagnosticSourceRestrictions(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{1920, 1080}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	req := StillRequest{Path: "source.mp4", Width: 1920, Height: 1080, Canvas: CanvasSize{1920, 1080}, OutputWidth: 960, OutputHeight: 540}
	v.Range = "pc"
	if _, err := previewScaleCandidate(v, req); err == nil || !strings.Contains(err.Error(), "limited-range HLG (tv); got \"pc\"") {
		t.Fatalf("range rejection: %v", err)
	}
	v.Range = "tv"
	v.ChromaLocation = "center"
	if _, err := previewScaleCandidate(v, req); err == nil || !strings.Contains(err.Error(), "left-sited chroma; got \"center\"") {
		t.Fatalf("chroma rejection: %v", err)
	}
}

func TestPreviewScaleDiagnosticWorstSample(t *testing.T) {
	var a previewQualityAccumulator
	c := []byte{10, 20, 30, 255, 40, 50, 60, 255}
	if err := a.add(c, c); err != nil {
		t.Fatal(err)
	}
	g := []byte{10, 20, 49, 255, 40, 67, 60, 255}
	if err := a.add(c, g); err != nil {
		t.Fatal(err)
	}
	q := a.quality
	if q.SamplesCompared != 12 || q.SamplesOverLimit != 2 || q.WorstSample == nil || *q.WorstSample != (PreviewScaleSample{FrameOffset: 1, PixelIndex: 0, Channel: "R", CPU: 30, CUDA: 49}) || q.withinLimits() {
		t.Fatalf("incorrect diagnostic: %+v worst=%+v", q, q.WorstSample)
	}
	if err := a.add(c, c); err != nil || a.quality.SamplesOverLimit != 2 || a.quality.WorstSample.FrameOffset != 1 {
		t.Fatalf("worst sample not retained: %+v %v", a.quality, err)
	}
}
