package composition

import (
	"context"
	"errors"
	"math"

	"github.com/markrai/pixelcut/internal/media"
)

const previewLinearMax = 65535

var previewToLinear, previewFromLinear = makePreviewTransferTables()

func makePreviewTransferTables() ([256]int64, [previewLinearMax + 1]byte) {
	var inverse [256]int64
	var forward [previewLinearMax + 1]byte
	for i := range inverse {
		v := float64(i) / 255
		if v < 0.081 {
			v /= 4.5
		} else {
			v = math.Pow((v+0.099)/1.099, 1/0.45)
		}
		inverse[i] = int64(math.Round(v * previewLinearMax))
	}
	for i := range forward {
		v := float64(i) / previewLinearMax
		if v < 0.018 {
			v *= 4.5
		} else {
			v = 1.099*math.Pow(v, 0.45) - 0.099
		}
		forward[i] = byte(math.Round(max(0, min(1, v)) * 255))
	}
	return inverse, forward
}

func blendSDRTransition(ctx context.Context, dst, from, to []byte, width, height int, weight int64) error {
	if width < 1 || height < 1 || width > media.MaxStillSide || height > media.MaxStillSide || len(dst) != width*height*4 || len(from) != len(dst) || len(to) != len(dst) || weight < 0 || weight > TransitionWeightScale {
		return errors.New("invalid SDR transition buffer or weight")
	}
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < width; x++ {
			i := (y*width + x) * 4
			for c := 0; c < 3; c++ {
				switch weight {
				case 0:
					dst[i+c] = from[i+c]
				case TransitionWeightScale:
					dst[i+c] = to[i+c]
				default:
					v := (previewToLinear[from[i+c]]*(TransitionWeightScale-weight) + previewToLinear[to[i+c]]*weight + TransitionWeightScale/2) / TransitionWeightScale
					dst[i+c] = previewFromLinear[v]
				}
			}
			dst[i+3] = 255
		}
	}
	return ctx.Err()
}
