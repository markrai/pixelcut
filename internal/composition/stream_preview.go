package composition

import (
	"context"
	"errors"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
)

type SequentialDecoder interface {
	StillDecoder
	Retain([]string) error
}

type CompositeFrame struct {
	Time, ValidUntil int64
	Frame            media.StillFrame
	Sources          map[string]mediacache.Source
	End              bool
}

const CompositePreviewSide = 640

func NextCompositeTick(at, end int64) int64 {
	if at >= end || end-at <= media.CompositeStep {
		return end
	}
	return at + media.CompositeStep
}

func RenderSequence(ctx context.Context, e *Evaluator, decoder SequentialDecoder, at int64, present func(context.Context, CompositeFrame) error) error {
	if e == nil || decoder == nil || present == nil || at < 0 || at >= e.Duration() {
		return errors.New("continuous preview requires a position before timeline end")
	}
	identities := make(map[string]mediacache.Source)
	start := at
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		frame, err := renderCompositeFrame(ctx, e, decoder, at, identities)
		if err != nil {
			return err
		}
		if err := present(ctx, frame); err != nil {
			return err
		}
		if at == e.Duration() {
			return nil
		}
		wait := time.Until(started.Add(time.Second / media.CompositeFPS))
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		at = e.NextVideoSample(start, at)
	}
}

func renderCompositeFrame(ctx context.Context, e *Evaluator, decoder SequentialDecoder, at int64, identities map[string]mediacache.Source) (CompositeFrame, error) {
	plan, err := e.PlanLayers(at)
	if err != nil {
		return CompositeFrame{}, err
	}
	layers := plan.DecodeLayers()
	ids := make([]string, 0, len(layers))
	sources := make(map[string]mediacache.Source)
	for _, layer := range layers {
		ids = append(ids, layer.Layer.Ref.ClipID)
		path := layer.Layer.Asset.Path
		if _, ok := sources[path]; ok {
			continue
		}
		s, err := mediacache.Identify(path)
		if err != nil {
			return CompositeFrame{}, err
		}
		if prior, ok := identities[path]; ok && prior != s {
			return CompositeFrame{}, errors.New("source changed during continuous preview")
		}
		identities[path] = s
		sources[path] = s
	}
	if err := decoder.Retain(ids); err != nil {
		return CompositeFrame{}, err
	}
	job, cancel := context.WithTimeout(ctx, 90*time.Second)
	frame, err := RenderStill(job, decoder, plan, CompositePreviewSide)
	cancel()
	if err != nil {
		return CompositeFrame{}, err
	}
	for path, identity := range sources {
		now, err := mediacache.Identify(path)
		if err != nil || now != identity {
			return CompositeFrame{}, errors.New("source changed during composite frame")
		}
	}
	return CompositeFrame{Time: at, ValidUntil: e.NextVideoBoundary(at), Frame: frame, Sources: sources, End: at == e.Duration()}, nil
}

func RenderClockedSequence(ctx context.Context, e *Evaluator, decoder SequentialDecoder, start int64, clock func() int64, present func(context.Context, CompositeFrame) error) error {
	return RenderClockedSequenceObserved(ctx, e, decoder, start, clock, present, nil)
}

type FrameObservation struct {
	DecodeTime time.Duration
	Eligible   bool
}

func RenderClockedSequenceObserved(ctx context.Context, e *Evaluator, decoder SequentialDecoder, start int64, clock func() int64, present func(context.Context, CompositeFrame) error, observe func(FrameObservation)) error {
	if e == nil || decoder == nil || clock == nil || present == nil || start < 0 || start >= e.Duration() {
		return errors.New("invalid clocked preview request")
	}
	identities := make(map[string]mediacache.Source)
	at := start
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := min(e.Duration(), max(start, clock()))
		if now == e.Duration() {
			at = now
		} else {
			at = max(at, e.VideoSampleAt(start, now))
		}
		started := time.Now()
		frame, err := renderCompositeFrame(ctx, e, decoder, at, identities)
		elapsed := time.Since(started)
		if err != nil {
			return err
		}
		for clock() < at {
			timer := time.NewTimer(5 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		eligible := frame.CurrentAt(clock())
		if observe != nil {
			observe(FrameObservation{DecodeTime: elapsed, Eligible: eligible})
		}
		if eligible {
			if err := present(ctx, frame); err != nil {
				return err
			}
		}
		if at == e.Duration() {
			return nil
		}
		at = e.NextVideoSample(start, at)
	}
}
