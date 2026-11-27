package media

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewBackendPaths(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{1920, 1080}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	req := StillRequest{Path: "source.mp4", Width: 1920, Height: 1080, Canvas: CanvasSize{1920, 1080}, OutputWidth: 960, OutputHeight: 540}
	hybrid, err := buildPreviewBackendPaths(v, req, 2, 4, PreviewBackendOptions{Backend: "hybrid"})
	if err != nil || hybrid.candidateBGRA != cudaDownloadFilter+","+hybrid.referenceBGRA || hybrid.candidatePlanes != cudaDownloadFilter+","+hybrid.referencePlanes || !strings.Contains(strings.Join(hybrid.candidateInput, " "), "-hwaccel cuda") {
		t.Fatalf("hybrid graph: %+v %v", hybrid, err)
	}
	vulkan, err := buildPreviewBackendPaths(v, req, 2, 4, PreviewBackendOptions{Backend: "vulkan", VulkanDevice: 2})
	if err != nil || !strings.Contains(strings.Join(vulkan.candidateInput, " "), "-init_hw_device vulkan=pcvk:2 -filter_hw_device pcvk") || !strings.Contains(vulkan.candidatePlanes, "select=between(n\\,2\\,5),scale=1920:1080:flags=lanczos,libplacebo=w=960:h=540") || !strings.Contains(vulkan.candidatePlanes, "chroma_location=auto") || strings.Contains(vulkan.candidatePlanes, "scale_cuda") || !strings.HasSuffix(vulkan.candidateBGRA, ","+previewToneMapGraph) {
		t.Fatalf("Vulkan graph: %+v %v", vulkan, err)
	}
	if vulkan.referenceBGRA != hybrid.referenceBGRA || vulkan.referencePlanes != hybrid.referencePlanes {
		t.Fatal("CPU reference changed")
	}
	if _, err := buildPreviewBackendPaths(v, req, 0, 4, PreviewBackendOptions{Backend: "hybrid", VulkanDevice: 2}); err == nil {
		t.Fatal("hybrid accepted Vulkan device")
	}
	if _, err := buildPreviewBackendPaths(v, req, 0, 4, PreviewBackendOptions{Backend: "vulkan", VulkanDevice: -1}); err == nil {
		t.Fatal("invalid Vulkan device accepted")
	}
	if _, err := buildPreviewBackendPaths(v, req, 0, 4, PreviewBackendOptions{Backend: "auto"}); err == nil {
		t.Fatal("automatic alternative selection accepted")
	}
}

func TestPreviewBackendRejectsInvalidRequest(t *testing.T) {
	r, err := Default().CheckPreviewBackend(context.Background(), "missing.mp4", 0, 121, 960, 540, PreviewBackendOptions{Backend: "hybrid"})
	if err == nil || r.Status != "failed" || r.Failure == "" {
		t.Fatalf("invalid request accepted: %+v %v", r, err)
	}
}
