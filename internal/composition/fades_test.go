package composition

import (
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestFadeFactors(t *testing.T) {
	interval := project.Interval{Start: 1000000, End: 2000000}
	for _, tc := range []struct {
		at, in, out, want int64
	}{
		{1000000, 500000, 500000, 0},
		{1250000, 500000, 500000, 500000},
		{1500000, 500000, 500000, 1000000},
		{1750000, 500000, 500000, 500000},
		{2000000, 500000, 500000, 0},
		{1500000, 1000000, 1000000, 250000},
		{1500000, 2000000, 2000000, 250000},
		{999999, 0, 0, 0},
	} {
		if got := fadeFactor(interval, tc.in, tc.out, tc.at); got != tc.want {
			t.Fatalf("fade %+v got %d", tc, got)
		}
	}
}

func TestFadeMixerSampleRampAndSeek(t *testing.T) {
	a := AudioContribution{
		Contribution: Contribution{Timeline: project.Interval{Start: 0, End: 200000}},
		GainPercent:  100,
		FadeIn:       100000,
		FadeOut:      100000,
	}
	var m PCMMixer
	samples := []int16{20000, -20000, 20000, -20000}
	out := make([]int16, len(samples))
	for _, tc := range []struct {
		start, offset int64
		want          []int16
	}{
		{0, 0, []int16{0, 0, 4, -4}},
		{0, 2400, []int16{10000, -10000, 10004, -10004}},
		{50000, 0, []int16{10000, -10000, 10004, -10004}},
		{150000, 0, []int16{10000, -10000, 9996, -9996}},
	} {
		if err := m.Reset(2); err != nil {
			t.Fatal(err)
		}
		if err := m.AddContribution(samples, a, tc.start, tc.offset); err != nil {
			t.Fatal(err)
		}
		if err := m.Finish(out); err != nil || !reflect.DeepEqual(out, tc.want) {
			t.Fatalf("fade %+v got %v: %v", tc, out, err)
		}
	}
}
