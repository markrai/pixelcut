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
	Time    int64
	Frame   media.StillFrame
	Sources map[string]mediacache.Source
	End     bool
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
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		plan, err := e.PlanLayers(at)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(plan.Layers))
		sources := make(map[string]mediacache.Source)
		for _, layer := range plan.Layers {
			ids = append(ids, layer.Layer.Ref.ClipID)
			path := layer.Layer.Asset.Path
			if _, ok := sources[path]; ok {
				continue
			}
			s, err := mediacache.Identify(path)
			if err != nil {
				return err
			}
			if prior, ok := identities[path]; ok && prior != s {
				return errors.New("source changed during continuous preview")
			}
			identities[path] = s
			sources[path] = s
		}
		if err := decoder.Retain(ids); err != nil {
			return err
		}
		job, cancel := context.WithTimeout(ctx, 90*time.Second)
		frame, err := RenderStill(job, decoder, plan, CompositePreviewSide)
		cancel()
		if err != nil {
			return err
		}
		for path, identity := range sources {
			now, err := mediacache.Identify(path)
			if err != nil || now != identity {
				return errors.New("source changed during composite frame")
			}
		}
		if err := present(ctx, CompositeFrame{Time: at, Frame: frame, Sources: sources, End: at == e.Duration()}); err != nil {
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
		at = NextCompositeTick(at, e.Duration())
	}
}
