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
		n := int(next-pos)
		sum := make([]int32, n*media.PCMChannels)
		for _, a := range f.Audio {
			samples, err := decoder.DecodePCM(ctx, media.PCMRequest{ClipID: a.Ref.ClipID, Path: a.Asset.Path, Tick: a.SourceTime, Offset: pos, Frames: n})
			if err != nil {
				return PCMBlock{}, fmt.Errorf("audio %s/%s: %w", a.Ref.TrackID, a.Ref.ClipID, err)
			}
			if len(samples) != len(sum) {
				return PCMBlock{}, errors.New("invalid decoded PCM block")
			}
			for i, value := range samples {
				sum[i] += int32(value)
			}
		}
		for i, value := range sum {
			out.Samples[int(pos-offset)*media.PCMChannels+i] = int16(max(-32768, min(32767, value)))
		}
		pos = next
	}
	if err := ctx.Err(); err != nil {
		return PCMBlock{}, err
	}
	return out, nil
}
