package media

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSourceAuditionGainArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{MPV: exe}
	path := filepath.Join(t.TempDir(), "source.mp4")
	for _, tc := range []struct {
		gain   int
		silent bool
		want   string
	}{
		{100, false, ""},
		{75, false, "--af=lavfi=[volume=0.75:precision=double]"},
		{400, false, "--af=lavfi=[volume=4.00:precision=double]"},
		{0, false, "--aid=no"},
		{175, true, "--aid=no"},
	} {
		cmd, err := e.EmbeddedPreviewGain(context.Background(), path, 1, 0, 1, true, "", "pipe", false, "", tc.silent, tc.gain)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		for _, arg := range cmd.Args {
			if arg == "--aid=no" || strings.HasPrefix(arg, "--af=") {
				if got != "" {
					t.Fatal("duplicate gain/mute argument")
				}
				got = arg
			}
		}
		if got != tc.want || !slices.Contains(cmd.Args, "--target-colorspace-hint=yes") {
			t.Fatalf("gain %d silent %t: %q want %q", tc.gain, tc.silent, got, tc.want)
		}
	}
	cmd, err := e.AudioPreviewGain(context.Background(), path, 0, 1, "pipe", false, 25)
	if err != nil || !slices.Contains(cmd.Args, "--af=lavfi=[volume=0.25:precision=double]") {
		t.Fatalf("independent audio gain missing: %v", err)
	}
	for _, gain := range []int{-1, 0, 401} {
		if _, err := e.AudioPreviewGain(context.Background(), path, 0, 1, "pipe", false, gain); err == nil {
			t.Fatal("invalid/silent audio-only audition accepted")
		}
	}
}
