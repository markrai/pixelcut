package projectexport

import (
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
