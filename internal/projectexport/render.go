package projectexport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

type sourceIndex struct {
	identity mediacache.Source
	index    *media.FrameIndex
}

type renderIndexes struct {
	entries []sourceIndex
}

func (r *renderIndexes) get(ctx context.Context, engine media.Engine, source Source) (*media.FrameIndex, error) {
	for i, entry := range r.entries {
		if entry.identity == source.Identity {
			copy(r.entries[i:], r.entries[i+1:])
			r.entries[len(r.entries)-1] = entry
			return entry.index, nil
		}
	}
	index, err := engine.ExportFrames(ctx, source.Identity.Path)
	if err != nil {
		return nil, err
	}
	if len(r.entries) >= MaxVideoLayers {
		copy(r.entries, r.entries[1:])
		r.entries = r.entries[:len(r.entries)-1]
	}
	r.entries = append(r.entries, sourceIndex{source.Identity, index})
	return index, nil
}

type layerDecoder struct {
	process *exportProcess
	reader  io.ReadCloser
	index   *media.FrameIndex
	layer   composition.PlannedLayer
	current int
	last    int
	pixels  []byte
}

func sourceTick(layer composition.PlannedLayer, tick, spanStart int64) int64 {
	return layer.Layer.SourceTime + (tick - spanStart)
}

func openLayer(ctx context.Context, engine media.Engine, indexes *renderIndexes, source Source, layer composition.PlannedLayer, span Span, plan Plan) (*layerDecoder, error) {
	index, err := indexes.get(ctx, engine, source)
	if err != nil {
		return nil, err
	}
	start, ok := plan.FrameTick(span.FirstFrame)
	end, endOK := plan.FrameTick(span.EndFrame - 1)
	if !ok || !endOK {
		return nil, errors.New("invalid span frame interval")
	}
	first, ok := index.OrdinalAtOrBefore(sourceTick(layer, start, span.Start))
	last, lastOK := index.OrdinalAtOrBefore(sourceTick(layer, end, span.Start))
	if !ok || !lastOK || last < first {
		return nil, errors.New("source has no frame at requested timestamp")
	}
	graph, err := layerFilter(layer, source, first, last)
	if err != nil {
		return nil, err
	}
	process := newExportProcess(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-threads", "2", "-filter_threads", "1", "-noautorotate", "-i", source.Identity.Path, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", graph, "-fps_mode", "passthrough", "-frames:v", strconv.Itoa(last-first+1), "-c:v", "rawvideo", "-pix_fmt", "yuv444p10le", "-threads", "1", "-f", "rawvideo", "-")
	reader, err := process.cmd.StdoutPipe()
	if err != nil {
		process.close()
		return nil, err
	}
	process.pipe = reader
	if err := process.cmd.Start(); err != nil {
		process.close()
		return nil, err
	}
	b := layer.Geometry.Bounds
	return &layerDecoder{process: process, reader: reader, index: index, layer: layer, current: first - 1, last: last, pixels: make([]byte, b.Width*b.Height*6)}, nil
}

func (d *layerDecoder) frame(tick int64) ([]byte, error) {
	target, ok := d.index.OrdinalAtOrBefore(tick)
	if !ok || target < d.current || target > d.last {
		return nil, errors.New("source frame selection outside decoder interval")
	}
	for d.current < target {
		if _, err := io.ReadFull(d.reader, d.pixels); err != nil {
			return nil, fmt.Errorf("read source frame: %w", err)
		}
		d.current++
	}
	return d.pixels, nil
}

func (d *layerDecoder) finish() error {
	if d.current != d.last {
		return errors.New("source decoder did not reach requested final frame")
	}
	var extra [1]byte
	if n, err := d.reader.Read(extra[:]); n != 0 || err != io.EOF {
		return errors.New("unexpected extra or incomplete raw source data")
	}
	return d.process.wait()
}

func renderSpan(ctx context.Context, engine media.Engine, indexes *renderIndexes, sources map[string]Source, plan Plan, span Span, dst io.Writer, black, frame []byte) error {
	if span.FirstFrame == span.EndFrame {
		return nil
	}
	var decoders []*layerDecoder
	defer func() {
		for _, decoder := range decoders {
			decoder.process.close()
		}
	}()
	for _, layer := range span.Layers {
		if layer.Geometry.Bounds.Width == 0 || layer.Geometry.Bounds.Height == 0 {
			continue
		}
		source, ok := sources[layer.Layer.Asset.ID]
		if !ok {
			return errors.New("missing prepared source")
		}
		now, err := mediacache.Identify(source.Identity.Path)
		if err != nil || now != source.Identity {
			return errors.New("source changed before span decode")
		}
		decoder, err := openLayer(ctx, engine, indexes, source, layer, span, plan)
		if err != nil {
			return fmt.Errorf("clip %s: %w", layer.Layer.Ref.ClipID, err)
		}
		decoders = append(decoders, decoder)
	}
	var layers [MaxVideoLayers]layerFrame
	for n := span.FirstFrame; n < span.EndFrame; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		tick, ok := plan.FrameTick(n)
		if !ok {
			return errors.New("invalid output frame position")
		}
		for i, d := range decoders {
			pixels, err := d.frame(sourceTick(d.layer, tick, span.Start))
			if err != nil {
				return err
			}
			layers[i] = layerFrame{Pixels: pixels, Bounds: d.layer.Geometry.Bounds, Opacity: d.layer.Layer.Opacity}
		}
		copy(frame, black)
		if err := compositeHLG(ctx, frame, plan.Contract.Canvas, layers[:len(decoders)]); err != nil {
			return err
		}
		if written, err := dst.Write(frame); err != nil {
			return err
		} else if written != len(frame) {
			return io.ErrShortWrite
		}
	}
	for _, decoder := range decoders {
		if err := decoder.finish(); err != nil {
			return err
		}
		source := sources[decoder.layer.Layer.Asset.ID]
		now, err := mediacache.Identify(source.Identity.Path)
		if err != nil || now != source.Identity {
			return errors.New("source changed during span decode")
		}
	}
	return ctx.Err()
}

func verifyVideoOutput(plan Plan, report media.Report) error {
	contract := plan.Contract
	contract.Audio = false
	if err := contract.VerifyOutput(report); err != nil {
		return err
	}
	v, err := report.Video()
	if err != nil {
		return err
	}
	count, err := strconv.ParseInt(v.FrameCount, 10, 64)
	if err != nil || count != plan.VideoFrames || v.Profile != "Main 10" {
		return errors.New("output frame count or HEVC profile does not match contract")
	}
	duration, err := project.ParseSeconds(v.Duration)
	want := (count*project.TicksPerSecond + int64(contract.FPS)/2) / int64(contract.FPS)
	if err != nil || duration < want-1 || duration > want+1 {
		return errors.New("output video duration does not match CFR frame count")
	}
	return nil
}

func RenderVideo(ctx context.Context, engine media.Engine, p project.Project, color string, fps int, discardAudio bool, output string) error {
	out, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(out), ".mp4") {
		return errors.New("output must have an .mp4 extension")
	}
	if _, err := os.Lstat(out); err == nil {
		return errors.New("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	prepared, err := Prepare(ctx, engine, p, color, fps)
	if err != nil {
		return err
	}
	plan := prepared.Plan
	if err := validateVideo(plan, discardAudio); err != nil {
		return err
	}
	sources := make(map[string]Source, len(prepared.Sources))
	for _, source := range prepared.Sources {
		sources[source.Asset.ID] = source
	}
	temp, err := os.CreateTemp(filepath.Dir(out), ".pixelcut-*.pixelcut-tmp.mp4")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Close(); err != nil {
		return err
	}
	c := plan.Contract.Canvas
	encoder := newExportProcess(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-filter_threads", "1", "-f", "rawvideo", "-pixel_format", "yuv444p10le", "-video_size", fmt.Sprintf("%dx%d", c.Width, c.Height), "-framerate", strconv.Itoa(fps), "-i", "pipe:0", "-map", "0:v:0", "-vf", "zscale=rangein=limited:range=limited:matrixin=2020_ncl:matrix=2020_ncl:primariesin=2020:primaries=2020:transferin=arib-std-b67:transfer=arib-std-b67:chromal=left:filter=lanczos,format=yuv420p10le,setsar=1", "-c:v", "libx265", "-profile:v", "main10", "-preset", "medium", "-crf", "16", "-x265-params", "pools=4:frame-threads=2:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-an", "-fps_mode", "passthrough", "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1", "-movflags", "+faststart", "-y", name)
	defer encoder.close()
	stdin, err := encoder.cmd.StdinPipe()
	if err != nil {
		return err
	}
	encoder.pipe = stdin
	if err := encoder.cmd.Start(); err != nil {
		return err
	}
	black := black444(c)
	frame := make([]byte, len(black))
	indexes := &renderIndexes{}
	for _, span := range plan.Spans {
		if err := renderSpan(ctx, engine, indexes, sources, plan, span, stdin, black, frame); err != nil {
			return fmt.Errorf("render span [%d,%d): %w", span.Start, span.End, err)
		}
	}
	if err := stdin.Close(); err != nil {
		return err
	}
	if err := encoder.wait(); err != nil {
		return err
	}
	report, err := engine.Inspect(ctx, name)
	if err != nil {
		return err
	}
	if err := verifyVideoOutput(plan, report); err != nil {
		return err
	}
	if err := prepared.VerifySources(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(name, out); err != nil {
		return fmt.Errorf("publish output (requires filesystem hard-link support): %w", err)
	}
	return nil
}
