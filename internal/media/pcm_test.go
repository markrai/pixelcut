package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPCMDecoderPersistentStereoAndRetirement(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.Join(t.TempDir(), "tone.m4a")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=1", "-c:a", "aac", path)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	d := NewPCMDecoder(ctx, Default())
	defer d.Close()
	req := PCMRequest{ClipID: "tone", Path: path, Tick: 250000, Frames: PCMBlockFrames, SourceFrame: 12000, SourceEnd: 48000}
	first, err := d.DecodePCM(ctx, req)
	if err != nil || len(first) != PCMBlockFrames*PCMChannels {
		t.Fatalf("PCM decode: %v", err)
	}
	nonzero := false
	for _, v := range first {
		nonzero = nonzero || v != 0
	}
	if !nonzero {
		t.Fatal("decoded tone was silent")
	}
	stream := d.streams["tone"]
	req.Offset += PCMBlockFrames
	req.Tick += 50000
	req.SourceFrame += PCMBlockFrames
	if _, err = d.DecodePCM(ctx, req); err != nil || d.streams["tone"] != stream {
		t.Fatalf("PCM stream not reused: %v", err)
	}
	if err = d.Retain(nil); err != nil || stream.cmd.ProcessState == nil || len(d.streams) != 0 {
		t.Fatal("PCM stream was not reaped")
	}
	if err = d.Retain([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}); err == nil {
		t.Fatal("unbounded stream count accepted")
	}
	cancel()
	if _, err = d.DecodePCM(ctx, req); err == nil {
		t.Fatal("canceled request accepted")
	}
}

func TestPCMDecoderCanonicalEndAndSourceDiscontinuity(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.Join(t.TempDir(), "tone.m4a")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=44100:duration=0.25", "-c:a", "aac", path)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	d := NewPCMDecoder(ctx, Default())
	defer d.Close()
	req := PCMRequest{ClipID: "tone", Path: path, Frames: 100, SourceFrame: 2400, SourceEnd: 2700}
	if _, err := d.DecodePCM(ctx, req); err != nil {
		t.Fatal(err)
	}
	old := d.streams["tone"]
	req.Offset = 100
	req.SourceFrame = 2600
	if _, err := d.DecodePCM(ctx, req); err != nil {
		t.Fatal(err)
	}
	if old.cmd.ProcessState == nil || len(d.streams) != 0 || d.Stats().Opened != 2 || d.Stats().Retired != 2 {
		t.Fatalf("discontinuous or completed source interval was not retired: %+v", d.Stats())
	}
	req.SourceEnd = req.SourceFrame
	if _, err := d.DecodePCM(ctx, req); err == nil {
		t.Fatal("empty source interval accepted")
	}
}
