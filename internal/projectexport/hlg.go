package projectexport

import (
	"context"
	"encoding/binary"
	"errors"
	"math"

	"github.com/markrai/pixelcut/internal/media"
)

const (
	hlgA = 0.17883277
	hlgB = 1 - 4*hlgA
	hlgC = 0.55991073
	kr   = 0.2627
	kb   = 0.0593
	kg   = 1 - kr - kb
)

type linearRGB struct {
	R, G, B float64
}

type ycbcr10 struct {
	Y, Cb, Cr uint16
}

func clampUnit(x float64) float64 { return max(0, min(1, x)) }

func hlgToLinear(e float64) float64 {
	e = clampUnit(e)
	if e <= 0.5 {
		return e * e / 3
	}
	return (math.Exp((e-hlgC)/hlgA) + hlgB) / 12
}

func linearToHLG(e float64) float64 {
	e = clampUnit(e)
	if e <= 1.0/12 {
		return math.Sqrt(3 * e)
	}
	return hlgA*math.Log(12*e-hlgB) + hlgC
}

func decodeHLG(p ycbcr10) linearRGB {
	y := (float64(p.Y) - 64) / 876
	cb := (float64(p.Cb) - 512) / 896
	cr := (float64(p.Cr) - 512) / 896
	r := y + 2*(1-kr)*cr
	b := y + 2*(1-kb)*cb
	g := (y - kr*r - kb*b) / kg
	return linearRGB{hlgToLinear(r), hlgToLinear(g), hlgToLinear(b)}
}

func encodeHLG(p linearRGB) ycbcr10 {
	r, g, b := linearToHLG(p.R), linearToHLG(p.G), linearToHLG(p.B)
	y := kr*r + kg*g + kb*b
	cb := (b - y) / (2 * (1 - kb))
	cr := (r - y) / (2 * (1 - kr))
	return ycbcr10{
		Y:  uint16(math.Round(64 + 876*clampUnit(y))),
		Cb: uint16(math.Round(max(64, min(960, 512+896*cb)))),
		Cr: uint16(math.Round(max(64, min(960, 512+896*cr)))),
	}
}

type layerFrame struct {
	Pixels  []byte
	Bounds  media.Placement
	Opacity int
}

func readPixel(p []byte, planeBytes, offset int) ycbcr10 {
	return ycbcr10{binary.LittleEndian.Uint16(p[offset:]), binary.LittleEndian.Uint16(p[planeBytes+offset:]), binary.LittleEndian.Uint16(p[2*planeBytes+offset:])}
}

func writePixel(p []byte, planeBytes, offset int, v ycbcr10) {
	binary.LittleEndian.PutUint16(p[offset:], v.Y)
	binary.LittleEndian.PutUint16(p[planeBytes+offset:], v.Cb)
	binary.LittleEndian.PutUint16(p[2*planeBytes+offset:], v.Cr)
}

func compositeHLG(ctx context.Context, dst []byte, canvas media.CanvasSize, layers []layerFrame) error {
	if canvas.Validate() != nil || len(dst) != canvas.Width*canvas.Height*6 || len(layers) > MaxVideoLayers {
		return errors.New("invalid HDR composition buffer")
	}
	translucent := false
	for _, layer := range layers {
		b := layer.Bounds
		if layer.Opacity < 0 || layer.Opacity > 100 || b.Width <= 0 || b.Height <= 0 || b.DestX < 0 || b.DestY < 0 || b.Width > canvas.Width-b.DestX || b.Height > canvas.Height-b.DestY || len(layer.Pixels) != b.Width*b.Height*6 {
			return errors.New("invalid HDR layer buffer or placement")
		}
		translucent = translucent || (layer.Opacity > 0 && layer.Opacity < 100)
	}
	if !translucent {
		for _, layer := range layers {
			if err := ctx.Err(); err != nil {
				return err
			}
			if layer.Opacity == 100 {
				if err := copyOpaque(dst, layer.Pixels, canvas, layer.Bounds); err != nil {
					return err
				}
			}
		}
		return ctx.Err()
	}
	planeBytes := canvas.Width * canvas.Height * 2
	for y := 0; y < canvas.Height; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < canvas.Width; x++ {
			offset := (y*canvas.Width + x) * 2
			remaining := 1.0
			var sum linearRGB
			mixed := false
			for i := len(layers) - 1; i >= 0; i-- {
				layer := layers[i]
				b := layer.Bounds
				if layer.Opacity == 0 || x < b.DestX || x >= b.DestX+b.Width || y < b.DestY || y >= b.DestY+b.Height {
					continue
				}
				srcOffset := ((y-b.DestY)*b.Width + x - b.DestX) * 2
				p := readPixel(layer.Pixels, b.Width*b.Height*2, srcOffset)
				if !mixed && layer.Opacity == 100 {
					writePixel(dst, planeBytes, offset, p)
					break
				}
				mixed = true
				alpha := float64(layer.Opacity) / 100
				weight := remaining * alpha
				v := decodeHLG(p)
				sum.R += weight * v.R
				sum.G += weight * v.G
				sum.B += weight * v.B
				remaining *= 1 - alpha
				if layer.Opacity == 100 {
					break
				}
			}
			if mixed {
				writePixel(dst, planeBytes, offset, encodeHLG(sum))
			}
		}
	}
	return ctx.Err()
}
