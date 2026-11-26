package media

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/markrai/pixelcut/internal/mediacache"
)

const MaxDecodeCheckFrames = 300

type DecodeCheckReport struct {
	Backend    string  `json:"backend"`
	Source     string  `json:"source"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FirstFrame int     `json:"first_frame"`
	Frames     int     `json:"frames"`
	Matched    int     `json:"matched"`
	CPUSeconds float64 `json:"cpu_seconds"`
	GPUSeconds float64 `json:"gpu_seconds"`
	Status     string  `json:"status"`
	Failure    string  `json:"failure,omitempty"`
}

type decodeFrameHash struct {
	pts, duration, size int64
	hash                string
}

func hevcDecodeCheckArgs(path string, first, count int, cuda bool) ([]string, error) {
	return hevcFrameCheckArgs(path, first, count, cuda, nil)
}

func hevcFrameCheckArgs(path string, first, count int, cuda bool, scale *scaleCheckSize) ([]string, error) {
	if path == "" || first < 0 || count < 1 || count > MaxDecodeCheckFrames || first > MaxIndexedFrames-count {
		return nil, errors.New("invalid HEVC decode check interval")
	}
	args := []string{"-v", "error", "-xerror", "-nostdin", "-threads", "2", "-filter_threads", "1", "-copyts", "-start_at_zero", "-noautorotate"}
	graph := "format=yuv420p10le"
	if cuda {
		args = append(args, cudaDecodeInput()...)
		graph = cudaDownloadFilter
	} else {
		args = append(args, "-hwaccel", "none")
	}
	if scale != nil {
		if err := scale.validate(); err != nil {
			return nil, err
		}
		graph = scale.filter(cuda)
	}
	graph += fmt.Sprintf(",select=between(n\\,%d\\,%d)", first, first+count-1)
	if !cuda {
		args = append(args, "-c:v", "hevc")
	}
	return append(args, "-i", path, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", graph, "-fps_mode", "passthrough", "-frames:v", strconv.Itoa(count), "-c:v", "rawvideo", "-pix_fmt", "yuv420p10le", "-threads", "1", "-enc_time_base", "1:1000000", "-f", "framehash", "-hash", "sha256", "-"), nil
}

func parseDecodeFrameHashes(raw string, width, height, count int) ([]decodeFrameHash, error) {
	if width < 2 || height < 2 || width%2 != 0 || height%2 != 0 || width > 8192 || height > 8192 || int64(width)*int64(height) > 33554432 || count < 1 || count > MaxDecodeCheckFrames {
		return nil, errors.New("invalid frame hash dimensions or count")
	}
	sc := bufio.NewScanner(strings.NewReader(raw))
	var hashes []decodeFrameHash
	var timebase, dimensions, sha bool
	expectedSize := int64(width) * int64(height) * 3
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			switch {
			case strings.HasPrefix(line, "#tb "):
				timebase = line == "#tb 0: 1/1000000"
			case strings.HasPrefix(line, "#dimensions "):
				dimensions = line == fmt.Sprintf("#dimensions 0: %dx%d", width, height)
			case strings.HasPrefix(line, "#hash:"):
				sha = line == "#hash: SHA256"
			}
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) != 6 || strings.TrimSpace(fields[0]) != "0" || len(hashes) >= count {
			return nil, errors.New("unexpected frame hash row")
		}
		var n [4]int64
		for i := range n {
			v, err := strconv.ParseInt(strings.TrimSpace(fields[i+1]), 10, 64)
			if err != nil {
				return nil, errors.New("invalid frame hash timestamp or size")
			}
			n[i] = v
		}
		sum := strings.TrimSpace(fields[5])
		decoded, err := hex.DecodeString(sum)
		if err != nil || len(decoded) != 32 || n[0] != n[1] || n[1] < 0 || n[2] < 0 || n[3] != expectedSize || (len(hashes) > 0 && n[1] <= hashes[len(hashes)-1].pts) {
			return nil, errors.New("invalid or nonmonotonic 10-bit frame hash")
		}
		hashes = append(hashes, decodeFrameHash{pts: n[1], duration: n[2], size: n[3], hash: strings.ToLower(sum)})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !timebase || !dimensions || !sha || len(hashes) != count {
		return nil, errors.New("incomplete frame hash contract")
	}
	return hashes, nil
}

func (e Engine) decodeCheckFrames(ctx context.Context, path string, v Stream, first, count int, cuda bool) ([]decodeFrameHash, error) {
	return e.frameCheckFrames(ctx, path, v, first, count, cuda, nil)
}

func (e Engine) frameCheckFrames(ctx context.Context, path string, v Stream, first, count int, cuda bool, scale *scaleCheckSize) ([]decodeFrameHash, error) {
	args, err := hevcFrameCheckArgs(path, first, count, cuda, scale)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, e.FFmpeg, args...)
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr capabilityOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || stdout.overflow || stderr.overflow || len(stderr.data) != 0 {
		detail := strings.TrimSpace(string(stderr.data))
		if len(detail) > 1024 {
			detail = detail[:1024]
		}
		return nil, fmt.Errorf("HEVC decode failed: %v %s", err, detail)
	}
	width, height := v.Width, v.Height
	if scale != nil {
		width, height = scale.width, scale.height
	}
	return parseDecodeFrameHashes(string(stdout.data), width, height, count)
}

func compareDecodeHashes(cpu, gpu []decodeFrameHash, ticks []int64, first int) (int, error) {
	if len(cpu) == 0 || len(cpu) != len(gpu) || len(cpu) != len(ticks) {
		return 0, errors.New("frame hash or index count mismatch")
	}
	for i, c := range cpu {
		if tickDistance(c.pts, ticks[i]) > 1 {
			return i, fmt.Errorf("CPU frame %d does not match source timestamp index", first+i)
		}
		if c != gpu[i] {
			return i, fmt.Errorf("CUDA frame %d differs from CPU pixels or timing", first+i)
		}
	}
	return len(cpu), nil
}

func (e Engine) CheckHEVCDecode(ctx context.Context, path string, first, count int) (DecodeCheckReport, error) {
	return e.checkHEVCFrames(ctx, path, first, count, nil)
}

func (e Engine) checkHEVCFrames(ctx context.Context, path string, first, count int, scale *scaleCheckSize) (r DecodeCheckReport, err error) {
	r = DecodeCheckReport{Backend: "cuda", Source: path, FirstFrame: first, Frames: count, Status: "failed"}
	defer func() {
		if err != nil {
			r.Failure = err.Error()
		}
	}()
	if _, err := hevcFrameCheckArgs(path, first, count, true, scale); err != nil {
		return r, err
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
	if err = v.ValidateHLG(); err != nil {
		return r, err
	}
	if err = validateCUDADecodeSource(v); err != nil {
		return r, err
	}
	r.Width, r.Height = v.Width, v.Height
	index, err := e.ExportFrames(ctx, identity.Path)
	if err != nil {
		return r, err
	}
	if first >= index.Count() || count > index.Count()-first {
		return r, fmt.Errorf("requested frame interval exceeds %d indexed frames", index.Count())
	}
	started := time.Now()
	cpu, err := e.frameCheckFrames(ctx, identity.Path, v, first, count, false, scale)
	r.CPUSeconds = time.Since(started).Seconds()
	if err != nil {
		return r, err
	}
	if !unchanged(identity.Path, identity) {
		return r, errors.New("source changed after CPU decode")
	}
	started = time.Now()
	gpu, err := e.frameCheckFrames(ctx, identity.Path, v, first, count, true, scale)
	r.GPUSeconds = time.Since(started).Seconds()
	if err != nil {
		return r, err
	}
	if !unchanged(identity.Path, identity) {
		return r, errors.New("source changed after CUDA decode")
	}
	r.Matched, err = compareDecodeHashes(cpu, gpu, index.ticks[first:first+count], first)
	if err != nil {
		return r, err
	}
	r.Status = "matched"
	return r, nil
}
