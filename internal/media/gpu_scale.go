package media

import (
	"context"
	"errors"
	"fmt"
)

type scaleCheckSize struct {
	width, height int
}

func (s scaleCheckSize) validate() error {
	if s.width < 2 || s.height < 2 || s.width%2 != 0 || s.height%2 != 0 || s.width > 8192 || s.height > 8192 || int64(s.width)*int64(s.height) > 33554432 {
		return errors.New("scale check requires positive even output dimensions within 8192 per side and 33554432 pixels")
	}
	return nil
}

func (s scaleCheckSize) filter(cuda bool) string {
	if cuda {
		return fmt.Sprintf("scale_cuda=w=%d:h=%d:format=p010le:interp_algo=nearest:passthrough=0,%s", s.width, s.height, cudaDownloadFilter)
	}
	return fmt.Sprintf("format=yuv420p10le,scale=w=%d:h=%d:flags=neighbor,format=yuv420p10le", s.width, s.height)
}

type ScaleCheckReport struct {
	DecodeCheckReport
	OutputWidth           int    `json:"output_width"`
	OutputHeight          int    `json:"output_height"`
	Interpolation         string `json:"interpolation"`
	SourceBytesPerFrame   int64  `json:"source_bytes_per_frame,omitempty"`
	DownloadBytesPerFrame int64  `json:"download_bytes_per_frame,omitempty"`
}

func (e Engine) CheckCUDAScale(ctx context.Context, path string, first, count, width, height int) (ScaleCheckReport, error) {
	size := scaleCheckSize{width: width, height: height}
	decoded, err := e.checkHEVCFrames(ctx, path, first, count, &size)
	r := ScaleCheckReport{
		DecodeCheckReport: decoded,
		OutputWidth:       width,
		OutputHeight:      height,
		Interpolation:     "nearest",
	}
	if decoded.Width > 0 && size.validate() == nil {
		r.SourceBytesPerFrame = int64(decoded.Width) * int64(decoded.Height) * 3
		r.DownloadBytesPerFrame = int64(width) * int64(height) * 3
	}
	return r, err
}
