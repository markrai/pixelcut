package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"time"

	"github.com/markrai/pixelcut/internal/mediacache"
)

const CompositeFPS = 10
const CompositeStep int64 = 1000000 / CompositeFPS
const MaxCompositeStreams = 4

type previewStream struct {
	identity mediacache.Source
	graph    string
	width    int
	height   int
	next     int64
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	pipe     io.ReadCloser
}

func (s *previewStream) close() {
	s.cancel()
	_ = s.pipe.Close()
	_ = s.cmd.Wait()
}

type StreamDecoder struct {
	ctx     context.Context
	indexer *StillDecoder
	streams map[string]*previewStream
	stats   PreviewDecoderStats
}

func NewStreamDecoder(ctx context.Context, engine Engine) *StreamDecoder {
	return NewStreamDecoderWithCache(ctx, engine, nil)
}

func NewStreamDecoderWithCache(ctx context.Context, engine Engine, disk *mediacache.Cache) *StreamDecoder {
	return &StreamDecoder{ctx: ctx, indexer: NewStillDecoderWithCache(engine, disk), streams: make(map[string]*previewStream)}
}

func (d *StreamDecoder) Stats() PreviewDecoderStats { return d.stats }

func (d *StreamDecoder) retire(id string) {
	if s := d.streams[id]; s != nil {
		s.close()
		delete(d.streams, id)
		d.stats.Retired++
	}
}

func (d *StreamDecoder) Retain(ids []string) error {
	keep := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			return errors.New("missing composite clip identity")
		}
		keep[id] = true
	}
	if len(keep) > MaxCompositeStreams {
		return fmt.Errorf("continuous preview supports at most %d simultaneous video layers", MaxCompositeStreams)
	}
	for id := range d.streams {
		if !keep[id] {
			d.retire(id)
		}
	}
	return nil
}

func (d *StreamDecoder) Close() {
	for id := range d.streams {
		d.retire(id)
	}
}

func streamFilter(graph string, offset int64) string {
	return fmt.Sprintf("setpts=PTS-STARTPTS-%s/TB,tpad=stop_mode=clone:stop_duration=0.2,fps=%d:start_time=0:round=up:eof_action=pass,%s", seconds(float64(offset)/1000000), CompositeFPS, graph)
}

func (d *StreamDecoder) open(ctx context.Context, req StillRequest, identity mediacache.Source, graph string) (*previewStream, error) {
	source, err := d.indexer.source(ctx, identity, req.Path)
	if err != nil {
		return nil, err
	}
	if source.stream.Width != req.Width || source.stream.Height != req.Height || (req.Range != "" && req.Range != source.stream.Range) {
		return nil, errors.New("source geometry or color range changed; relink required")
	}
	if req.Transform != nil && source.stream.Range != "tv" {
		return nil, errors.New("canvas transforms require limited-range HLG")
	}
	at, ok := source.index.AtOrBefore(req.Tick)
	if !ok {
		return nil, errors.New("no source frame at or before requested time")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	run, cancel := context.WithCancel(d.ctx)
	cmd := exec.CommandContext(run, d.indexer.engine.FFmpeg, "-v", "error", "-nostdin", "-threads", "2", "-filter_threads", "1", "-ss", seconds(float64(at)/1000000), "-i", req.Path, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", streamFilter(graph, req.Tick-at), "-c:v", "rawvideo", "-pix_fmt", "bgra", "-threads", "1", "-f", "rawvideo", "-")
	cmd.WaitDelay = 2 * time.Second
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		cancel()
		_ = pipe.Close()
		return nil, err
	}
	return &previewStream{identity: identity, graph: graph, width: req.OutputWidth, height: req.OutputHeight, next: req.Tick, cancel: cancel, cmd: cmd, pipe: pipe}, nil
}

func (d *StreamDecoder) Decode(ctx context.Context, req StillRequest) (StillFrame, error) {
	if err := ctx.Err(); err != nil {
		return StillFrame{}, err
	}
	if err := d.ctx.Err(); err != nil {
		return StillFrame{}, err
	}
	if req.ClipID == "" || req.SourceEnd <= 0 || req.Tick < 0 || req.Tick >= req.SourceEnd {
		return StillFrame{}, errors.New("invalid continuous preview source interval")
	}
	graph, err := stillFilter(req)
	if err != nil {
		return StillFrame{}, err
	}
	identity, err := mediacache.Identify(req.Path)
	if err != nil {
		return StillFrame{}, err
	}
	s := d.streams[req.ClipID]
	if s != nil && s.identity != identity {
		return StillFrame{}, errors.New("source changed during continuous preview")
	}
	if s != nil && (s.next != req.Tick || s.graph != graph || s.width != req.OutputWidth || s.height != req.OutputHeight) {
		d.retire(req.ClipID)
		s = nil
	}
	if s == nil {
		if len(d.streams) >= MaxCompositeStreams {
			return StillFrame{}, errors.New("continuous preview decoder limit reached")
		}
		s, err = d.open(ctx, req, identity, graph)
		if err != nil {
			return StillFrame{}, err
		}
		d.streams[req.ClipID] = s
		d.stats.opened(len(d.streams))
	}
	stop := context.AfterFunc(ctx, s.cancel)
	pixels := make([]byte, s.width*s.height*4)
	_, err = io.ReadFull(s.pipe, pixels)
	stop()
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if d.ctx.Err() != nil {
		err = d.ctx.Err()
	}
	if err != nil || !unchanged(req.Path, identity) {
		d.retire(req.ClipID)
		if err != nil {
			return StillFrame{}, fmt.Errorf("continuous decoder: %w", err)
		}
		return StillFrame{}, errors.New("source changed during frame decode")
	}
	if s.next > math.MaxInt64-CompositeStep {
		s.next = -1
	} else {
		s.next += CompositeStep
	}
	d.stats.Units++
	return StillFrame{Width: s.width, Height: s.height, Pixels: pixels}, nil
}
