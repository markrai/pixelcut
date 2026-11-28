package projectexport

import (
	"context"
	"errors"
	"math"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

const SDRMappingDescription = "SDR BT.709 uses inverse BT.709 transfer and BT.2020 gamut conversion; SDR reference white maps to 75% HLG signal (whole-project HLG export)"

var sdrReferenceWhite = hlgToLinear(0.75)

func exportDecodeEngine(engine media.Engine, asset project.Asset) media.Engine {
	if asset.ColorFamily == media.ColorSDR709 && engine.DecodeBackend == media.BackendCUDA {
		engine.DecodeBackend = media.BackendCPU
	}
	return engine
}

func sdrToLinear(v float64) float64 {
	v = clampUnit(v)
	if v < 0.081 {
		return v / 4.5
	}
	return math.Pow((v+0.099)/1.099, 1/0.45)
}

func decodeSDR709(p ycbcr10) linearRGB {
	const r709, b709 = 0.2126, 0.0722
	const g709 = 1 - r709 - b709
	y := (float64(p.Y) - 64) / 876
	cb := (float64(p.Cb) - 512) / 896
	cr := (float64(p.Cr) - 512) / 896
	r := y + 2*(1-r709)*cr
	b := y + 2*(1-b709)*cb
	g := (y - r709*r - b709*b) / g709
	return linearRGB{sdrToLinear(r), sdrToLinear(g), sdrToLinear(b)}
}

func mapSDR709(p ycbcr10) ycbcr10 {
	v := decodeSDR709(p)
	r, g, b := v.R, v.G, v.B
	return encodeHLG(linearRGB{
		R: sdrReferenceWhite * (0.6274040*r + 0.3292820*g + 0.0433136*b),
		G: sdrReferenceWhite * (0.0690970*r + 0.9195400*g + 0.0113612*b),
		B: sdrReferenceWhite * (0.0163916*r + 0.0880132*g + 0.8955950*b),
	})
}

func convertSDRFrame(ctx context.Context, pixels []byte, width, height int) error {
	return convertColorFrame(ctx, pixels, width, height, mapSDR709)
}

func convertColorFrame(ctx context.Context, pixels []byte, width, height int, convert func(ycbcr10) ycbcr10) error {
	if width <= 0 || height <= 0 || width > MaxIntermediatePixels/height || len(pixels) != width*height*6 {
		return errors.New("invalid color conversion frame")
	}
	planeBytes := width * height * 2
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 2
			writePixel(pixels, planeBytes, offset, convert(readPixel(pixels, planeBytes, offset)))
		}
	}
	return ctx.Err()
}
