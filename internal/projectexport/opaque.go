package projectexport

import (
	"errors"
	"fmt"
	"strings"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
)

const MaxVideoLayers = 4
const MaxExportPixels = 4096 * 2160
const MaxIntermediatePixels = 16777216

func layerFilter(layer composition.PlannedLayer, source Source, first, last int) (string, error) {
	g := layer.Geometry
	p, b := g.Content, g.Bounds
	if first < 0 || last < first || b.Width <= 0 || b.Height <= 0 || int64(layer.Layer.Asset.Width)*int64(layer.Layer.Asset.Height) > MaxIntermediatePixels || int64(p.ScaledWidth)*int64(p.ScaledHeight) > MaxIntermediatePixels {
		return "", errors.New("unsupported export layer geometry or source interval")
	}
	rangeName := "limited"
	if source.Asset.Range == "pc" {
		rangeName = "full"
	} else if source.Asset.Range != "tv" {
		return "", errors.New("unsupported source range")
	}
	if source.Chroma != "left" && source.Chroma != "center" {
		return "", errors.New("unsupported source chroma siting")
	}
	if source.Asset.Classified() && source.Asset.ColorFamily != media.ColorSDR709 {
		return "", errors.New("unsupported classified export source")
	}
	matrix, primaries, transfer := "2020_ncl", "2020", "arib-std-b67"
	if source.Asset.ColorFamily == media.ColorSDR709 {
		matrix, primaries, transfer = "709", "709", "709"
	}
	normalize := fmt.Sprintf("zscale=rangein=%s:range=limited:chromalin=%s:matrixin=%s:matrix=%s:primariesin=%s:primaries=%s:transferin=%s:transfer=%s:filter=lanczos", rangeName, source.Chroma, matrix, matrix, primaries, primaries, transfer, transfer)
	filters := []string{fmt.Sprintf("select=between(n\\,%d\\,%d)", first, last), normalize, "format=yuv444p10le"}
	switch source.Rotation {
	case 0:
	case -90:
		filters = append(filters, "transpose=clock")
	case 90:
		filters = append(filters, "transpose=cclock")
	case 180:
		filters = append(filters, "hflip", "vflip")
	default:
		return "", errors.New("unsupported source rotation")
	}
	if c := layer.Layer.Crop; c != nil {
		filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d", c.Width, c.Height, c.X, c.Y))
	}
	if t := layer.Layer.Transform; t != nil {
		switch t.Rotation {
		case 90:
			filters = append(filters, "transpose=clock")
		case 180:
			filters = append(filters, "hflip", "vflip")
		case 270:
			filters = append(filters, "transpose=cclock")
		}
		if t.FlipHorizontal {
			filters = append(filters, "hflip")
		}
		if t.FlipVertical {
			filters = append(filters, "vflip")
		}
	}
	scale := func(w, h int) string {
		return fmt.Sprintf("scale=%d:%d:flags=lanczos:in_range=tv:out_range=tv", w, h)
	}
	filters = append(filters, scale(p.ScaledWidth, p.ScaledHeight))
	if p.SourceX != 0 || p.SourceY != 0 || p.Width != p.ScaledWidth || p.Height != p.ScaledHeight {
		filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d", p.Width, p.Height, p.SourceX, p.SourceY))
	}
	if p.Width != b.Width || p.Height != b.Height {
		filters = append(filters, scale(b.Width, b.Height))
	}
	return strings.Join(append(filters, "setsar=1"), ","), nil
}

func black444(canvas media.CanvasSize) []byte {
	n := canvas.Width * canvas.Height
	out := make([]byte, n*6)
	for i := 0; i < n*2; i += 2 {
		out[i] = 64
	}
	for i := n*2 + 1; i < len(out); i += 2 {
		out[i] = 2
	}
	return out
}

func copyOpaque(dst, src []byte, canvas media.CanvasSize, b media.Placement) error {
	if canvas.Validate() != nil || b.Width <= 0 || b.Height <= 0 || b.DestX < 0 || b.DestY < 0 || b.Width > canvas.Width-b.DestX || b.Height > canvas.Height-b.DestY || len(dst) != canvas.Width*canvas.Height*6 || len(src) != b.Width*b.Height*6 {
		return errors.New("invalid opaque layer buffer or placement")
	}
	for plane := 0; plane < 3; plane++ {
		for y := 0; y < b.Height; y++ {
			d := (plane*canvas.Width*canvas.Height + (b.DestY+y)*canvas.Width + b.DestX) * 2
			s := (plane*b.Width*b.Height + y*b.Width) * 2
			copy(dst[d:d+b.Width*2], src[s:s+b.Width*2])
		}
	}
	return nil
}

func validateVideo(plan Plan, discardAudio bool) error {
	c := plan.Contract.Canvas
	if c.Width > 4096 || c.Height > 4096 || int64(c.Width)*int64(c.Height) > MaxExportPixels {
		return errors.New("video export currently supports at most 4096 pixels per side and 4096x2160 pixels total")
	}
	if plan.Contract.Audio && !discardAudio {
		return errors.New("project contains enabled audio; use export-project or pass -discard-audio to explicitly export video only")
	}
	sampled := false
	for _, s := range plan.Spans {
		if s.FirstFrame == s.EndFrame {
			continue
		}
		if err := validateTransitionSpan(s, c); err != nil {
			return err
		}
		if len(s.Layers) > MaxVideoLayers {
			return errors.New("video export supports at most four simultaneous video layers")
		}
		for _, layer := range spanDecodeLayers(s) {
			if layer.Layer.Opacity < 0 || layer.Layer.Opacity > 100 {
				return errors.New("invalid layer opacity")
			}
			if layer.Geometry.Bounds.Width == 0 || layer.Geometry.Bounds.Height == 0 {
				continue
			}
			sampled = sampled || layer.Layer.Opacity > 0
			asset := layer.Layer.Asset
			if asset.Range == "" {
				asset.Range = "tv"
			}
			chroma := "left"
			if asset.Classified() {
				chroma = asset.Chroma
			}
			if _, err := layerFilter(layer, Source{Asset: asset, Chroma: chroma}, 0, 0); err != nil {
				return err
			}
		}
	}
	if !sampled {
		return errors.New("no visible video pixels sampled on the selected CFR grid")
	}
	return nil
}
