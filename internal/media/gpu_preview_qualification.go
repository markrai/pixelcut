package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type PreviewScaleOptions struct {
	Diagnose      bool
	Chroma        bool
	Benchmark     bool
	CorrectChroma bool
}

type PreviewQualificationTools struct {
	FFmpegPath     string `json:"ffmpeg_path"`
	FFmpegVersion  string `json:"ffmpeg_version"`
	FFprobePath    string `json:"ffprobe_path"`
	FFprobeVersion string `json:"ffprobe_version"`
}

func qualificationVersion(ctx context.Context, name, kind string) (string, string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	job, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(job, path, "-version")
	cmd.WaitDelay = time.Second
	var out capabilityOutput
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return "", "", err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(out.data)), "\n")
	first = strings.TrimSpace(first)
	if out.overflow || !strings.HasPrefix(first, kind+" version ") || len(first) > 1024 {
		return "", "", errors.New("unexpected qualification tool version")
	}
	return path, first, nil
}

func (e Engine) previewQualificationTools(ctx context.Context) (Engine, *PreviewQualificationTools, error) {
	var tools PreviewQualificationTools
	var err error
	tools.FFmpegPath, tools.FFmpegVersion, err = qualificationVersion(ctx, e.FFmpeg, "ffmpeg")
	if err != nil {
		return e, nil, fmt.Errorf("identify FFmpeg: %w", err)
	}
	tools.FFprobePath, tools.FFprobeVersion, err = qualificationVersion(ctx, e.FFprobe, "ffprobe")
	if err != nil {
		return e, &tools, fmt.Errorf("identify FFprobe: %w", err)
	}
	e.FFmpeg, e.FFprobe = tools.FFmpegPath, tools.FFprobePath
	return e, &tools, nil
}

type PreviewChromaControl struct {
	InputLocation  string                  `json:"input_location"`
	OutputLocation string                  `json:"output_location"`
	CUDAAgainstCPU *PreviewPlaneComparison `json:"cuda_against_cpu,omitempty"`
}

func previewChromaControlGraph(v Stream, req StillRequest, first, count int, in, out string) (string, error) {
	if (in != "left" && in != "center") || (out != "left" && out != "center") {
		return "", errors.New("unsupported chroma diagnostic location")
	}
	if _, err := buildPreviewStageGraphs(v, req, first, count); err != nil {
		return "", err
	}
	rotation, err := v.DisplayRotation()
	if err != nil {
		return "", err
	}
	w, h := req.OutputWidth, req.OutputHeight
	if rotation == -90 || rotation == 90 {
		w, h = h, w
	}
	filters := []string{fmt.Sprintf("scale=%d:%d:flags=lanczos:in_chroma_loc=%s:out_chroma_loc=%s", w, h, in, out), "format=yuv420p10le"}
	filters = append(filters, displayRotationFilters(rotation)...)
	filters = append(filters, fmt.Sprintf("select=between(n\\,%d\\,%d)", first, first+count-1), "format=yuv420p10le")
	return strings.Join(filters, ","), nil
}

func (e Engine) diagnosePreviewChroma(ctx context.Context, v Stream, req StillRequest, first int, ticks []int64) ([]PreviewChromaControl, error) {
	graphs, err := buildPreviewStageGraphs(v, req, first, len(ticks))
	if err != nil {
		return nil, err
	}
	cuda := previewPairArgs(req.Path, append([]string{"-noautorotate"}, cudaDecodeInput()...), graphs.cuda, len(ticks), "yuv420p10le")
	cpuInput := []string{"-noautorotate", "-hwaccel", "none", "-c:v", "hevc"}
	results := make([]PreviewChromaControl, 0, 4)
	for _, locations := range [][2]string{{"left", "left"}, {"left", "center"}, {"center", "left"}, {"center", "center"}} {
		graph, err := previewChromaControlGraph(v, req, first, len(ticks), locations[0], locations[1])
		if err != nil {
			return results, err
		}
		cpu := previewPairArgs(req.Path, cpuInput, graph, len(ticks), "yuv420p10le")
		comparison, err := e.comparePreviewPlanes(ctx, req, first, ticks, cpu, cuda)
		if err != nil {
			return results, fmt.Errorf("chroma control %s to %s (requires FFmpeg scale in_chroma_loc/out_chroma_loc): %w", locations[0], locations[1], err)
		}
		results = append(results, PreviewChromaControl{InputLocation: locations[0], OutputLocation: locations[1], CUDAAgainstCPU: comparison})
	}
	return results, nil
}

type PreviewScaleBenchmark struct {
	Frames           int       `json:"frames"`
	CPUSeconds       []float64 `json:"cpu_seconds"`
	CUDASeconds      []float64 `json:"cuda_seconds"`
	CPUMedianSeconds float64   `json:"cpu_median_seconds"`
	GPUMedianSeconds float64   `json:"cuda_median_seconds"`
}

func medianPreviewSeconds(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func (e Engine) timePreviewPath(ctx context.Context, args []string, req StillRequest, ticks []int64) (float64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()
	child, err := startPreviewCheck(ctx, e, args)
	if err != nil {
		return 0, err
	}
	defer func() { cancel(); child.close() }()
	pixels := make([]byte, req.OutputWidth*req.OutputHeight*4)
	sums := make([][32]byte, len(ticks))
	for i := range ticks {
		if err := child.readFrame(pixels); err != nil {
			return 0, err
		}
		sums[i] = sha256.Sum256(pixels)
	}
	hashes, err := child.finish(req.OutputWidth, req.OutputHeight, len(ticks), 4)
	if err != nil {
		return 0, err
	}
	for i, tick := range ticks {
		if tickDistance(hashes[i].pts, tick) > 1 || hashes[i].hash != hex.EncodeToString(sums[i][:]) {
			return 0, fmt.Errorf("benchmark frame %d timestamp or raw hash mismatch", i)
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return time.Since(start).Seconds(), nil
}

func (e Engine) benchmarkPreviewScale(ctx context.Context, v Stream, req StillRequest, first int, ticks []int64, corrected bool) (*PreviewScaleBenchmark, error) {
	cpu, err := previewCheckArgs(e, v, req, first, len(ticks), false)
	if err != nil {
		return nil, err
	}
	cuda, err := previewCheckArgsWithCorrection(e, v, req, first, len(ticks), true, corrected)
	if err != nil {
		return nil, err
	}
	r := &PreviewScaleBenchmark{Frames: len(ticks), CPUSeconds: []float64{}, CUDASeconds: []float64{}}
	for round := 0; round < 3; round++ {
		for _, hardware := range []bool{round%2 == 1, round%2 != 1} {
			args, target, name := cpu, &r.CPUSeconds, "CPU"
			if hardware {
				args, target, name = cuda, &r.CUDASeconds, "CUDA"
			}
			seconds, err := e.timePreviewPath(ctx, args, req, ticks)
			if err != nil {
				return r, fmt.Errorf("%s benchmark round %d: %w", name, round+1, err)
			}
			*target = append(*target, seconds)
		}
	}
	r.CPUMedianSeconds = medianPreviewSeconds(r.CPUSeconds)
	r.GPUMedianSeconds = medianPreviewSeconds(r.CUDASeconds)
	return r, nil
}
