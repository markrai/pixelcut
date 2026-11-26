package media

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"time"

	"github.com/markrai/pixelcut/internal/mediacache"
)

const PCMRate = 48000
const PCMChannels = 2
const PCMBlockFrames = 2400
const MaxAudioStreams = 8

func PCMFramesForTicks(ticks int64) int64 {
	if ticks <= 0 {
		return 0
	}
	return ticks/1000000*PCMRate + (ticks%1000000*PCMRate+999999)/1000000
}

func PCMTicksForFrames(frames int64) int64 {
	if frames <= 0 {
		return 0
	}
	seconds, tail := frames/PCMRate, frames%PCMRate*1000000/PCMRate
	if seconds > (math.MaxInt64-tail)/1000000 {
		return math.MaxInt64
	}
	return seconds*1000000 + tail
}

type PCMRequest struct {
	ClipID, Path           string
	Tick, Offset           int64
	Frames                 int
	SourceFrame, SourceEnd int64
}

type pcmStream struct {
	identity              mediacache.Source
	next, sourceNext, end int64
	cancel                context.CancelFunc
	cmd                   *exec.Cmd
	pipe                  io.ReadCloser
	waited                bool
}

func (s *pcmStream) close() {
	s.cancel()
	_ = s.pipe.Close()
	if !s.waited {
		_ = s.cmd.Wait()
		s.waited = true
	}
}

type PCMDecoder struct {
	ctx        context.Context
	engine     Engine
	streams    map[string]*pcmStream
	identities map[string]mediacache.Source
	stats      PreviewDecoderStats
}

func NewPCMDecoder(ctx context.Context, engine Engine) *PCMDecoder {
	return &PCMDecoder{ctx: ctx, engine: engine, streams: make(map[string]*pcmStream), identities: make(map[string]mediacache.Source)}
}

func (d *PCMDecoder) Stats() PreviewDecoderStats { return d.stats }

func (d *PCMDecoder) retire(id string) {
	if s := d.streams[id]; s != nil {
		s.close()
		delete(d.streams, id)
		d.stats.Retired++
	}
}

func (d *PCMDecoder) Retain(ids []string) error {
	keep := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			return errors.New("missing audio clip identity")
		}
		keep[id] = true
	}
	if len(keep) > MaxAudioStreams {
		return fmt.Errorf("composite preview supports at most %d simultaneous audio contributions", MaxAudioStreams)
	}
	for id := range d.streams {
		if !keep[id] {
			d.retire(id)
		}
	}
	return nil
}

func (d *PCMDecoder) Close() {
	_ = d.Retain(nil)
}

func (d *PCMDecoder) open(req PCMRequest, identity mediacache.Source) (*pcmStream, error) {
	args, err := PCMDecodeArgs(identity.Path, req.SourceFrame, req.SourceEnd)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(d.ctx)
	cmd := exec.CommandContext(ctx, d.engine.FFmpeg, args...)
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
	return &pcmStream{identity: identity, next: req.Offset, sourceNext: req.SourceFrame, end: req.SourceEnd, cancel: cancel, cmd: cmd, pipe: pipe}, nil
}

func (d *PCMDecoder) DecodePCM(ctx context.Context, req PCMRequest) ([]int16, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := d.ctx.Err(); err != nil {
		return nil, err
	}
	if req.ClipID == "" || req.Path == "" || req.Tick < 0 || req.Offset < 0 || req.Frames < 1 || req.Frames > PCMBlockFrames || req.Offset > math.MaxInt64-int64(req.Frames) || req.SourceFrame < 0 || req.SourceEnd <= req.SourceFrame || int64(req.Frames) > req.SourceEnd-req.SourceFrame {
		return nil, errors.New("invalid PCM request")
	}
	identity, err := mediacache.Identify(req.Path)
	if err != nil {
		return nil, err
	}
	if prior, ok := d.identities[identity.Path]; ok && prior != identity {
		return nil, errors.New("audio source changed during playback")
	}
	d.identities[identity.Path] = identity
	s := d.streams[req.ClipID]
	if s != nil && (s.identity != identity || s.next != req.Offset || s.sourceNext != req.SourceFrame || s.end != req.SourceEnd) {
		d.retire(req.ClipID)
		s = nil
	}
	if s == nil {
		if len(d.streams) >= MaxAudioStreams {
			return nil, errors.New("audio decoder limit reached")
		}
		s, err = d.open(req, identity)
		if err != nil {
			return nil, err
		}
		d.streams[req.ClipID] = s
		d.stats.opened(len(d.streams))
	}
	stop := context.AfterFunc(ctx, s.cancel)
	raw := make([]byte, req.Frames*PCMChannels*2)
	_, err = io.ReadFull(s.pipe, raw)
	if err == nil && req.SourceFrame+int64(req.Frames) == req.SourceEnd {
		var extra [1]byte
		if n, readErr := s.pipe.Read(extra[:]); n != 0 || readErr != io.EOF {
			err = errors.New("unexpected extra or incomplete PCM source data")
		} else {
			err = s.cmd.Wait()
			s.waited = true
		}
	}
	stop()
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if d.ctx.Err() != nil {
		err = d.ctx.Err()
	}
	if err != nil || !unchanged(req.Path, identity) {
		d.retire(req.ClipID)
		if err != nil {
			return nil, fmt.Errorf("audio decoder: %w", err)
		}
		return nil, errors.New("audio source changed during decode")
	}
	out := make([]int16, len(raw)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(raw[i*2:]))
	}
	s.next += int64(req.Frames)
	s.sourceNext += int64(req.Frames)
	d.stats.Units += int64(req.Frames)
	if s.waited {
		d.retire(req.ClipID)
	}
	return out, nil
}
