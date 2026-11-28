package media

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
)

func (e Engine) Thumbnail(ctx context.Context, path string, at float64, w, h int) ([]byte, error) {
	if math.IsNaN(at) || math.IsInf(at, 0) || at < 0 || w < 2 || h < 2 || w > 256 || h > 256 || w%2 != 0 || h%2 != 0 {
		return nil, errors.New("invalid thumbnail request")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	report, err := e.Inspect(ctx, p)
	if err != nil {
		return nil, err
	}
	v, err := report.Video()
	if err != nil {
		return nil, err
	}
	color, err := v.ClassifySourceColor()
	if err != nil {
		return nil, err
	}
	graph := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=mobius:desat=2,zscale=t=bt709:m=bt709:r=limited,format=yuv420p,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,format=bgra", w, h, w, h)
	if color.Family == ColorSDR709 {
		if err := report.ValidateClassifiedImportStreams(); err != nil {
			return nil, err
		}
		convert, err := sdrPreviewGraph(color)
		if err != nil {
			return nil, err
		}
		graph = fmt.Sprintf("%s,scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,format=bgra", convert, w, h, w, h)
	} else if color.Family != ColorHLG {
		return nil, errors.New("PQ thumbnail unavailable")
	}
	cmd := exec.CommandContext(ctx, e.FFmpeg, "-v", "error", "-nostdin", "-threads", "2", "-filter_threads", "1", "-ss", seconds(at), "-i", p, "-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn", "-vf", graph, "-c:v", "rawvideo", "-threads", "1", "-f", "rawvideo", "-")
	b, err := cmd.Output()
	if err != nil {
		return nil, commandError(e.FFmpeg, err)
	}
	if len(b) != w*h*4 {
		return nil, errors.New("thumbnail frame unavailable")
	}
	return b, nil
}
