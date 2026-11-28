package media

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const ExportColorHLG = "hlg"
const ExportColorSDR = "sdr"

type ExportContract struct {
	Color  string     `json:"color"`
	Canvas CanvasSize `json:"canvas"`
	FPS    int        `json:"fps"`
	Audio  bool       `json:"audio"`
}

func (c ExportContract) Validate() error {
	if c.Color != ExportColorHLG && c.Color != ExportColorSDR {
		return errors.New("explicit HLG or SDR export color contract required")
	}
	if err := c.Canvas.Validate(); err != nil {
		return err
	}
	switch c.FPS {
	case 24, 25, 30, 50, 60:
		return nil
	default:
		return errors.New("export FPS must be 24, 25, 30, 50 or 60")
	}
}

func (c ExportContract) ExpectedVideo() Stream {
	if c.Color == ExportColorSDR {
		return Stream{CodecType: "video", CodecName: "h264", CodecTag: "avc1", PixelFormat: "yuv420p", Width: c.Canvas.Width, Height: c.Canvas.Height, Primaries: "bt709", Transfer: "bt709", Matrix: "bt709", Range: "tv", ChromaLocation: "left", SampleAspectRatio: "1:1"}
	}
	return Stream{CodecType: "video", CodecName: "hevc", CodecTag: "hvc1", PixelFormat: "yuv420p10le", Width: c.Canvas.Width, Height: c.Canvas.Height, Primaries: "bt2020", Transfer: "arib-std-b67", Matrix: "bt2020nc", Range: "tv", ChromaLocation: "left", SampleAspectRatio: "1:1"}
}

func (c ExportContract) VerifyOutput(r Report) error {
	if err := c.Validate(); err != nil {
		return err
	}
	v, err := r.Video()
	if err != nil {
		return err
	}
	if c.Color == ExportColorHLG {
		if err = v.ValidateHLG(); err != nil {
			return err
		}
	} else if v.HasStaticHDRMetadata() {
		return errors.New("SDR output must not contain static HDR metadata")
	}
	want := c.ExpectedVideo()
	if v.CodecName != want.CodecName || v.CodecTag != want.CodecTag {
		return fmt.Errorf("expected %s %s output", want.CodecName, want.CodecTag)
	}
	if err = verifyColor(want, v, c.Canvas.Width, c.Canvas.Height); err != nil {
		return err
	}
	if rotation, err := v.DisplayRotation(); err != nil || rotation != 0 {
		return errors.New("export orientation must be normalized")
	}
	num, den, ok := strings.Cut(v.AvgFrameRate, "/")
	n, ne := strconv.ParseInt(num, 10, 32)
	d, de := strconv.ParseInt(den, 10, 32)
	if !ok || ne != nil || de != nil || d <= 0 || n != int64(c.FPS)*d {
		return fmt.Errorf("output frame rate does not match %d fps", c.FPS)
	}
	expected := Report{Streams: []Stream{want}}
	if c.Audio {
		for _, stream := range r.Streams {
			if stream.CodecType == "audio" && (stream.SampleRate != "48000" || stream.Channels != 2) {
				return errors.New("expected 48 kHz stereo AAC output")
			}
		}
		expected.Streams = append(expected.Streams, Stream{CodecType: "audio", CodecName: "aac"})
	}
	return r.verifyExportStreams(expected, true)
}

func (r Report) ValidateProjectExportSource() error {
	v, err := r.Video()
	if err != nil {
		return err
	}
	if err = v.ValidateHLG(); err != nil {
		return err
	}
	if r.audioCount() > 1 {
		return errors.New("project export does not support multiple source audio streams")
	}
	return r.validateExportStreams()
}

func (r Report) ExcludedProjectMetadata() int {
	n := 0
	for _, stream := range r.Streams {
		if stream.pixelMetadata() {
			n++
		}
	}
	return n
}
