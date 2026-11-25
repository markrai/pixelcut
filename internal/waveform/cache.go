package waveform

import (
	"encoding/binary"
	"errors"
)

func (p *Peaks) MarshalBinary() ([]byte, error) {
	if p == nil || p.size <= 0 || len(p.tree) != p.size*2 {
		return nil, errors.New("invalid waveform")
	}
	b := make([]byte, 16+p.size*2)
	binary.LittleEndian.PutUint64(b, uint64(p.frames))
	binary.LittleEndian.PutUint64(b[8:], uint64(p.framesPerBin))
	for i, v := range p.tree[p.size:] {
		binary.LittleEndian.PutUint16(b[16+i*2:], v)
	}
	return b, nil
}

func DecodePeaks(b []byte, duration int64) (*Peaks, error) {
	if duration <= 0 || duration > MaxDuration || len(b) < 18 {
		return nil, errors.New("invalid cached waveform")
	}
	frames := (duration*SampleRate + TicksPerSecond - 1) / TicksPerSecond
	step := max(int64(80), (frames+maxBins-1)/maxBins)
	bins := int((frames + step - 1) / step)
	size := 1
	for size < bins {
		size *= 2
	}
	if binary.LittleEndian.Uint64(b) != uint64(frames) || binary.LittleEndian.Uint64(b[8:]) != uint64(step) || len(b) != 16+size*2 {
		return nil, errors.New("cached waveform geometry mismatch")
	}
	p := &Peaks{framesPerBin: step, frames: frames, size: size, tree: make([]uint16, size*2)}
	for i := 0; i < size; i++ {
		v := binary.LittleEndian.Uint16(b[16+i*2:])
		if v > 32768 || (i >= bins && v != 0) {
			return nil, errors.New("invalid cached waveform peak")
		}
		p.tree[size+i] = v
	}
	for i := size - 1; i > 0; i-- {
		p.tree[i] = max(p.tree[i*2], p.tree[i*2+1])
	}
	return p, nil
}
