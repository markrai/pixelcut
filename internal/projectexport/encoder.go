package projectexport

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/markrai/pixelcut/internal/media"
)

func hlgVideoEncoderArgs(c media.CanvasSize, fps int, name string) []string {
	args := hlgVideoEncoderInput(c, fps)
	args = append(args, "-c:v", "libx265", "-profile:v", "main10", "-preset", "medium", "-crf", "16", "-x265-params", "pools=4:frame-threads=2:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-pix_fmt", "yuv420p10le")
	return append(args, hlgVideoEncoderOutput(name)...)
}

func hlgVideoEncoderInput(c media.CanvasSize, fps int) []string {
	return []string{"-v", "error", "-xerror", "-nostdin", "-filter_threads", "1", "-f", "rawvideo", "-pixel_format", "yuv444p10le", "-video_size", fmt.Sprintf("%dx%d", c.Width, c.Height), "-framerate", strconv.Itoa(fps), "-i", "pipe:0", "-map", "0:v:0", "-vf", "zscale=rangein=limited:range=limited:matrixin=2020_ncl:matrix=2020_ncl:primariesin=2020:primaries=2020:transferin=arib-std-b67:transfer=arib-std-b67:chromal=left:filter=lanczos,format=yuv420p10le,setsar=1"}
}

func hlgVideoEncoderOutput(name string) []string {
	return []string{"-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-an", "-fps_mode", "passthrough", "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1", "-video_track_timescale", "240000", "-movie_timescale", "240000", "-movflags", "+faststart", "-y", name}
}

type videoEncoderSpec struct {
	nvenc        bool
	experimental bool
	device       int
	qp           int
}

func projectEncoderSpec(engine media.Engine) (videoEncoderSpec, error) {
	backend, err := media.ParseEncodeBackend(string(engine.EncodeBackend))
	if err != nil {
		return videoEncoderSpec{}, err
	}
	decision, err := media.SelectVideoBackend(media.StageEncode, backend)
	if err != nil {
		return videoEncoderSpec{}, err
	}
	if decision.Selected == media.BackendNVENC {
		return videoEncoderSpec{nvenc: true, experimental: true, device: 0, qp: 16}, nil
	}
	return videoEncoderSpec{}, nil
}

func (s videoEncoderSpec) args(c media.CanvasSize, fps int, name string) ([]string, error) {
	if !s.nvenc {
		if s.experimental || s.device != 0 || s.qp != 0 {
			return nil, errors.New("CPU encoder does not accept NVENC options")
		}
		return hlgVideoEncoderArgs(c, fps, name), nil
	}
	contract := media.ExportContract{Color: "hlg", Canvas: c, FPS: fps}
	if contract.Validate() != nil || c.Width > 4096 || c.Height > 4096 || c.Width%2 != 0 || c.Height%2 != 0 || int64(c.Width)*int64(c.Height) > MaxExportPixels || s.device < 0 || s.device > 31 || s.qp < 0 || s.qp > 51 {
		return nil, errors.New("invalid NVENC encoder contract, device or QP")
	}
	args := hlgVideoEncoderInput(c, fps)
	args = append(args, "-c:v", "hevc_nvenc", "-gpu", strconv.Itoa(s.device), "-profile:v", "main10", "-preset", "p6", "-tune", "hq", "-rc", "constqp", "-qp", strconv.Itoa(s.qp), "-pix_fmt", "p010le")
	return append(args, hlgVideoEncoderOutput(name)...), nil
}

func (s videoEncoderSpec) name() string {
	if s.nvenc {
		if s.experimental {
			return "NVENC (experimental)"
		}
		return "NVENC (qualification only)"
	}
	return "CPU"
}

func (s videoEncoderSpec) contractArgs(contract media.ExportContract, name string) ([]string, error) {
	if contract.Validate() != nil {
		return nil, errors.New("invalid output color contract")
	}
	if contract.Color == media.ExportColorHLG {
		return s.args(contract.Canvas, contract.FPS, name)
	}
	c := contract.Canvas
	if c.Width > 4096 || c.Height > 4096 || c.Width%2 != 0 || c.Height%2 != 0 || int64(c.Width)*int64(c.Height) > MaxExportPixels || s.device < 0 || s.device > 31 || s.qp < 0 || s.qp > 51 {
		return nil, errors.New("invalid SDR encoder geometry, device or QP")
	}
	args := []string{"-v", "error", "-xerror", "-nostdin", "-filter_threads", "1", "-f", "rawvideo", "-pixel_format", "yuv444p10le", "-video_size", fmt.Sprintf("%dx%d", c.Width, c.Height), "-framerate", strconv.Itoa(contract.FPS), "-i", "pipe:0", "-map", "0:v:0", "-vf", "zscale=rangein=limited:range=limited:matrixin=709:matrix=709:primariesin=709:primaries=709:transferin=709:transfer=709:chromal=left:filter=lanczos,format=yuv420p,setsar=1"}
	if s.nvenc {
		args = append(args, "-c:v", "h264_nvenc", "-gpu", strconv.Itoa(s.device), "-profile:v", "high", "-preset", "p6", "-tune", "hq", "-rc", "constqp", "-qp", strconv.Itoa(s.qp), "-pix_fmt", "yuv420p")
	} else {
		if s.experimental || s.device != 0 || s.qp != 0 {
			return nil, errors.New("CPU encoder does not accept NVENC options")
		}
		args = append(args, "-c:v", "libx264", "-threads", "4", "-profile:v", "high", "-preset", "medium", "-crf", "16", "-x264-params", "colorprim=bt709:transfer=bt709:colormatrix=bt709:range=tv:chromaloc=0", "-pix_fmt", "yuv420p")
	}
	return append(args, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "avc1", "-an", "-fps_mode", "passthrough", "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1", "-video_track_timescale", "240000", "-movie_timescale", "240000", "-movflags", "+faststart+write_colr", "-y", name), nil
}
