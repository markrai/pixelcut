package composition

import (
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestVolumeEnvelopeMixerUsesSourcePosition(t *testing.T) {
	a := AudioContribution{
		Contribution: Contribution{
			Asset:          project.Asset{Duration: 1000000},
			Timeline:       project.Interval{Start: 100000, End: 300000},
			SourceInterval: project.Interval{Start: 500000, End: 700000},
		},
		GainPercent: 200,
		FadeIn:      100000,
		Envelope:    &project.VolumeEnvelope{Points: []project.VolumeKeyframe{{SourceTime: 500000, Percent: 0}, {SourceTime: 600000, Percent: 50}, {SourceTime: 700000, Percent: 100}}},
	}
	var m PCMMixer
	for _, tc := range []struct {
		start, offset int64
		want          []int16
	}{
		{100000, 0, []int16{0, 0}},
		{150000, 0, []int16{5000, -5000}},
		{0, 7200, []int16{5000, -5000}},
		{200000, 0, []int16{20000, -20000}},
		{250000, 0, []int16{30000, -30000}},
	} {
		if err := m.Reset(1); err != nil {
			t.Fatal(err)
		}
		if err := m.AddContribution([]int16{20000, -20000}, a, tc.start, tc.offset); err != nil {
			t.Fatal(err)
		}
		out := make([]int16, 2)
		if err := m.Finish(out); err != nil || !reflect.DeepEqual(out, tc.want) {
			t.Fatalf("envelope %+v got %v: %v", tc, out, err)
		}
	}
}
