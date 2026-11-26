package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/markrai/pixelcut/internal/mediacache"
)

const MaxStillSide = 960
const stillCacheBytes = 24 * 1024 * 1024
const stillCacheEntries = 64

type StillRequest struct {
	Path          string
	ClipID        string
	Tick          int64
	SourceEnd     int64
	Width, Height int
	Range         string
	Canvas        CanvasSize
	Crop          *Crop
	Transform     *Transform
	OutputWidth   int
	OutputHeight  int
}

type StillFrame struct {
	Width, Height int
	Pixels        []byte
}

func (f StillFrame) Valid() bool {
	return f.Width > 0 && f.Height > 0 && f.Width <= MaxStillSide && f.Height <= MaxStillSide && len(f.Pixels) == f.Width*f.Height*4
}

func (f *FrameIndex) AtOrBefore(tick int64) (int64, bool) {
	if f == nil || tick < 0 {
		return 0, false
	}
	i := sort.Search(len(f.ticks), func(i int) bool { return f.ticks[i] > tick })
	if i == 0 {
		return 0, false
	}
	return f.ticks[i-1], true
}

func stillFilter(req StillRequest) (string, error) {
	if req.Tick < 0 || req.Path == "" || req.OutputWidth < 1 || req.OutputHeight < 1 || req.OutputWidth > MaxStillSide || req.OutputHeight > MaxStillSide {
		return "", errors.New("invalid still frame request")
	}
	g, err := PlanLayer(req.Canvas, req.Width, req.Height, req.Crop, req.Transform)
	if err != nil {
		return "", err
	}
	p := g.Content
	if g.Bounds.Width == 0 || g.Bounds.Height == 0 {
		return "", errors.New("layer has no visible pixels")
	}
	if int64(p.ScaledWidth)*int64(p.ScaledHeight) > 16777216 {
		return "", errors.New("layer exceeds static preview intermediate frame budget")
	}
	var filters []string
	if c := req.Crop; c != nil {
		filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d", c.Width, c.Height, c.X, c.Y))
	}
	if t := req.Transform; t != nil {
		switch t.Rotation {
		case 90:
			filters = append(filters, "transpose=clock")
		case 180:
			filters = append(filters, "hflip", "vflip")
		case 270:
			filters = append(filters, "transpose=cclock")
		}
		if t.FlipHorizontal {
			filters = append(filters, "hflip")
		}
		if t.FlipVertical {
			filters = append(filters, "vflip")
		}
	}
	filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=lanczos", p.ScaledWidth, p.ScaledHeight))
	if p.SourceX != 0 || p.SourceY != 0 || p.Width != p.ScaledWidth || p.Height != p.ScaledHeight {
		filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d", p.Width, p.Height, p.SourceX, p.SourceY))
	}
	w, h := max(2, (req.OutputWidth+1)&^1), max(2, (req.OutputHeight+1)&^1)
	filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h), "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709", "tonemap=mobius:desat=2", "zscale=t=bt709:m=bt709:r=limited", "format=yuv420p", "format=bgra")
	if w != req.OutputWidth || h != req.OutputHeight {
		filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=lanczos", req.OutputWidth, req.OutputHeight))
	}
	return strings.Join(filters, ","), nil
}

type stillKey struct {
	source mediacache.Source
	tick   int64
	filter string
}

type stillEntry struct {
	key   stillKey
	frame StillFrame
}

type stillSource struct {
	source mediacache.Source
	stream Stream
	raw    Stream
	index  *FrameIndex
}

type StillDecoder struct {
	engine  Engine
	disk    *mediacache.Cache
	sources []stillSource
	frames  []stillEntry
	bytes   int
}

func NewStillDecoder(engine Engine) *StillDecoder {
	return NewStillDecoderWithCache(engine, nil)
}

func NewStillDecoderWithCache(engine Engine, disk *mediacache.Cache) *StillDecoder {
	return &StillDecoder{engine: engine, disk: disk}
}

func (d *StillDecoder) source(ctx context.Context, identity mediacache.Source, path string) (stillSource, error) {
	for i, s := range d.sources {
		if s.source == identity {
			copy(d.sources[i:], d.sources[i+1:])
			d.sources[len(d.sources)-1] = s
			return s, nil
		}
	}
	r, err := d.engine.Inspect(ctx, path)
	if err != nil {
		return stillSource{}, err
	}
	v, err := r.Video()
	if err != nil {
		return stillSource{}, err
	}
	if err = v.ValidateHLG(); err != nil {
		return stillSource{}, err
	}
	raw := v
	if _, _, err := d.engine.VideoDecodePipeline(raw, "", false); err != nil {
		return stillSource{}, err
	}
	v, err = v.DisplayGeometry()
	if err != nil {
		return stillSource{}, err
	}
	if v.Width > 16384 || v.Height > 16384 || int64(v.Width)*int64(v.Height) > 33554432 {
		return stillSource{}, errors.New("source exceeds static preview frame budget")
	}
	var index *FrameIndex
	if d.engine.DecodeBackend == BackendCUDA {
		index, err = d.engine.ExportFrames(ctx, path)
	} else {
		index, err = d.engine.CachedFrames(ctx, d.disk, path)
	}
	if err != nil {
		return stillSource{}, err
	}
	if !unchanged(path, identity) {
		return stillSource{}, errors.New("source changed during frame indexing")
	}
	s := stillSource{source: identity, stream: v, raw: raw, index: index}
	if len(d.sources) >= 4 {
		copy(d.sources, d.sources[1:])
		d.sources[len(d.sources)-1] = stillSource{}
		d.sources = d.sources[:len(d.sources)-1]
	}
	d.sources = append(d.sources, s)
	return s, nil
}

func cloneStill(f StillFrame) StillFrame {
	f.Pixels = append([]byte(nil), f.Pixels...)
	return f
}

func (d *StillDecoder) store(key stillKey, frame StillFrame) {
	if len(frame.Pixels) > stillCacheBytes {
		return
	}
	for len(d.frames) > 0 && (d.bytes+len(frame.Pixels) > stillCacheBytes || len(d.frames) >= stillCacheEntries) {
		d.bytes -= len(d.frames[0].frame.Pixels)
		d.frames[0] = stillEntry{}
		d.frames = d.frames[1:]
	}
	d.frames = append(d.frames, stillEntry{key: key, frame: cloneStill(frame)})
	d.bytes += len(frame.Pixels)
}

func (d *StillDecoder) Decode(ctx context.Context, req StillRequest) (StillFrame, error) {
	if err := ctx.Err(); err != nil {
		return StillFrame{}, err
	}
	graph, err := stillFilter(req)
	if err != nil {
		return StillFrame{}, err
	}
	identity, err := mediacache.Identify(req.Path)
	if err != nil {
		return StillFrame{}, err
	}
	s, err := d.source(ctx, identity, req.Path)
	if err != nil {
		return StillFrame{}, err
	}
	if s.stream.Width != req.Width || s.stream.Height != req.Height || (req.Range != "" && req.Range != s.stream.Range) {
		return StillFrame{}, errors.New("source geometry or color range changed; relink required")
	}
	if req.Transform != nil && s.stream.Range != "tv" {
		return StillFrame{}, errors.New("canvas transforms require limited-range HLG")
	}
	tick, ok := s.index.AtOrBefore(req.Tick)
	if !ok {
		return StillFrame{}, errors.New("no source frame at or before requested time")
	}
	key := stillKey{source: identity, tick: tick, filter: graph}
	for i, entry := range d.frames {
		if entry.key == key {
			if !unchanged(req.Path, identity) {
				return StillFrame{}, errors.New("source changed during still preview")
			}
			copy(d.frames[i:], d.frames[i+1:])
			d.frames[len(d.frames)-1] = entry
			return cloneStill(entry.frame), nil
		}
	}
	args := []string{"-v", "error", "-nostdin", "-threads", "2", "-filter_threads", "1"}
	if d.engine.DecodeBackend == BackendCUDA {
		ordinal, ok := s.index.OrdinalAtOrBefore(req.Tick)
		if !ok {
			return StillFrame{}, errors.New("missing CUDA source ordinal")
		}
		input, filter, err := d.engine.VideoDecodePipeline(s.raw, fmt.Sprintf("select=eq(n\\,%d),%s", ordinal, graph), true)
		if err != nil {
			return StillFrame{}, err
		}
		args = append(args, "-xerror", "-copyts", "-start_at_zero")
		args = append(args, input...)
		graph = filter
	} else {
		args = append(args, "-ss", seconds(float64(tick)/1000000))
	}
	args = append(args, "-i", req.Path, "-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn", "-vf", graph, "-c:v", "rawvideo", "-pix_fmt", "bgra", "-threads", "1", "-f", "rawvideo", "-")
	cmd := exec.CommandContext(ctx, d.engine.FFmpeg, args...)
	if d.engine.DecodeBackend == BackendCUDA {
		cmd.WaitDelay = 2 * time.Second
	}
	b, err := cmd.Output()
	if err != nil {
		return StillFrame{}, fmt.Errorf("%s still decode: %w", d.engine.DecodeBackendName(), commandError(d.engine.FFmpeg, err))
	}
	frame := StillFrame{Width: req.OutputWidth, Height: req.OutputHeight, Pixels: b}
	if !frame.Valid() {
		return StillFrame{}, errors.New("decoded still frame unavailable")
	}
	if err = ctx.Err(); err != nil {
		return StillFrame{}, err
	}
	if !unchanged(req.Path, identity) {
		return StillFrame{}, errors.New("source changed during still preview")
	}
	d.store(key, frame)
	return frame, nil
}
