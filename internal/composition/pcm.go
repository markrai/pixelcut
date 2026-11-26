package composition

import (
	"context"
	"errors"
	"fmt"

	"github.com/markrai/pixelcut/internal/media"
)

type PCMDecoder interface {
	Retain([]string) error
	DecodePCM(context.Context, media.PCMRequest) ([]int16, error)
}

type PCMBlock struct {
	Offset  int64
	Frames  int
	Samples []int16
	Levels  media.AudioLevels
}

func MixPCM(ctx context.Context, e *Evaluator, decoder PCMDecoder, start, offset int64, frames int) (PCMBlock, error) {
	if e == nil || decoder == nil || start < 0 || start >= e.Duration() || offset < 0 || frames < 1 || frames > media.PCMBlockFrames {
		return PCMBlock{}, errors.New("invalid composite audio request")
	}
	total := media.PCMFramesForTicks(e.Duration() - start)
	if offset >= total || int64(frames) > total-offset {
		return PCMBlock{}, errors.New("audio request outside timeline")
	}
	out := PCMBlock{Offset: offset, Frames: frames, Samples: make([]int16, frames*media.PCMChannels)}
	var mixer PCMMixer
	for pos := offset; pos < offset+int64(frames); {
		if err := ctx.Err(); err != nil {
			return PCMBlock{}, err
		}
		at := start + media.PCMTicksForFrames(pos)
		f, err := e.At(at)
		if err != nil {
			return PCMBlock{}, err
		}
		next := min(offset+int64(frames), media.PCMFramesForTicks(f.NextBoundary-start))
		if next <= pos {
			return PCMBlock{}, errors.New("audio boundary did not advance")
		}
		ids := make([]string, 0, len(f.Audio))
		for _, a := range f.Audio {
			ids = append(ids, a.Ref.ClipID)
		}
		if err := decoder.Retain(ids); err != nil {
			return PCMBlock{}, err
		}
		n := int(next - pos)
		if err := mixer.Reset(n); err != nil {
			return PCMBlock{}, err
		}
		for _, a := range f.Audio {
			first, err := media.PCMSourceFrame(a.SourceInterval.Start, a.Timeline.Start, start, pos)
			if err != nil {
				return PCMBlock{}, err
			}
			end := media.PCMFramesForTicks(a.Timeline.End - start)
			last, err := media.PCMSourceFrame(a.SourceInterval.Start, a.Timeline.Start, start, end)
			if err != nil {
				return PCMBlock{}, err
			}
			samples, err := decoder.DecodePCM(ctx, media.PCMRequest{ClipID: a.Ref.ClipID, Path: a.Asset.Path, Tick: a.SourceTime, Offset: pos, Frames: n, SourceFrame: first, SourceEnd: last})
			if err != nil {
				return PCMBlock{}, fmt.Errorf("audio %s/%s: %w", a.Ref.TrackID, a.Ref.ClipID, err)
			}
			if err := mixer.AddContribution(samples, a, start, pos); err != nil {
				return PCMBlock{}, err
			}
		}
		first := int(pos-offset) * media.PCMChannels
		if err := mixer.FinishObserved(out.Samples[first:first+n*media.PCMChannels], &out.Levels); err != nil {
			return PCMBlock{}, err
		}
		pos = next
	}
	if err := ctx.Err(); err != nil {
		return PCMBlock{}, err
	}
	return out, nil
}
