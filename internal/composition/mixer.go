package composition

import (
	"errors"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

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
		m.sum[i] += int64(sample) * int64(gainPercent)
	}
	return nil
}

func (m *PCMMixer) Finish(dst []int16) error {
	if len(m.sum) == 0 || len(dst) != len(m.sum) {
		return errors.New("invalid PCM destination")
	}
	for i, value := range m.sum {
		if value < 0 {
			value = (value - 50) / 100
		} else {
			value = (value + 50) / 100
		}
		dst[i] = int16(max(-32768, min(32767, value)))
	}
	return nil
}
