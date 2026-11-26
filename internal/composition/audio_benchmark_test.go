package composition

import (
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func BenchmarkCompositePreviewAudioAutomation(b *testing.B) {
	samples := make([]int16, media.PCMBlockFrames*media.PCMChannels)
	for i := range samples {
		samples[i] = int16((i%2000 - 1000) * 20)
	}
	out := make([]int16, len(samples))
	a := AudioContribution{
		Contribution: Contribution{
			Asset:          project.Asset{Duration: 1000000},
			Timeline:       project.Interval{Start: 0, End: 1000000},
			SourceInterval: project.Interval{Start: 0, End: 1000000},
		},
		GainPercent: 75,
		FadeIn:      200000,
		FadeOut:     200000,
		Envelope:    &project.VolumeEnvelope{Points: []project.VolumeKeyframe{{SourceTime: 0, Percent: 100}, {SourceTime: 100000, Percent: 20}, {SourceTime: 1000000, Percent: 100}}},
	}
	for _, tc := range []struct {
		name      string
		automated bool
		streams   int
	}{
		{"UnityOne", false, 1},
		{"AutomatedEight", true, media.MaxAudioStreams},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var m PCMMixer
			if err := m.Reset(media.PCMBlockFrames); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(samples) * 2))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var levels media.AudioLevels
				if err := m.Reset(media.PCMBlockFrames); err != nil {
					b.Fatal(err)
				}
				for j := 0; j < tc.streams; j++ {
					var err error
					if tc.automated {
						err = m.AddContribution(samples, a, 0, 2400)
					} else {
						err = m.Add(samples, 100)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				if err := m.FinishObserved(out, &levels); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
