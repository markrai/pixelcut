package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/markrai/pixelcut/internal/mediacache"
)

type PreviewBackendOptions struct {
	Backend      string
	VulkanDevice int
	Benchmark    bool
}

type PreviewBackendReport struct {
	Backend      string                     `json:"backend"`
	Decode       string                     `json:"decode"`
	Geometry     string                     `json:"geometry"`
	Transfer     string                     `json:"transfer"`
	VulkanDevice *int                       `json:"vulkan_device_index,omitempty"`
	Source       string                     `json:"source"`
	SourceVideo  *PreviewScaleSource        `json:"source_video,omitempty"`
	Tools        *PreviewQualificationTools `json:"tools,omitempty"`
	FirstFrame   int                        `json:"first_frame"`
	Frames       int                        `json:"frames"`
	Compared     int                        `json:"compared"`
	OutputWidth  int                        `json:"output_width"`
	OutputHeight int                        `json:"output_height"`
	Status       string                     `json:"status"`
	Failure      string                     `json:"failure,omitempty"`
	Quality      PreviewScaleQuality        `json:"quality"`
	Planes       *PreviewPlaneComparison    `json:"planes,omitempty"`
	Benchmark    *PreviewBackendBenchmark   `json:"benchmark,omitempty"`
	WallSeconds  float64                    `json:"wall_seconds"`
}

type previewBackendPaths struct {
	referenceInput, candidateInput   []string
	referencePlanes, candidatePlanes string
	referenceBGRA, candidateBGRA     string
}

func validatePreviewBackendOptions(options PreviewBackendOptions) error {
	switch options.Backend {
	case "hybrid":
		if options.VulkanDevice != 0 {
			return errors.New("Vulkan device index applies only to vulkan")
		}
	case "vulkan":
		if options.VulkanDevice < 0 || options.VulkanDevice > 31 {
			return errors.New("Vulkan device index must be 0-31")
		}
	default:
		return errors.New("preview backend must be hybrid or vulkan")
	}
	return nil
}

func buildPreviewBackendPaths(v Stream, req StillRequest, first, count int, options PreviewBackendOptions) (previewBackendPaths, error) {
	var paths previewBackendPaths
	if err := validatePreviewBackendOptions(options); err != nil {
		return paths, err
	}
	graphs, err := buildPreviewStageGraphs(v, req, first, count)
	if err != nil {
		return paths, err
	}
	reference, ok := strings.CutSuffix(graphs.reference, ",format=yuv420p10le")
	if !ok {
		return paths, errors.New("unsupported planar reference boundary")
	}
	paths.referenceInput = []string{"-noautorotate", "-hwaccel", "none", "-c:v", "hevc"}
	paths.referencePlanes = graphs.reference
	paths.referenceBGRA = reference + "," + previewToneMapGraph
	switch options.Backend {
	case "hybrid":
		paths.candidateInput = append([]string{"-noautorotate"}, cudaDecodeInput()...)
		paths.candidatePlanes = cudaDownloadFilter + "," + paths.referencePlanes
		paths.candidateBGRA = cudaDownloadFilter + "," + paths.referenceBGRA
	case "vulkan":
		scale := fmt.Sprintf(",scale=%d:%d:flags=lanczos", req.OutputWidth, req.OutputHeight)
		prefix, ok := strings.CutSuffix(reference, scale)
		if !ok {
			return paths, errors.New("unsupported production output scale boundary")
		}
		paths.candidateInput = append([]string{"-init_hw_device", fmt.Sprintf("vulkan=pcvk:%d", options.VulkanDevice), "-filter_hw_device", "pcvk"}, paths.referenceInput...)
		filter := fmt.Sprintf("libplacebo=w=%d:h=%d:format=yuv420p10le:upscaler=lanczos:downscaler=lanczos:chroma_location=auto:colorspace=bt2020nc:color_primaries=bt2020:color_trc=arib-std-b67:range=tv:peak_detect=0:deband=0:sigmoid=0:disable_linear=1:dithering=none:apply_dovi=0", req.OutputWidth, req.OutputHeight)
		paths.candidatePlanes = prefix + "," + filter + ",format=yuv420p10le"
		paths.candidateBGRA = paths.candidatePlanes + "," + previewToneMapGraph
	}
	return paths, nil
}

func (e Engine) CheckPreviewBackend(ctx context.Context, path string, first, count, width, height int, options PreviewBackendOptions) (r PreviewBackendReport, err error) {
	r = PreviewBackendReport{Backend: options.Backend, Source: path, FirstFrame: first, Frames: count, OutputWidth: width, OutputHeight: height, Status: "failed"}
	defer func() {
		if err != nil {
			r.Failure = err.Error()
		}
	}()
	if err = validatePreviewBackendOptions(options); err != nil {
		return r, err
	}
	if path == "" || first < 0 || count < 1 || count > maxPreviewScaleFrames || first > MaxIndexedFrames-count ||
		width < 2 || height < 2 || width > MaxStillSide || height > MaxStillSide || width%2 != 0 || height%2 != 0 {
		return r, errors.New("invalid preview backend dimensions or interval (even dimensions up to 960; 1-120 frames)")
	}
	switch options.Backend {
	case "hybrid":
		r.Decode, r.Geometry, r.Transfer = "cuda", "cpu", "full_resolution_download"
	case "vulkan":
		r.Decode, r.Geometry, r.Transfer = "cpu", "libplacebo", "host_upload_reduced_download"
		r.VulkanDevice = &options.VulkanDevice
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	identity, err := mediacache.Identify(path)
	if err != nil {
		return r, err
	}
	e, r.Tools, err = e.previewQualificationTools(ctx)
	if err != nil {
		return r, err
	}
	report, err := e.Inspect(ctx, identity.Path)
	if err != nil {
		return r, err
	}
	v, err := report.Video()
	if err != nil {
		return r, err
	}
	rotation, err := v.DisplayRotation()
	if err != nil {
		return r, err
	}
	r.SourceVideo = &PreviewScaleSource{Width: v.Width, Height: v.Height, Range: v.Range, ChromaLocation: v.ChromaLocation, Rotation: &rotation}
	display, err := v.DisplayGeometry()
	if err != nil {
		return r, err
	}
	req := StillRequest{Path: identity.Path, Width: display.Width, Height: display.Height, Canvas: CanvasSize{display.Width, display.Height}, OutputWidth: width, OutputHeight: height}
	paths, err := buildPreviewBackendPaths(v, req, first, count, options)
	if err != nil {
		return r, err
	}
	index, err := e.ExportFrames(ctx, identity.Path)
	if err != nil {
		return r, err
	}
	if first >= index.Count() || count > index.Count()-first {
		return r, errors.New("preview backend interval exceeds source index")
	}
	ticks := index.ticks[first:first+count]
	cpu := previewPairArgs(req.Path, paths.referenceInput, paths.referenceBGRA, count, "bgra")
	candidate := previewPairArgs(req.Path, paths.candidateInput, paths.candidateBGRA, count, "bgra")
	var quality previewQualityAccumulator
	started := time.Now()
	r.Compared, err = e.comparePreviewPair(ctx, cpu, candidate, width, height, 4, first, ticks, quality.add)
	r.WallSeconds, r.Quality = time.Since(started).Seconds(), quality.quality
	if err != nil {
		return r, fmt.Errorf("%s displayed comparison: %w", options.Backend, err)
	}
	if !unchanged(identity.Path, identity) {
		return r, errors.New("source changed during preview backend check")
	}
	left := previewPairArgs(req.Path, paths.referenceInput, paths.referencePlanes, count, "yuv420p10le")
	right := previewPairArgs(req.Path, paths.candidateInput, paths.candidatePlanes, count, "yuv420p10le")
	r.Planes, err = e.comparePreviewPlanes(ctx, req, first, ticks, left, right)
	if err != nil {
		return r, fmt.Errorf("%s planar comparison: %w", options.Backend, err)
	}
	if options.Benchmark {
		r.Benchmark, err = e.benchmarkPreviewPaths(ctx, req, ticks, cpu, candidate, options.Backend)
		if err != nil {
			return r, err
		}
	}
	if !unchanged(identity.Path, identity) {
		return r, errors.New("source changed during preview backend qualification")
	}
	if options.Backend == "hybrid" && (r.Planes.ExactFrames != count || r.Quality.ExactFrames != count) {
		return r, errors.New("hybrid decode plus identical CPU geometry requires exact planar and displayed parity")
	}
	if !r.Quality.withinLimits() {
		return r, errors.New("preview backend exceeds provisional quality limits (worst frame/channel PSNR >= 40 dB, max channel delta <= 16)")
	}
	r.Status = "within_limits"
	return r, nil
}
