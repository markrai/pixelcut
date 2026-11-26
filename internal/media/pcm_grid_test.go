package media

import (
	"math"
	"slices"
	"testing"
)

func TestPCMSourceFrameGrid(t *testing.T) {
	for _, tc := range []struct {
		source, timeline, start, offset, want int64
	}{
		{20000, 10001, 0, 481, 960},
		{0, 10001, 0, 481, 0},
		{20000, 10001, 150000, 0, 7679},
		{20000, 10001, 0, 7200, 7679},
		{0, 1, 0, 1, 0},
		{0, 0, 1, 0, 0},
		{0, math.MaxInt64, 0, PCMFramesForTicks(math.MaxInt64), 0},
	} {
		got, err := PCMSourceFrame(tc.source, tc.timeline, tc.start, tc.offset)
		if err != nil || got != tc.want {
			t.Fatalf("grid %+v: %d %v", tc, got, err)
		}
	}
	for _, tc := range [][4]int64{{-1, 0, 0, 0}, {0, 1, 0, 0}, {math.MaxInt64, 0, 1, 0}, {1000000, 0, 0, math.MaxInt64}} {
		if _, err := PCMSourceFrame(tc[0], tc[1], tc[2], tc[3]); err == nil {
			t.Fatalf("invalid grid accepted: %v", tc)
		}
	}
	args, err := PCMDecodeArgs("source.mp4", 960, 5760)
	if err != nil || !slices.Contains(args, "-copyts") || !slices.Contains(args, "-start_at_zero") || slices.Contains(args, "-ss") || !slices.Contains(args, "aresample=48000:async=1:first_pts=0,apad=whole_len=5760,atrim=start_sample=960:end_sample=5760,asetpts=N/SR/TB") {
		t.Fatalf("unexpected canonical PCM args: %v %v", args, err)
	}
	if _, err := PCMDecodeArgs("source.mp4", 4, 4); err == nil {
		t.Fatal("empty sample interval accepted")
	}
}
