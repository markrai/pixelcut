package media

import (
	"math"
	"testing"
)

func TestAudioLevelsAndDBFS(t *testing.T) {
	if !math.IsInf(PeakDBFS(0), -1) || PeakDBFS(32768) != 0 || math.Abs(PeakDBFS(16384)+6.020599913) > 0.000001 {
		t.Fatal("invalid dBFS mapping")
	}
	var l AudioLevels
	l.Merge(AudioLevels{Frames: 10, Peak: [2]uint32{32767, 123}, Clipped: [2]int64{2, 0}})
	l.Merge(AudioLevels{Frames: 20, Peak: [2]uint32{321, 32768}, Clipped: [2]int64{0, 3}})
	if l.Frames != 30 || l.ClipCount() != 5 || l.Peak != [2]uint32{32767, 32768} {
		t.Fatalf("incorrect level aggregation: %+v", l)
	}
	if PeakLabel(0) != "-inf" || PeakLabel(16384) != "-6.0" {
		t.Fatal("incorrect peak labels")
	}
}
