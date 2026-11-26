package media

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type PreviewPlaneQuality struct {
	Samples           uint64  `json:"samples"`
	DifferentSamples  uint64  `json:"different_samples"`
	MaxDelta          int     `json:"max_delta"`
	MeanAbsoluteDelta float64 `json:"mean_absolute_delta"`
	MinFramePSNRDB    float64 `json:"min_frame_psnr_db"`
}

type PreviewPlaneComparison struct {
	Compared    int                 `json:"compared"`
	ExactFrames int                 `json:"exact_frames"`
	Y           PreviewPlaneQuality `json:"y"`
	U           PreviewPlaneQuality `json:"u"`
	V           PreviewPlaneQuality `json:"v"`
	WallSeconds float64             `json:"wall_seconds"`
}

type PreviewScaleDiagnostics struct {
	CUDAAgainstReference *PreviewPlaneComparison `json:"cuda_against_reference,omitempty"`
	CPUOrderAgainstRef   *PreviewPlaneComparison `json:"cpu_order_against_reference,omitempty"`
	CUDAAgainstCPUOrder  *PreviewPlaneComparison `json:"cuda_against_cpu_order,omitempty"`
}

type planeAccumulator struct {
	q   PreviewPlaneQuality
	sum uint64
}

func (a *planeAccumulator) add(cpu, candidate []byte) error {
	if len(cpu) == 0 || len(cpu) != len(candidate) || len(cpu)%2 != 0 {
		return errors.New("invalid planar 10-bit sample buffer")
	}
	var sum, squares, different uint64
	maxDelta := 0
	for i := 0; i < len(cpu); i += 2 {
		c, g := int(binary.LittleEndian.Uint16(cpu[i:])), int(binary.LittleEndian.Uint16(candidate[i:]))
		if c > 1023 || g > 1023 {
			return errors.New("sample exceeds planar 10-bit code range")
		}
		d := c - g
		if d < 0 {
			d = -d
		}
		if d != 0 {
			different++
		}
		maxDelta = max(maxDelta, d)
		sum += uint64(d)
		squares += uint64(d * d)
	}
	samples := uint64(len(cpu) / 2)
	psnr := 100.0
	if squares != 0 {
		psnr = min(psnr, 10*math.Log10(1023*1023/(float64(squares)/float64(samples))))
	}
	if a.q.Samples == 0 || psnr < a.q.MinFramePSNRDB {
		a.q.MinFramePSNRDB = psnr
	}
	a.sum += sum
	a.q.Samples += samples
	a.q.DifferentSamples += different
	a.q.MaxDelta = max(a.q.MaxDelta, maxDelta)
	a.q.MeanAbsoluteDelta = float64(a.sum) / float64(a.q.Samples)
	return nil
}

type previewPlaneAccumulator struct {
	width, height int
	planes        [3]planeAccumulator
	exactFrames   int
}

func (a *previewPlaneAccumulator) add(cpu, candidate []byte) error {
	y := a.width * a.height * 2
	uv := y / 4
	if a.width < 2 || a.height < 2 || a.width%2 != 0 || a.height%2 != 0 || len(cpu) != y+2*uv || len(candidate) != len(cpu) {
		return errors.New("invalid planar 4:2:0 frame")
	}
	bounds := [4]int{0, y, y + uv, y + 2*uv}
	exact := true
	for i := range a.planes {
		p := &a.planes[i]
		before := p.q.DifferentSamples
		if err := p.add(cpu[bounds[i]:bounds[i+1]], candidate[bounds[i]:bounds[i+1]]); err != nil {
			return err
		}
		exact = exact && p.q.DifferentSamples == before
	}
	if exact {
		a.exactFrames++
	}
	return nil
}

type previewStageGraphs struct {
	reference, cpuOrder, cuda string
}

func buildPreviewStageGraphs(v Stream, req StillRequest, first, count int) (previewStageGraphs, error) {
	if first < 0 || count < 1 || count > maxPreviewScaleFrames || first > MaxIndexedFrames-count {
		return previewStageGraphs{}, errors.New("invalid stage diagnostic interval")
	}
	cuda, err := previewScaleCandidate(v, req)
	if err != nil {
		return previewStageGraphs{}, err
	}
	reference, err := stillFilter(req)
	if err != nil {
		return previewStageGraphs{}, err
	}
	reference, ok := strings.CutSuffix(reference, ","+previewToneMapGraph)
	if !ok {
		return previewStageGraphs{}, errors.New("unsupported preview tone map boundary")
	}
	rotation, err := v.DisplayRotation()
	if err != nil {
		return previewStageGraphs{}, err
	}
	temporal := fmt.Sprintf("select=between(n\\,%d\\,%d)", first, first+count-1)
	rotate := displayRotationFilters(rotation)
	w, h := req.OutputWidth, req.OutputHeight
	if rotation == -90 || rotation == 90 {
		w, h = h, w
	}
	order := []string{fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h), "format=yuv420p10le"}
	order = append(order, rotate...)
	return previewStageGraphs{
		reference: strings.Join(append(displayRotationFilters(rotation), temporal, reference, "format=yuv420p10le"), ","),
		cpuOrder:  strings.Join(append(order, temporal, "format=yuv420p10le"), ","),
		cuda:      cuda + "," + temporal + ",format=yuv420p10le",
	}, nil
}

func (e Engine) comparePreviewPlanes(ctx context.Context, req StillRequest, first int, ticks []int64, left, right []string) (*PreviewPlaneComparison, error) {
	a := previewPlaneAccumulator{width: req.OutputWidth, height: req.OutputHeight}
	start := time.Now()
	n, err := e.comparePreviewPair(ctx, left, right, a.width, a.height, 3, first, ticks, a.add)
	if err != nil {
		return nil, err
	}
	return &PreviewPlaneComparison{
		Compared: n, ExactFrames: a.exactFrames,
		Y: a.planes[0].q, U: a.planes[1].q, V: a.planes[2].q,
		WallSeconds: time.Since(start).Seconds(),
	}, nil
}

func (e Engine) diagnosePreviewStages(ctx context.Context, v Stream, req StillRequest, first int, ticks []int64, out *PreviewScaleDiagnostics) error {
	graphs, err := buildPreviewStageGraphs(v, req, first, len(ticks))
	if err != nil {
		return err
	}
	cpuInput := []string{"-noautorotate", "-hwaccel", "none", "-c:v", "hevc"}
	cudaInput := append([]string{"-noautorotate"}, cudaDecodeInput()...)
	ref := previewPairArgs(req.Path, cpuInput, graphs.reference, len(ticks), "yuv420p10le")
	order := previewPairArgs(req.Path, cpuInput, graphs.cpuOrder, len(ticks), "yuv420p10le")
	cuda := previewPairArgs(req.Path, cudaInput, graphs.cuda, len(ticks), "yuv420p10le")
	out.CUDAAgainstReference, err = e.comparePreviewPlanes(ctx, req, first, ticks, ref, cuda)
	if err != nil {
		return fmt.Errorf("CUDA versus reference planes: %w", err)
	}
	out.CPUOrderAgainstRef, err = e.comparePreviewPlanes(ctx, req, first, ticks, ref, order)
	if err != nil {
		return fmt.Errorf("CPU geometry order planes: %w", err)
	}
	out.CUDAAgainstCPUOrder, err = e.comparePreviewPlanes(ctx, req, first, ticks, order, cuda)
	if err != nil {
		return fmt.Errorf("CUDA versus reordered CPU planes: %w", err)
	}
	return nil
}
