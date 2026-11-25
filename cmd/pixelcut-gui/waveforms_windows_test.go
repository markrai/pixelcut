package main

import "testing"

func TestWaveformInterval(t *testing.T) {
	for _, tc := range []struct {
		x, r       int32
		start, end int64
	}{
		{100, 102, 2000000, 2020000},
		{150, 152, 2500000, 2520000},
		{98, 100, 2000000, 2000000},
		{398, 400, 4980000, 5000000},
		{400, 402, 5000000, 5000000},
	} {
		a, b := waveformInterval(2000000, 5000000, 100, 100, tc.x, tc.r)
		if a != tc.start || b != tc.end {
			t.Fatalf("pixels %d-%d mapped to %d-%d, want %d-%d", tc.x, tc.r, a, b, tc.start, tc.end)
		}
	}
}
