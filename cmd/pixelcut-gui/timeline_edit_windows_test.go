package main

import "testing"

func TestTimelineEditPreservesPlayhead(t *testing.T) {
	for _, tc := range []struct {
		marker, end int64
		zoom, scale float64
		want        int64
		wantZoom    float64
	}{
		{3000000, 10000000, 0, 42, 3000000, 42},
		{3000000, 6000000, 80, 80, 3000000, 80},
		{9000000, 6000000, 80, 80, 6000000, 80},
		{3000000, 10000000, 0, .01, 3000000, 0},
	} {
		m, z := timelineEditView(tc.marker, tc.end, tc.zoom, tc.scale)
		if m != tc.want || z != tc.wantZoom {
			t.Fatalf("marker/zoom changed unexpectedly: %d %f", m, z)
		}
	}
}
