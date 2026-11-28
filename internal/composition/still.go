package composition

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/markrai/pixelcut/internal/media"
)

type StillDecoder interface {
	Decode(context.Context, media.StillRequest) (media.StillFrame, error)
}

func StillSize(canvas media.CanvasSize, limit int) (int, int, error) {
	if err := canvas.Validate(); err != nil {
		return 0, 0, err
	}
	if limit < 2 || limit > media.MaxStillSide {
		return 0, 0, errors.New("invalid static preview resolution")
	}
	scale := math.Min(1, float64(limit)/float64(max(canvas.Width, canvas.Height)))
	return max(1, int(math.Round(float64(canvas.Width)*scale))), max(1, int(math.Round(float64(canvas.Height)*scale))), nil
}

func RenderStill(ctx context.Context, decoder StillDecoder, plan LayerPlan, limit int) (media.StillFrame, error) {
	transitionIndex, sample, err := plan.ActiveTransition()
	if err != nil {
		return media.StillFrame{}, err
	}
	w, h, err := StillSize(plan.Canvas, limit)
	if err != nil {
		return media.StillFrame{}, err
	}
	if decoder == nil {
		return media.StillFrame{}, errors.New("still decoder unavailable")
	}
	out := media.StillFrame{Width: w, Height: h, Pixels: make([]byte, w*h*4)}
	for i := 3; i < len(out.Pixels); i += 4 {
		out.Pixels[i] = 255
	}
	x := func(v int) int { return int(math.Round(float64(v) * float64(w) / float64(plan.Canvas.Width))) }
	y := func(v int) int { return int(math.Round(float64(v) * float64(h) / float64(plan.Canvas.Height))) }
	for i, item := range plan.Layers {
		if err = ctx.Err(); err != nil {
			return media.StillFrame{}, err
		}
		layer := item.Layer
		if layer.Opacity < 0 || layer.Opacity > 100 {
			return media.StillFrame{}, errors.New("invalid layer opacity")
		}
		if layer.Opacity == 0 {
			continue
		}
		b := item.Geometry.Bounds
		if b.DestX < 0 || b.DestY < 0 || b.DestX > plan.Canvas.Width || b.DestY > plan.Canvas.Height || b.Width < 0 || b.Height < 0 || b.Width > plan.Canvas.Width-b.DestX || b.Height > plan.Canvas.Height-b.DestY {
			return media.StillFrame{}, errors.New("layer bounds outside composition canvas")
		}
		left, top, right, bottom := x(b.DestX), y(b.DestY), x(b.DestX+b.Width), y(b.DestY+b.Height)
		if right <= left || bottom <= top {
			continue
		}
		frame, err := decodePreviewLayer(ctx, decoder, plan.Canvas, item, right-left, bottom-top)
		if err != nil {
			return media.StillFrame{}, err
		}
		if i == transitionIndex {
			from, err := decodePreviewLayer(ctx, decoder, plan.Canvas, plan.Transitions[0].From, w, h)
			if err != nil {
				return media.StillFrame{}, err
			}
			if err := blendSDRTransition(ctx, out.Pixels, from.Pixels, frame.Pixels, w, h, sample.IncomingWeight); err != nil {
				return media.StillFrame{}, err
			}
			continue
		}
		alpha := layer.Opacity
		for row := 0; row < frame.Height; row++ {
			if err = ctx.Err(); err != nil {
				return media.StillFrame{}, err
			}
			dst := out.Pixels[((top+row)*w+left)*4:]
			src := frame.Pixels[row*frame.Width*4:]
			for col := 0; col < frame.Width*4; col += 4 {
				for c := 0; c < 3; c++ {
					dst[col+c] = byte((int(src[col+c])*alpha + int(dst[col+c])*(100-alpha) + 50) / 100)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return media.StillFrame{}, err
	}
	return out, nil
}

func decodePreviewLayer(ctx context.Context, decoder StillDecoder, canvas media.CanvasSize, item PlannedLayer, width, height int) (media.StillFrame, error) {
	layer := item.Layer
	var crop *media.Crop
	if c := layer.Crop; c != nil {
		crop = &media.Crop{X: c.X, Y: c.Y, Width: c.Width, Height: c.Height}
	}
	frame, err := decoder.Decode(ctx, media.StillRequest{Path: layer.Asset.Path, ClipID: layer.Ref.ClipID, Tick: layer.SourceTime, SourceEnd: layer.Asset.Duration, Width: layer.Asset.Width, Height: layer.Asset.Height, Range: layer.Asset.Range, Color: layer.Asset.PreviewColor(), Canvas: canvas, Crop: crop, Transform: layer.Transform, OutputWidth: width, OutputHeight: height})
	if err != nil {
		return media.StillFrame{}, fmt.Errorf("layer %s/%s: %w", layer.Ref.TrackID, layer.Ref.ClipID, err)
	}
	if !frame.Valid() || frame.Width != width || frame.Height != height {
		return media.StillFrame{}, errors.New("decoder returned invalid layer dimensions")
	}
	return frame, nil
}
