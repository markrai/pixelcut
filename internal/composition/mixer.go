package composition

import (
	"errors"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

const fadeScale int64 = 1000000
const mixScale = 100 * fadeScale

type PCMMixer struct {
	sum    []int64
	inputs int
}

func (m *PCMMixer) Reset(frames int) error {
	if frames < 1 || frames > media.PCMBlockFrames {
		return errors.New("invalid mix frame count")
	}
	n := frames * media.PCMChannels
	if cap(m.sum) < n {
		m.sum = make([]int64, n)
	} else {
		m.sum = m.sum[:n]
		clear(m.sum)
	}
	m.inputs = 0
	return nil
}

func (m *PCMMixer) Add(samples []int16, gainPercent int) error {
	if len(m.sum) == 0 || len(samples) != len(m.sum) || gainPercent < 0 || gainPercent > project.MaxGainPercent || m.inputs >= media.MaxAudioStreams {
		return errors.New("invalid PCM contribution")
	}
	m.inputs++
	for i, sample := range samples {
		m.sum[i] += int64(sample) * int64(gainPercent) * fadeScale
	}
	return nil
}

func fadeFactor(interval project.Interval, fadeIn, fadeOut, at int64) int64 {
	if !interval.Contains(at) {
		return 0
	}
	duration := interval.End - interval.Start
	in, out := min(fadeIn, duration), min(fadeOut, duration)
	a, b := fadeScale, fadeScale
	if in > 0 && at-interval.Start < in {
		a = (at - interval.Start) * fadeScale / in
	}
	if out > 0 && interval.End-at < out {
		b = (interval.End - at) * fadeScale / out
	}
	return (a*b + fadeScale/2) / fadeScale
}

func (m *PCMMixer) AddContribution(samples []int16, a AudioContribution, start, offset int64) error {
	if a.FadeIn == 0 && a.FadeOut == 0 {
		return m.Add(samples, a.GainPercent)
	}
	if len(m.sum) == 0 || len(samples) != len(m.sum) || a.GainPercent < 0 || a.GainPercent > project.MaxGainPercent || m.inputs >= media.MaxAudioStreams || !a.Timeline.Valid() || !project.ValidAudioFades(a.FadeIn, a.FadeOut) || start < 0 || offset < 0 {
		return errors.New("invalid PCM fade contribution")
	}
	frames := len(samples) / media.PCMChannels
	if offset > media.PCMFramesForTicks(a.Timeline.End) || start > a.Timeline.End || media.PCMTicksForFrames(offset) > a.Timeline.End-start {
		return errors.New("invalid PCM fade sample position")
	}
	m.inputs++
	for i := 0; i < frames; i++ {
		tick := media.PCMTicksForFrames(offset + int64(i))
		factor := int64(0)
		if tick < a.Timeline.End-start {
			factor = fadeFactor(a.Timeline, a.FadeIn, a.FadeOut, start+tick)
		}
		gain := int64(a.GainPercent) * factor
		for ch := 0; ch < media.PCMChannels; ch++ {
			j := i*media.PCMChannels + ch
			m.sum[j] += int64(samples[j]) * gain
		}
	}
	return nil
}

func (m *PCMMixer) Finish(dst []int16) error {
	if len(m.sum) == 0 || len(dst) != len(m.sum) {
		return errors.New("invalid PCM destination")
	}
	for i, value := range m.sum {
		if value < 0 {
			value = (value - mixScale/2) / mixScale
		} else {
			value = (value + mixScale/2) / mixScale
		}
		dst[i] = int16(max(-32768, min(32767, value)))
	}
	return nil
}
