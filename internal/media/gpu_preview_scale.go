package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"time"

	"github.com/markrai/pixelcut/internal/mediacache"
)

const maxPreviewScaleFrames = 120

func previewScaleCandidate(v Stream, req StillRequest) (string, error) {
	if err := validateCUDADecodeSource(v); err != nil {
		return "", err
	}
	rotation, err := v.DisplayRotation()
	if err != nil {
		return "", err
	}
	w, h := req.OutputWidth, req.OutputHeight
	if rotation != 0 || v.Range != "tv" || v.ChromaLocation != "left" || req.Crop != nil || req.Transform != nil ||
		req.Width != v.Width || req.Height != v.Height || req.Canvas != (CanvasSize{v.Width, v.Height}) ||
		w < 2 || h < 2 || w > MaxStillSide || h > MaxStillSide || w%2 != 0 || h%2 != 0 ||
		w > v.Width || h > v.Height || int64(w)*int64(v.Height) != int64(h)*int64(v.Width) {
		return "", errors.New("CUDA preview scale candidate requires uncropped unrotated limited-range left-sited HLG, source-sized canvas and even aspect-preserving downscale")
	}
	return fmt.Sprintf("scale_cuda=w=%d:h=%d:format=p010le:interp_algo=lanczos:passthrough=0,%s", w, h, cudaDownloadFilter), nil
}

func (e Engine) previewCUDAPipeline(v Stream, req StillRequest, temporal, graph string, candidate bool) ([]string, string, error) {
	if !candidate {
		return e.VideoDecodePipeline(v, temporal+","+graph, true)
	}
	if e.DecodeBackend != BackendCUDA {
		return nil, "", errors.New("CUDA preview scale candidate requires explicit CUDA decode")
	}
	prefix, err := previewScaleCandidate(v, req)
	if err != nil {
		return nil, "", err
	}
	return append([]string{"-noautorotate"}, cudaDecodeInput()...), prefix + "," + temporal + "," + previewToneMapGraph, nil
}

type PreviewScaleQuality struct {
	MinFramePSNRDB    float64 `json:"min_frame_psnr_db"`
	MaxChannelDelta   int     `json:"max_channel_delta"`
	MeanAbsoluteDelta float64 `json:"mean_absolute_delta"`
	ExactFrames       int     `json:"exact_frames"`
}

type PreviewScaleReport struct {
	Backend       string              `json:"backend"`
	Source        string              `json:"source"`
	FirstFrame    int                 `json:"first_frame"`
	Frames        int                 `json:"frames"`
	Compared      int                 `json:"compared"`
	OutputWidth   int                 `json:"output_width"`
	OutputHeight  int                 `json:"output_height"`
	Interpolation string              `json:"interpolation"`
	Status        string              `json:"status"`
	Failure       string              `json:"failure,omitempty"`
	Quality       PreviewScaleQuality `json:"quality"`
	WallSeconds   float64             `json:"wall_seconds"`
}

type previewQualityAccumulator struct {
	quality           PreviewScaleQuality
	absolute, samples uint64
}

func (a *previewQualityAccumulator) add(cpu, gpu []byte) error {
	if len(cpu) == 0 || len(cpu) != len(gpu) || len(cpu)%4 != 0 {
		return errors.New("invalid BGRA comparison frame")
	}
	var sum uint64
	var squares [3]uint64
	maxDelta := 0
	for i := 0; i < len(cpu); i += 4 {
		if cpu[i+3] != 255 || gpu[i+3] != 255 {
			return errors.New("nonopaque preview pixel")
		}
		for c := 0; c < 3; c++ {
			d := int(cpu[i+c]) - int(gpu[i+c])
			if d < 0 {
				d = -d
			}
			maxDelta = max(maxDelta, d)
			sum += uint64(d)
			squares[c] += uint64(d * d)
		}
	}
	psnr := 100.0
	for _, s := range squares {
		if s != 0 {
			psnr = min(psnr, 10*math.Log10(255*255/(float64(s)/float64(len(cpu)/4))))
		}
	}
	if a.samples == 0 || psnr < a.quality.MinFramePSNRDB {
		a.quality.MinFramePSNRDB = psnr
	}
	a.quality.MaxChannelDelta = max(a.quality.MaxChannelDelta, maxDelta)
	if maxDelta == 0 {
		a.quality.ExactFrames++
	}
	a.absolute += sum
	a.samples += uint64(len(cpu) / 4 * 3)
	a.quality.MeanAbsoluteDelta = float64(a.absolute) / float64(a.samples)
	return nil
}

func (q PreviewScaleQuality) withinLimits() bool {
	return q.MinFramePSNRDB >= 40 && q.MaxChannelDelta <= 16
}

func previewCheckArgs(e Engine, v Stream, req StillRequest, first, count int, cuda bool) ([]string, error) {
	if first < 0 || count < 1 || count > maxPreviewScaleFrames || first > MaxIndexedFrames-count {
		return nil, errors.New("invalid preview scale check interval (1-120 frames)")
	}
	if _, err := previewScaleCandidate(v, req); err != nil {
		return nil, err
	}
	graph, err := stillFilter(req)
	if err != nil {
		return nil, err
	}
	temporal := fmt.Sprintf("select=between(n\\,%d\\,%d)", first, first+count-1)
	input := []string{"-noautorotate", "-hwaccel", "none", "-c:v", "hevc"}
	graph = temporal + "," + graph
	if cuda {
		e.DecodeBackend = BackendCUDA
		input, graph, err = e.previewCUDAPipeline(v, req, temporal, "", true)
		if err != nil {
			return nil, err
		}
	}
	args := []string{"-v", "error", "-xerror", "-nostdin", "-threads", "2", "-filter_complex_threads", "1", "-copyts", "-start_at_zero"}
	args = append(args, input...)
	args = append(args, "-i", req.Path, "-filter_complex", "[0:v:0]"+graph+",split=2[pixels][hash]")
	for _, out := range []struct{ label, format, pipe string }{{"[pixels]", "rawvideo", "pipe:1"}, {"[hash]", "framehash", "pipe:2"}} {
		args = append(args, "-map", out.label, "-an", "-sn", "-dn", "-c:v", "rawvideo", "-pix_fmt", "bgra", "-threads", "1", "-fps_mode", "passthrough", "-enc_time_base", "1:1000000", "-frames:v", strconv.Itoa(count), "-f", out.format)
		if out.format == "framehash" {
			args = append(args, "-hash", "sha256")
		}
		args = append(args, out.pipe)
	}
	return args, nil
}

type previewCheckChild struct {
	cmd    *exec.Cmd
	pipe   io.ReadCloser
	hashes capabilityOutput
	waited bool
}

func startPreviewCheck(ctx context.Context, e Engine, args []string) (*previewCheckChild, error) {
	p := &previewCheckChild{cmd: exec.CommandContext(ctx, e.FFmpeg, args...)}
	p.cmd.WaitDelay = 2 * time.Second
	p.cmd.Stderr = &p.hashes
	pipe, err := p.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p.pipe = pipe
	if err = p.cmd.Start(); err != nil {
		_ = pipe.Close()
		return nil, err
	}
	return p, nil
}

func (p *previewCheckChild) finish(width, height, count int) ([]decodeFrameHash, error) {
	var extra [1]byte
	n, err := io.ReadFull(p.pipe, extra[:])
	if n != 0 || err != io.EOF {
		return nil, errors.New("unexpected extra or incomplete preview output")
	}
	err = p.cmd.Wait()
	p.waited = true
	if err != nil || p.hashes.overflow {
		return nil, fmt.Errorf("preview scale FFmpeg failed: %v %s", err, string(p.hashes.data[:min(len(p.hashes.data), 1024)]))
	}
	return parseFrameHashes(string(p.hashes.data), width, height, count, 4)
}

func (p *previewCheckChild) close() {
	_ = p.pipe.Close()
	if !p.waited {
		_ = p.cmd.Wait()
		p.waited = true
	}
}

func (e Engine) comparePreviewScale(ctx context.Context, v Stream, req StillRequest, first int, ticks []int64, hardware bool) (PreviewScaleQuality, int, error) {
	cpuArgs, err := previewCheckArgs(e, v, req, first, len(ticks), false)
	if err != nil {
		return PreviewScaleQuality{}, 0, err
	}
	gpuArgs, err := previewCheckArgs(e, v, req, first, len(ticks), hardware)
	if err != nil {
		return PreviewScaleQuality{}, 0, err
	}
	ctx, cancel := context.WithCancel(ctx)
	cpu, err := startPreviewCheck(ctx, e, cpuArgs)
	if err != nil {
		cancel()
		return PreviewScaleQuality{}, 0, err
	}
	defer func() { cancel(); cpu.close() }()
	gpu, err := startPreviewCheck(ctx, e, gpuArgs)
	if err != nil {
		return PreviewScaleQuality{}, 0, err
	}
	defer func() { cancel(); gpu.close() }()
	w, h := req.OutputWidth, req.OutputHeight
	c, g := make([]byte, w*h*4), make([]byte, w*h*4)
	ch, gh := make([][32]byte, len(ticks)), make([][32]byte, len(ticks))
	var quality previewQualityAccumulator
	for i := range ticks {
		if _, err := io.ReadFull(cpu.pipe, c); err != nil {
			return quality.quality, i, fmt.Errorf("CPU preview frame %d: %w", first+i, err)
		}
		if _, err := io.ReadFull(gpu.pipe, g); err != nil {
			return quality.quality, i, fmt.Errorf("candidate preview frame %d: %w", first+i, err)
		}
		ch[i], gh[i] = sha256.Sum256(c), sha256.Sum256(g)
		if err := quality.add(c, g); err != nil {
			return quality.quality, i, err
		}
	}
	cpuHashes, err := cpu.finish(w, h, len(ticks))
	if err != nil {
		return quality.quality, 0, err
	}
	gpuHashes, err := gpu.finish(w, h, len(ticks))
	if err != nil {
		return quality.quality, 0, err
	}
	for i, tick := range ticks {
		cf, gf := cpuHashes[i], gpuHashes[i]
		if tickDistance(cf.pts, tick) > 1 || cf.pts != gf.pts || cf.duration != gf.duration ||
			cf.hash != hex.EncodeToString(ch[i][:]) || gf.hash != hex.EncodeToString(gh[i][:]) {
			return quality.quality, i, fmt.Errorf("preview frame %d timestamp or raw hash mismatch", first+i)
		}
	}
	if err := ctx.Err(); err != nil {
		return quality.quality, 0, err
	}
	return quality.quality, len(ticks), nil
}

func (e Engine) CheckCUDAPreviewScale(ctx context.Context, path string, first, count, width, height int) (r PreviewScaleReport, err error) {
	r = PreviewScaleReport{Backend: "cuda", Source: path, FirstFrame: first, Frames: count, OutputWidth: width, OutputHeight: height, Interpolation: "lanczos", Status: "failed"}
	defer func() {
		if err != nil {
			r.Failure = err.Error()
		}
	}()
	if path == "" || first < 0 || count < 1 || count > maxPreviewScaleFrames || first > MaxIndexedFrames-count ||
		width < 2 || height < 2 || width > MaxStillSide || height > MaxStillSide || width%2 != 0 || height%2 != 0 {
		return r, errors.New("invalid preview scale dimensions or interval (even dimensions up to 960; 1-120 frames)")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	identity, err := mediacache.Identify(path)
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
	req := StillRequest{Path: identity.Path, Width: v.Width, Height: v.Height, Canvas: CanvasSize{v.Width, v.Height}, OutputWidth: width, OutputHeight: height}
	if _, err = previewScaleCandidate(v, req); err != nil {
		return r, err
	}
	index, err := e.ExportFrames(ctx, identity.Path)
	if err != nil {
		return r, err
	}
	if first >= index.Count() || count > index.Count()-first {
		return r, errors.New("preview scale interval exceeds source index")
	}
	started := time.Now()
	r.Quality, r.Compared, err = e.comparePreviewScale(ctx, v, req, first, index.ticks[first:first+count], true)
	r.WallSeconds = time.Since(started).Seconds()
	if err != nil {
		return r, err
	}
	if !unchanged(identity.Path, identity) {
		return r, errors.New("source changed during preview scale check")
	}
	if !r.Quality.withinLimits() {
		return r, errors.New("preview scale candidate exceeds provisional quality limits (worst frame/channel PSNR >= 40 dB, max channel delta <= 16)")
	}
	r.Status = "within_limits"
	return r, nil
}
