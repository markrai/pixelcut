package media

import (
	"reflect"
	"testing"
)

func TestFrameCacheRoundTrip(t *testing.T) {
	f := &FrameIndex{ticks: []int64{0, 41667, 100000, 120000}}
	b, err := f.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	g, err := DecodeFrameIndex(b)
	if err != nil || !reflect.DeepEqual(f, g) {
		t.Fatal("frame cache roundtrip")
	}
	for _, v := range [][]byte{nil, b[:7], append(append([]byte(nil), b...), b[:8]...)} {
		if _, err := DecodeFrameIndex(v); err == nil {
			t.Fatal("invalid frame cache accepted")
		}
	}
}
