package media

import (
	"fmt"
	"math"
)

type AudioLevels struct {
	Frames  int64
	Peak    [PCMChannels]uint32
	Clipped [PCMChannels]int64
}

func (l *AudioLevels) Merge(v AudioLevels) {
	l.Frames += v.Frames
	for ch := 0; ch < PCMChannels; ch++ {
		l.Peak[ch] = max(l.Peak[ch], v.Peak[ch])
		l.Clipped[ch] += v.Clipped[ch]
	}
}

func (l AudioLevels) ClipCount() int64 {
	return l.Clipped[0] + l.Clipped[1]
}

func PeakDBFS(peak uint32) float64 {
	if peak == 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(float64(peak)/32768)
}

func PeakLabel(peak uint32) string {
	if peak == 0 {
		return "-inf"
	}
	return fmt.Sprintf("%.1f", PeakDBFS(peak))
}

func (l AudioLevels) String() string {
	if l.Frames == 0 {
		return "No measured PCM frames."
	}
	return fmt.Sprintf("Mixed PCM sample peaks (dBFS): L %s / R %s; clipped channel samples: L %d / R %d; measured frames: %d", PeakLabel(l.Peak[0]), PeakLabel(l.Peak[1]), l.Clipped[0], l.Clipped[1], l.Frames)
}
