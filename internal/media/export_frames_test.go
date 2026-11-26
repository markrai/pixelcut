package media

import (
	"strings"
	"testing"
)

func TestStrictExportOrdinalIndex(t *testing.T) {
	const input = "frame|best_effort_timestamp_time=1.000000\nframe|best_effort_timestamp_time=1.100000\nframe|best_effort_timestamp_time=1.250000\nformat|start_time=1.000000\n"
	index, err := readFrameIndexMode(strings.NewReader(input), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tick int64
		want int
	}{{0, 0}, {99999, 0}, {100000, 1}, {249999, 1}, {250000, 2}} {
		if got, ok := index.OrdinalAtOrBefore(tc.tick); !ok || got != tc.want {
			t.Fatalf("tick %d ordinal %d want %d", tc.tick, got, tc.want)
		}
	}
	for _, input := range []string{
		"frame|best_effort_timestamp_time=0.900000\nformat|start_time=1.000000\n",
		"frame|best_effort_timestamp_time=1.100000\nframe|best_effort_timestamp_time=1.000000\nformat|start_time=1.000000\n",
		"frame|best_effort_timestamp_time=1.000000\nframe|best_effort_timestamp_time=1.000000\nformat|start_time=1.000000\n",
	} {
		if _, err := readFrameIndexMode(strings.NewReader(input), true); err == nil {
			t.Fatal("ambiguous source ordinal sequence accepted")
		}
	}
}
