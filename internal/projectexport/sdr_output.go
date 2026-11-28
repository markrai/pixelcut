package projectexport

import "math"

const SDRToneMappingDescription = "HLG to SDR uses a fixed scene-referred luminance shoulder, mapping 75% HLG signal to 75% SDR linear white, with BT.709 gamut compression"

const sdrKr = 0.2126
const sdrKb = 0.0722
const sdrKg = 1 - sdrKr - sdrKb

func linearToSDR(v float64) float64 {
	v = clampUnit(v)
	if v < 0.018 {
		return 4.5 * v
	}
	return 1.099*math.Pow(v, 0.45) - 0.099
}

func encodeSDR709(p linearRGB) ycbcr10 {
	r, g, b := linearToSDR(p.R), linearToSDR(p.G), linearToSDR(p.B)
	y := sdrKr*r + sdrKg*g + sdrKb*b
	cb := (b - y) / (2 * (1 - sdrKb))
	cr := (r - y) / (2 * (1 - sdrKr))
	return ycbcr10{
		Y:  uint16(math.Round(64 + 876*clampUnit(y))),
		Cb: uint16(math.Round(max(64, min(960, 512+896*cb)))),
		Cr: uint16(math.Round(max(64, min(960, 512+896*cr)))),
	}
}

func sdrToneLuminance(v float64) float64 {
	v = clampUnit(v)
	const knee = 0.75
	slope := knee / sdrReferenceWhite
	if v <= sdrReferenceWhite {
		return v * slope
	}
	t := (v - sdrReferenceWhite) / (1 - sdrReferenceWhite)
	a := (1 - knee) / (slope*(1-sdrReferenceWhite) - (1 - knee))
	return knee + (1-knee)*(1+a)*t/(t+a)
}

func mapHLGToSDR(p ycbcr10) ycbcr10 {
	v := decodeHLG(p)
	y := kr*v.R + kg*v.G + kb*v.B
	if y <= 0 {
		return ycbcr10{64, 512, 512}
	}
	target := sdrToneLuminance(y)
	scale := target / y
	r := scale * (1.660491*v.R - 0.587641*v.G - 0.072850*v.B)
	g := scale * (-0.124550*v.R + 1.132900*v.G - 0.008350*v.B)
	b := scale * (-0.018151*v.R - 0.100579*v.G + 1.118730*v.B)
	neutral := clampUnit(sdrKr*r + sdrKg*g + sdrKb*b)
	saturation := 1.0
	for _, c := range [3]float64{r, g, b} {
		if c < 0 {
			saturation = min(saturation, neutral/(neutral-c))
		} else if c > 1 {
			saturation = min(saturation, (1-neutral)/(c-neutral))
		}
	}
	return encodeSDR709(linearRGB{
		R: neutral + saturation*(r-neutral),
		G: neutral + saturation*(g-neutral),
		B: neutral + saturation*(b-neutral),
	})
}
