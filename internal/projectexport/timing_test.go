package projectexport

import (
	"fmt"
	"strings"
	"testing"
)

func TestExactPacketGrid(t *testing.T) {
	for _, fps := range []int{24, 25, 30, 50, 60} {
		t.Run(fmt.Sprint(fps), func(t *testing.T) {
			step := exportTimescale / fps
			record := func(n, duration int) string { return fmt.Sprintf("packet|pts=%d|duration=%d\n", n, duration) }
			valid := record(0, step) + record(2*step, step) + record(step, step)
			if err := verifyPacketGrid(strings.NewReader(valid), fps, 3); err != nil {
				t.Fatal(err)
			}
			for _, input := range []string{
				record(0, step) + record(step, step),
				record(0, step) + record(step, step) + record(step, step),
				record(0, step) + record(step+1, step) + record(2*step, step),
				record(0, step) + record(step, step-1) + record(2*step, step),
				record(0, step) + record(step, step) + record(3*step, step),
				record(-step, step) + record(step, step) + record(2*step, step),
				"packet|pts=N/A|duration=8000\n",
				"packet|pts=0\n",
			} {
				if err := verifyPacketGrid(strings.NewReader(input), fps, 3); err == nil {
					t.Fatalf("invalid grid accepted: %s", input)
				}
			}
		})
	}
}

func TestExportRejectsUnsampledVideo(t *testing.T) {
	p := exportFixture(t)
	p.Tracks[0].Clips[0].Start = 1
	p.Tracks[0].Clips[0].Out = p.Tracks[0].Clips[0].In + 1
	p.Tracks[2].Clips = nil
	plan, err := Build(p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateVideo(plan, true); err == nil {
		t.Fatal("all-black output accepted when no visible video has a CFR sample")
	}
}
