package media

import (
	"encoding/binary"
	"errors"
	"math"
)

func (f *FrameIndex) MarshalBinary() ([]byte, error) {
	if f == nil || len(f.ticks) == 0 || len(f.ticks) > MaxIndexedFrames {
		return nil, errors.New("invalid frame index")
	}
	b := make([]byte, len(f.ticks)*8)
	for i, v := range f.ticks {
		binary.LittleEndian.PutUint64(b[i*8:], uint64(v))
	}
	return b, nil
}

func DecodeFrameIndex(b []byte) (*FrameIndex, error) {
	if len(b) == 0 || len(b)%8 != 0 || len(b)/8 > MaxIndexedFrames {
		return nil, errors.New("invalid cached frame count")
	}
	f := &FrameIndex{ticks: make([]int64, len(b)/8)}
	for i := range f.ticks {
		v := binary.LittleEndian.Uint64(b[i*8:])
		if v > math.MaxInt64 || (i > 0 && int64(v) <= f.ticks[i-1]) {
			return nil, errors.New("invalid cached frame timestamp")
		}
		f.ticks[i] = int64(v)
	}
	return f, nil
}
