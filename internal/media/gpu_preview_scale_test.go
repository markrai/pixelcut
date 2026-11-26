package media

import (
	"context"
	"strings"
	"testing"
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
