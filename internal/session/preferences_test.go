package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := LoadPreferences(dir)
	if err != nil || p != DefaultPreferences() {
		t.Fatal("missing preferences did not use defaults")
	}
	p.Snap = false
	p.Ripple = true
	p.FrameSnap = true
	p.Speed = 1.5
	p.HDR = true
	p.RestorePreview = false
	p.ViewSeconds = 15
	p.FFmpeg = filepath.Join(dir, "ffmpeg.exe")
	if err = SavePreferences(dir, p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPreferences(dir)
	if err != nil || got != p {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	q := p
	q.Speed = 1.25
	if err = SavePreferences(dir, q); err == nil {
		t.Fatal("invalid speed saved")
	}
	got, err = LoadPreferences(dir)
	if err != nil || got != p {
		t.Fatal("invalid write replaced preferences")
	}
	if err = SavePreferences(dir, DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	got, err = LoadPreferences(dir)
	if err != nil || got != DefaultPreferences() {
		t.Fatal("reset failed")
	}
}

func TestPreferencesInvalid(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []string{"null", "{}", "{\"version\":1,\"preview_speed\":1,\"view_checkpoint_seconds\":1,\"unexpected\":true}", "{\"version\":1,\"preview_speed\":1,\"view_checkpoint_seconds\":1,\"mpv\":\"relative.exe\"}"} {
		if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
		p, err := LoadPreferences(dir)
		if err == nil || p != DefaultPreferences() {
			t.Fatalf("accepted %s", s)
		}
	}
}

func TestDecodeBackendPreferences(t *testing.T) {
	dir := t.TempDir()
	p := DefaultPreferences()
	for _, mode := range []string{"", "cpu", "auto", "cuda"} {
		p.DecodeBackend = mode
		if err := SavePreferences(dir, p); err != nil {
			t.Fatal(err)
		}
		got, err := LoadPreferences(dir)
		if err != nil || got != p {
			t.Fatalf("decode preference round trip: %+v %v", got, err)
		}
	}
	p.DecodeBackend = "bogus"
	if err := SavePreferences(dir, p); err == nil {
		t.Fatal("unknown decode preference accepted")
	}
}

func TestEncodeBackendPreferences(t *testing.T) {
	dir := t.TempDir()
	p := DefaultPreferences()
	p.DecodeBackend = "cuda"
	for _, mode := range []string{"", "cpu", "nvenc"} {
		p.EncodeBackend = mode
		if err := SavePreferences(dir, p); err != nil {
			t.Fatal(err)
		}
		got, err := LoadPreferences(dir)
		if err != nil || got != p {
			t.Fatalf("encode preference round trip: %+v %v", got, err)
		}
	}
	p.EncodeBackend = "auto"
	if err := SavePreferences(dir, p); err == nil {
		t.Fatal("unsupported encoder accepted")
	}
	if err := SavePreferences(dir, DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPreferences(dir)
	if err != nil || got.EncodeBackend != "" {
		t.Fatal("default encoder not restored")
	}
}
