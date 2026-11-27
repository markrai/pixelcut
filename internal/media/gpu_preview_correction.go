package media

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const previewChromaWeightScale = 16384

type PreviewChromaCorrection struct {
	Method             string  `json:"method"`
	CodedSourceWidth   int     `json:"coded_source_width"`
	CodedOutputWidth   int     `json:"coded_output_width"`
	ShiftChromaSamples float64 `json:"shift_chroma_samples"`
	Weights            [7]int  `json:"weights"`
	WeightScale        int     `json:"weight_scale"`
}

func previewSinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

func planPreviewChromaCorrection(source, output int) (PreviewChromaCorrection, error) {
	p := PreviewChromaCorrection{Method: "reduced_chroma_lanczos3_phase", CodedSourceWidth: source, CodedOutputWidth: output, WeightScale: previewChromaWeightScale}
	if source < 2 || source > 8192 || output < 2 || output > MaxStillSide || output > source || source%2 != 0 || output%2 != 0 {
		return p, errors.New("invalid coded chroma correction width")
	}
	p.ShiftChromaSamples = -(1 - float64(output)/float64(source)) / 4
	if source == output {
		p.Weights[3] = p.WeightScale
		return p, nil
	}
	if output < 16 {
		return p, errors.New("chroma phase correction requires at least eight horizontal chroma samples")
	}
	var taps [7]float64
	var sum float64
	for i := range taps {
		x := float64(i-3) - p.ShiftChromaSamples
		if math.Abs(x) < 3 {
			taps[i] = previewSinc(x) * previewSinc(x/3)
			sum += taps[i]
		}
	}
	total := 0
	for i, tap := range taps {
		p.Weights[i] = int(math.Round(tap / sum * float64(p.WeightScale)))
		total += p.Weights[i]
	}
	p.Weights[3] += p.WeightScale - total
	return p, nil
}

func (p PreviewChromaCorrection) filter() string {
	taps := make([]string, len(p.Weights))
	for i, w := range p.Weights {
		taps[i] = strconv.Itoa(w)
	}
	matrix := strings.Join(taps, " ")
	div := strconv.FormatFloat(1/float64(p.WeightScale), 'f', -1, 64)
	return fmt.Sprintf("convolution=0m='0 1 0':0mode=row:1m='%s':1mode=row:2m='%s':2mode=row:1rdiv=%s:2rdiv=%s", matrix, matrix, div, div)
}
