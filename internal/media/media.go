package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Stream struct {
	Index             int               `json:"index"`
	CodecType         string            `json:"codec_type"`
	CodecName         string            `json:"codec_name"`
	Profile           string            `json:"profile,omitempty"`
	CodecTag          string            `json:"codec_tag_string"`
	PixelFormat       string            `json:"pix_fmt"`
	Width             int               `json:"width"`
	Height            int               `json:"height"`
	Primaries         string            `json:"color_primaries"`
	Transfer          string            `json:"color_transfer"`
	Matrix            string            `json:"color_space"`
	Range             string            `json:"color_range"`
	ChromaLocation    string            `json:"chroma_location"`
	SampleAspectRatio string            `json:"sample_aspect_ratio"`
	AvgFrameRate      string            `json:"avg_frame_rate"`
	FrameCount        string            `json:"nb_frames,omitempty"`
	SampleRate        string            `json:"sample_rate,omitempty"`
	Channels          int               `json:"channels,omitempty"`
	Duration          string            `json:"duration"`
	TimeBase          string            `json:"time_base,omitempty"`
	StartPTS          *int64            `json:"start_pts,omitempty"`
	DurationTS        *int64            `json:"duration_ts,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"`
	SideData          []map[string]any  `json:"side_data_list,omitempty"`
	Disposition       map[string]int    `json:"disposition,omitempty"`
}

type Report struct {
	Streams []Stream       `json:"streams"`
	Format  map[string]any `json:"format"`
}

type Engine struct{ FFmpeg, FFprobe, MPV string }

func Default() Engine { return Engine{"ffmpeg", "ffprobe", "mpv"} }

func (e Engine) Inspect(ctx context.Context, path string) (Report, error) {
	var r Report
	p, err := filepath.Abs(path)
	if err != nil {
		return r, err
	}
	cmd := exec.CommandContext(ctx, e.FFprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", "-i", p)
	b, err := cmd.Output()
	if err != nil {
		return r, commandError(e.FFprobe, err)
	}
	err = json.Unmarshal(b, &r)
	return r, err
}

func commandError(name string, err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(exit.Stderr)))
	}
	return fmt.Errorf("%s: %w", name, err)
}

func (r Report) Video() (Stream, error) {
	var v Stream
	count := 0
	for _, s := range r.Streams {
		if s.CodecType == "video" {
			v = s
			count++
		}
	}
	if count != 1 || v.Disposition["attached_pic"] != 0 {
		return v, errors.New("exactly one ordinary video stream is required")
	}
	return v, nil
}

func (r Report) audioCount() int {
	n := 0
	for _, s := range r.Streams {
		if s.CodecType == "audio" {
			n++
		}
	}
	return n
}

func (v Stream) ValidateHLG() error {
	if v.PixelFormat != "yuv420p10le" || v.Primaries != "bt2020" || v.Transfer != "arib-std-b67" || v.Matrix != "bt2020nc" || (v.Range != "tv" && v.Range != "pc") {
		return errors.New("unsupported or incomplete color signaling; expected 10-bit 4:2:0 BT.2020 HLG with explicit range")
	}
	if v.ChromaLocation != "left" && v.ChromaLocation != "center" {
		return errors.New("unsupported or missing chroma location")
	}
	if v.SampleAspectRatio != "1:1" {
		return errors.New("only explicit square-pixel input is supported")
	}
	if v.Width <= 0 || v.Height <= 0 || v.Width%2 != 0 || v.Height%2 != 0 {
		return errors.New("invalid or odd input dimensions")
	}
	_, err := v.DisplayRotation()
	return err
}

type Crop struct{ X, Y, Width, Height int }

func (c Crop) Validate(v Stream) error {
	if c.X < 0 || c.Y < 0 || c.Width <= 0 || c.Height <= 0 || c.Width > v.Width || c.Height > v.Height || c.X > v.Width-c.Width || c.Y > v.Height-c.Height || c.X%2 != 0 || c.Y%2 != 0 || c.Width%2 != 0 || c.Height%2 != 0 {
		return errors.New("crop must fit the frame and use even coordinates and dimensions")
	}
	return nil
}

type Edit struct {
	Crop            *Crop
	Transform       *Transform
	Start, Duration float64
	Trim            bool
	Silent          bool
}

func (e Edit) validate(v Stream) error {
	if e.Crop != nil {
		if err := e.Crop.Validate(v); err != nil {
			return err
		}
	}
	if e.Trim && (math.IsNaN(e.Start) || math.IsNaN(e.Duration) || math.IsInf(e.Start, 0) || math.IsInf(e.Duration, 0) || e.Start < 0 || e.Duration <= 0) {
		return errors.New("trim requires a finite nonnegative start and positive duration")
	}
	if e.Transform != nil {
		if v.Range != "tv" {
			return errors.New("canvas transforms currently require limited-range HLG")
		}
		if _, _, _, err := VideoFilters(v.Width, v.Height, e.Crop, e.Transform); err != nil {
			return err
		}
	}
	if !e.Trim && e.Crop == nil && e.Transform == nil && !e.Silent {
		return errors.New("no edit requested")
	}
	if e.Trim {
		if duration, err := strconv.ParseFloat(v.Duration, 64); err == nil && e.Start+e.Duration > duration+0.001 {
			return errors.New("trim extends beyond video duration")
		}
	}
	return nil
}

func verifyColor(a, b Stream, width, height int) error {
	if a.PixelFormat != b.PixelFormat || a.Primaries != b.Primaries || a.Transfer != b.Transfer || a.Matrix != b.Matrix || a.Range != b.Range || a.ChromaLocation != b.ChromaLocation || a.SampleAspectRatio != b.SampleAspectRatio || b.Width != width || b.Height != height {
		return errors.New("output does not match the expected color and geometry contract")
	}
	return nil
}

func (e Engine) Render(ctx context.Context, input, output string, edit Edit) error {
	r, err := e.Inspect(ctx, input)
	if err != nil {
		return err
	}
	v, err := r.Video()
	if err != nil {
		return err
	}
	if err = v.ValidateHLG(); err != nil {
		return err
	}
	display, err := v.DisplayGeometry()
	if err != nil {
		return err
	}
	if err = edit.validate(display); err != nil {
		return err
	}
	if r.audioCount() > 1 {
		return errors.New("multiple audio streams are not supported by the renderer")
	}
	if err = r.validateExportStreams(); err != nil {
		return err
	}
	in, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-i", in}
	if edit.Trim {
		args = append(args, "-ss", seconds(edit.Start), "-t", seconds(edit.Duration))
	}
	args = append(args, "-map", "0:v:0")
	if edit.Silent {
		args = append(args, "-an")
	} else {
		args = append(args, "-map", "0:a:0?")
	}
	expected := r
	if edit.Silent {
		expected.Streams = []Stream{v}
	}
	graph, w, h, err := VideoFilters(display.Width, display.Height, edit.Crop, edit.Transform)
	if err != nil {
		return err
	}
	if graph != "" {
		args = append(args, "-vf", graph)
	}
	rangeName, chroma := "limited", "0"
	if v.Range == "pc" {
		rangeName = "full"
	}
	if v.ChromaLocation == "center" {
		chroma = "1"
	}
	x265 := "colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=" + rangeName + ":chromaloc=" + chroma
	args = append(args, "-c:v", "libx265", "-x265-params", x265, "-preset", "medium", "-crf", "16", "-pix_fmt", v.PixelFormat, "-color_primaries", v.Primaries, "-color_trc", v.Transfer, "-colorspace", v.Matrix, "-color_range", v.Range, "-chroma_sample_location", v.ChromaLocation, "-fps_mode", "passthrough", "-tag:v", "hvc1")
	if edit.Trim {
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	} else {
		args = append(args, "-c:a", "copy")
	}
	args = append(args, "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1", "-movflags", "+faststart")
	err = e.publish(ctx, output, args, func(out Report) error {
		b, err := out.Video()
		if err != nil {
			return err
		}
		if b.CodecName != "hevc" || out.audioCount() != expected.audioCount() {
			return errors.New("unexpected output codec or audio stream count")
		}
		if err = b.ValidateHLG(); err != nil {
			return err
		}
		rotation, err := b.DisplayRotation()
		if err != nil || rotation != 0 {
			return errors.New("rendered video must have normalized orientation")
		}
		if err = out.verifyExportStreams(expected, !edit.Trim); err != nil {
			return err
		}
		return verifyColor(v, b, w, h)
	})
	if err == nil {
		r.reportExcluded()
	}
	return err
}

func (e Engine) Remux(ctx context.Context, input, output string) error {
	return e.remux(ctx, input, output, false)
}

func (e Engine) RemuxVideoOnly(ctx context.Context, input, output string) error {
	return e.remux(ctx, input, output, true)
}

func (e Engine) remux(ctx context.Context, input, output string, silent bool) error {
	r, err := e.Inspect(ctx, input)
	if err != nil {
		return err
	}
	v, err := r.Video()
	if err != nil {
		return err
	}
	in, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	if err = r.validateExportStreams(); err != nil {
		return err
	}
	rotation, err := v.DisplayRotation()
	if err != nil {
		return err
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-i", in, "-map", "0:v:0"}
	expected := r
	if silent {
		args = append(args, "-an")
		expected.Streams = []Stream{v}
	} else {
		args = append(args, "-map", "0:a?")
	}
	args = append(args, "-c", "copy", "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1", "-movflags", "+faststart")
	err = e.publish(ctx, output, args, func(out Report) error {
		b, err := out.Video()
		if err != nil {
			return err
		}
		if b.CodecName != v.CodecName {
			return errors.New("remux changed video codec")
		}
		actual, err := b.DisplayRotation()
		if err != nil || actual != rotation {
			return errors.New("remux changed display orientation")
		}
		if err = out.verifyExportStreams(expected, true); err != nil {
			return err
		}
		return verifyColor(v, b, v.Width, v.Height)
	})
	if err == nil {
		r.reportExcluded()
	}
	return err
}

func seconds(n float64) string { return strconv.FormatFloat(n, 'f', 6, 64) }

func (e Engine) publish(ctx context.Context, output string, args []string, verify func(Report) error) error {
	out, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(out), ".mp4") {
		return errors.New("output must have an .mp4 extension")
	}
	if _, err = os.Lstat(out); err == nil {
		return errors.New("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(out), ".pixelcut-*.pixelcut-tmp.mp4")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, e.FFmpeg, append(args, "-y", name)...)
	if _, err = cmd.Output(); err != nil {
		return commandError(e.FFmpeg, err)
	}
	report, err := e.Inspect(ctx, name)
	if err != nil {
		return err
	}
	if err = verify(report); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Link(name, out); err != nil {
		return fmt.Errorf("publish output (requires filesystem hard-link support): %w", err)
	}
	return nil
}

func (e Engine) Preview(ctx context.Context, path string, hdr bool) error {
	p, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	args := []string{"--no-config", "--vo=gpu-next", "--hwdec=no"}
	if hdr {
		args = append(args, "--target-colorspace-hint=yes")
	} else {
		args = append(args, "--target-prim=bt.709", "--target-trc=bt.1886", "--tone-mapping=auto")
	}
	player, err := findMPV(e.MPV)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, player, append(args, "--", p)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
