package main

import (
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/session"
)

func TestPreferenceMenu(t *testing.T) {
	p := session.DefaultPreferences()
	for _, tc := range []struct {
		id      int
		checked bool
	}{
		{prefSnap, true}, {prefRipple, false}, {prefFrames, false}, {prefRestore, true},
		{prefSDR, true}, {prefHDR, false}, {prefSpeed2, true}, {prefSpeed0, false},
		{prefView1, true}, {prefView5, false}, {prefView15, false},
	} {
		item, ok := findMenuItem(preferenceItems(p), tc.id)
		if !ok || !item.enabled || item.checked != tc.checked {
			t.Fatalf("preference %d: %+v", tc.id, item)
		}
	}
	p.Snap = false
	p.FrameSnap = true
	p.Ripple = true
	p.HDR = true
	p.RestorePreview = false
	p.Speed = 2
	p.ViewSeconds = 15
	for _, id := range []int{prefFrames, prefRipple, prefHDR, prefSpeed4, prefView15} {
		item, ok := findMenuItem(preferenceItems(p), id)
		if !ok || !item.checked {
			t.Fatalf("preference %d not selected", id)
		}
	}
	if engineForPreferences(session.DefaultPreferences()) != media.Default() {
		t.Fatal("default engine changed")
	}
	p.MPV = filepath.Join(t.TempDir(), "mpv.exe")
	e := engineForPreferences(p)
	if e.MPV != p.MPV || e.FFmpeg != media.Default().FFmpeg {
		t.Fatal("tool override mismatch")
	}
}

func TestLoadEditingPreferences(t *testing.T) {
	p := session.DefaultPreferences()
	p.Snap = false
	p.Ripple = true
	p.FrameSnap = true
	p.Speed = .5
	p.ViewSeconds = 5
	dir := t.TempDir()
	if err := session.SavePreferences(dir, p); err != nil {
		t.Fatal(err)
	}
	e := editor{recentDir: dir}
	e.loadPreferences()
	if e.prefs != p || !e.snapDisabled || !e.rippleTrim || !e.frameSnap || e.speed() != .5 {
		t.Fatal("preferences not applied")
	}
	e.rippleTrim = false
	if note := e.rememberPreferences(); note != "" {
		t.Fatal(note)
	}
	got, err := session.LoadPreferences(dir)
	if err != nil || got.Ripple || got.Speed != .5 {
		t.Fatal("editing toggle not persisted")
	}
}

func TestGPUInfoPreferenceAction(t *testing.T) {
	v, ok := findMenuItem(preferenceItems(session.DefaultPreferences()), prefGPUInfo)
	if !ok || !v.enabled {
		t.Fatal("GPU diagnostics unavailable")
	}
}

func TestDecodeBackendMenuAndEngine(t *testing.T) {
	p := session.DefaultPreferences()
	for _, tc := range []struct {
		mode string
		id   int
	}{{"", prefDecodeCPU}, {"cpu", prefDecodeCPU}, {"auto", prefDecodeAuto}, {"cuda", prefDecodeCUDA}} {
		p.DecodeBackend = tc.mode
		for _, id := range []int{prefDecodeCPU, prefDecodeAuto, prefDecodeCUDA} {
			item, ok := findMenuItem(preferenceItems(p), id)
			if !ok || !item.enabled || item.checked != (id == tc.id) {
				t.Fatalf("decode menu mode %q: %+v", tc.mode, item)
			}
		}
		if engineForPreferences(p).DecodeBackend != media.Backend(tc.mode) {
			t.Fatal("decode preference not applied to engine")
		}
	}
}
