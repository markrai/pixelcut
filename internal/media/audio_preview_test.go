package media

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAudioPreviewArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{MPV: exe}
	path := filepath.Join(t.TempDir(), "source.mp4")
	cmd, err := e.AudioPreview(context.Background(), path, 2, 3, "pipe", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"--vid=no", "--aid=auto", "--force-window=no", "--audio-display=no", "--start=2.000000", "--end=5.000000", "--pause=yes", "--input-ipc-server=pipe", path} {
		if !slices.Contains(cmd.Args, arg) {
			t.Fatalf("missing %s: %v", arg, cmd.Args)
		}
	}
	for _, arg := range cmd.Args {
		if arg == "--vo=gpu-next" || arg == "--wid=1" || arg == "--aid=no" {
			t.Fatalf("unexpected audio argument: %s", arg)
		}
	}
	if _, err := e.AudioPreview(context.Background(), path, 0, 0, "pipe", false); err == nil {
		t.Fatal("accepted zero duration")
	}
	muted, err := e.EmbeddedPreviewMode(context.Background(), path, 1, 2, 3, true, "", "pipe", false, "", true)
	if err != nil || !slices.Contains(muted.Args, "--aid=no") || !slices.Contains(muted.Args, "--target-colorspace-hint=yes") {
		t.Fatalf("detached video preview lost mute or HDR option: %v", err)
	}
	linked, err := e.EmbeddedPreview(context.Background(), path, 1, 2, 3, true, "", "pipe", false, "")
	if err != nil || slices.Contains(linked.Args, "--aid=no") {
		t.Fatal("ordinary video preview muted")
	}
}
