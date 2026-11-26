package projectexport

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

func floorAudioFrames(ticks int64) int64 {
	q, r := ticks/project.TicksPerSecond, ticks%project.TicksPerSecond
	n := r * media.PCMRate
	tail := n / project.TicksPerSecond
	if n < 0 && n%project.TicksPerSecond != 0 {
		tail--
	}
	return q*media.PCMRate + tail
}

func audioInterval(a composition.AudioContribution) (first, end, sourceFirst int64, err error) {
	if !a.Timeline.Valid() || !a.SourceInterval.Valid() || a.Timeline.End-a.Timeline.Start != a.SourceInterval.End-a.SourceInterval.Start {
		return 0, 0, 0, errors.New("invalid audio clip interval")
	}
	first = media.PCMFramesForTicks(a.Timeline.Start)
	end = media.PCMFramesForTicks(a.Timeline.End)
	sourceFirst = first + floorAudioFrames(a.SourceInterval.Start-a.Timeline.Start)
	if sourceFirst < 0 || end < first {
		return 0, 0, 0, errors.New("invalid source sample interval")
	}
	return first, end, sourceFirst, nil
}

type audioStream struct {
	process *exportProcess
	reader  io.ReadCloser
	source  Source
	next    int64
	end     int64
}

type offlineAudio struct {
	ctx     context.Context
	engine  media.Engine
	sources map[string]Source
	streams map[string]*audioStream
}

func (d *offlineAudio) open(a composition.AudioContribution, pos int64) (*audioStream, error) {
	source, ok := d.sources[a.Asset.ID]
	if !ok {
		return nil, errors.New("missing prepared audio source")
	}
	now, err := mediacache.Identify(source.Identity.Path)
	if err != nil || now != source.Identity {
		return nil, errors.New("audio source changed before decode")
	}
	first, end, sourceFirst, err := audioInterval(a)
	if err != nil || pos != first || end <= first {
		return nil, errors.New("audio decoder must start at first clip sample")
	}
	sourceEnd := sourceFirst + end - first
	graph := "aresample=48000:async=1:first_pts=0,apad=whole_len=" + strconv.FormatInt(sourceEnd, 10) + ",atrim=start_sample=" + strconv.FormatInt(sourceFirst, 10) + ":end_sample=" + strconv.FormatInt(sourceEnd, 10) + ",asetpts=N/SR/TB"
	proc := newExportProcess(d.ctx, d.engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-threads", "2", "-filter_threads", "1", "-copyts", "-start_at_zero", "-i", source.Identity.Path, "-map", "0:a:0", "-vn", "-sn", "-dn", "-af", graph, "-ac", "2", "-ar", "48000", "-c:a", "pcm_s16le", "-f", "s16le", "-")
	reader, err := proc.cmd.StdoutPipe()
	if err != nil {
		proc.close()
		return nil, err
	}
	proc.pipe = reader
	if err := proc.cmd.Start(); err != nil {
		proc.close()
		return nil, err
	}
	return &audioStream{process: proc, reader: reader, source: source, next: first, end: end}, nil
}

func (d *offlineAudio) Read(a composition.AudioContribution, pos int64, frames int) ([]int16, error) {
	if err := d.ctx.Err(); err != nil {
		return nil, err
	}
	if frames <= 0 || frames > media.PCMBlockFrames {
		return nil, errors.New("invalid audio block size")
	}
	id := a.Ref.ClipID
	s := d.streams[id]
	if s == nil {
		if len(d.streams) >= media.MaxAudioStreams {
			return nil, errors.New("offline audio decoder limit reached")
		}
		var err error
		s, err = d.open(a, pos)
		if err != nil {
			return nil, err
		}
		d.streams[id] = s
	}
	if pos != s.next || int64(frames) > s.end-pos {
		return nil, errors.New("noncontiguous audio decoder request")
	}
	raw := make([]byte, frames*media.PCMChannels*2)
	if _, err := io.ReadFull(s.reader, raw); err != nil {
		return nil, fmt.Errorf("read offline PCM: %w", err)
	}
	s.next += int64(frames)
	samples := make([]int16, len(raw)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return samples, nil
}

func (d *offlineAudio) Retain(ids map[string]bool) error {
	for id, s := range d.streams {
		if ids[id] {
			continue
		}
		if s.next != s.end {
			return errors.New("audio decoder did not reach clip end")
		}
		var extra [1]byte
		if n, err := s.reader.Read(extra[:]); n != 0 || err != io.EOF {
			return errors.New("unexpected extra or incomplete PCM source data")
		}
		if err := s.process.wait(); err != nil {
			return err
		}
		s.process.close()
		delete(d.streams, id)
		now, err := mediacache.Identify(s.source.Identity.Path)
		if err != nil || now != s.source.Identity {
			return errors.New("audio source changed during decode")
		}
	}
	return d.ctx.Err()
}

func (d *offlineAudio) Close() {
	for id, s := range d.streams {
		s.process.close()
		delete(d.streams, id)
	}
}

type audioBlocks interface {
	Read(composition.AudioContribution, int64, int) ([]int16, error)
	Retain(map[string]bool) error
}

func writePCM(dst io.Writer, raw []byte) error {
	n, err := dst.Write(raw)
	if err != nil {
		return err
	}
	if n != len(raw) {
		return io.ErrShortWrite
	}
	return nil
}

func validateAudio(plan Plan) error {
	if plan.Contract.Validate() != nil || plan.Duration <= 0 || plan.Duration > MaxDuration || plan.VideoFrames != frameCeiling(plan.Duration, plan.Contract.FPS) || !plan.Contract.Audio || plan.ContentAudioFrames != media.PCMFramesForTicks(plan.Duration) || plan.OutputAudioFrames != plan.VideoFrames*int64(media.PCMRate/plan.Contract.FPS) || plan.OutputAudioFrames < plan.ContentAudioFrames {
		return errors.New("invalid offline audio duration")
	}
	for _, span := range plan.Spans {
		if span.FirstSample < span.EndSample && len(span.Audio) > media.MaxAudioStreams {
			return fmt.Errorf("offline export supports at most %d simultaneous audio contributions", media.MaxAudioStreams)
		}
	}
	return nil
}

func mixAudio(ctx context.Context, plan Plan, decoder audioBlocks, dst io.Writer) error {
	return mixAudioObserved(ctx, plan, decoder, dst, nil)
}

func mixAudioObserved(ctx context.Context, plan Plan, decoder audioBlocks, dst io.Writer, observe func(media.AudioLevels)) error {
	if err := validateAudio(plan); err != nil {
		return err
	}
	var pos int64
	var levels media.AudioLevels
	var mixer composition.PCMMixer
	mixed := make([]int16, media.PCMBlockFrames*media.PCMChannels)
	raw := make([]byte, len(mixed)*2)
	for _, span := range plan.Spans {
		if err := ctx.Err(); err != nil {
			return err
		}
		if span.FirstSample != pos || span.EndSample < pos || span.EndSample > plan.ContentAudioFrames {
			return errors.New("noncontiguous audio plan")
		}
		if pos == span.EndSample {
			continue
		}
		ids := make(map[string]bool, len(span.Audio))
		for _, a := range span.Audio {
			ids[a.Ref.ClipID] = true
		}
		if err := decoder.Retain(ids); err != nil {
			return err
		}
		for pos < span.EndSample {
			if err := ctx.Err(); err != nil {
				return err
			}
			n := int(min(int64(media.PCMBlockFrames), span.EndSample-pos))
			block := mixed[:n*media.PCMChannels]
			if err := mixer.Reset(n); err != nil {
				return err
			}
			for _, a := range span.Audio {
				samples, err := decoder.Read(a, pos, n)
				if err != nil {
					return fmt.Errorf("audio %s/%s: %w", a.Ref.TrackID, a.Ref.ClipID, err)
				}
				if err := mixer.AddContribution(samples, a, 0, pos); err != nil {
					return err
				}
			}
			if err := mixer.FinishObserved(block, &levels); err != nil {
				return err
			}
			for i, v := range block {
				binary.LittleEndian.PutUint16(raw[2*i:], uint16(v))
			}
			if err := writePCM(dst, raw[:len(block)*2]); err != nil {
				return err
			}
			pos += int64(n)
			if observe != nil {
				observe(levels)
			}
		}
	}
	if pos != plan.ContentAudioFrames {
		return errors.New("audio content sample count mismatch")
	}
	if err := decoder.Retain(nil); err != nil {
		return err
	}
	clear(raw)
	for pos < plan.OutputAudioFrames {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := min(int64(media.PCMBlockFrames), plan.OutputAudioFrames-pos)
		if err := writePCM(dst, raw[:int(n)*media.PCMChannels*2]); err != nil {
			return err
		}
		pos += n
		levels.Frames += n
		if observe != nil {
			observe(levels)
		}
	}
	return ctx.Err()
}

func muxAudio(ctx context.Context, engine media.Engine, plan Plan, sources map[string]Source, video, output string) error {
	return muxAudioObserved(ctx, engine, plan, sources, video, output, nil)
}

func muxAudioObserved(ctx context.Context, engine media.Engine, plan Plan, sources map[string]Source, video, output string, observe Observer) error {
	if err := validateAudio(plan); err != nil {
		return err
	}
	proc := newExportProcess(ctx, engine.FFmpeg, "-v", "error", "-xerror", "-nostdin", "-i", video, "-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "pipe:0", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-tag:v", "hvc1", "-c:a", "aac", "-b:a", "192k", "-video_track_timescale", "240000", "-movie_timescale", "240000", "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1", "-movflags", "+faststart", "-y", output)
	defer proc.close()
	stdin, err := proc.cmd.StdinPipe()
	if err != nil {
		return err
	}
	proc.pipe = stdin
	if err := proc.cmd.Start(); err != nil {
		return err
	}
	decoder := &offlineAudio{ctx: ctx, engine: engine, sources: sources, streams: make(map[string]*audioStream)}
	defer decoder.Close()
	emit(observe, StageAudio, 0, plan.OutputAudioFrames)
	if err := mixAudioObserved(ctx, plan, decoder, stdin, func(levels media.AudioLevels) {
		if observe != nil {
			observe(Progress{Stage: StageAudio, Completed: levels.Frames, Total: plan.OutputAudioFrames, AudioLevels: levels})
		}
	}); err != nil {
		return err
	}
	emit(observe, StageAudioWait, 0, 0)
	if err := stdin.Close(); err != nil {
		return err
	}
	return proc.wait()
}
