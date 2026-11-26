package project

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestClipMutePreservesGainAndSchema(t *testing.T) {
	p := fixture(t)
	gain := 75
	p.Version = GainVersion
	p.Tracks[0].Clips[0].Gain = &gain
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	before := p.Fingerprint()
	q, err := p.SetClipMuted(ref, false)
	if err != nil || q.Fingerprint() != before || q.Version != GainVersion {
		t.Fatal("unmute no-op migrated old project")
	}
	q, err = p.SetClipMuted(ref, true)
	if err != nil || q.Version != Version || p.Fingerprint() != before {
		t.Fatalf("mute edit: %v", err)
	}
	c, _ := q.Clip(ref)
	if !c.AudioMuted || c.GainPercent() != 75 || c.EffectiveGainPercent() != 0 {
		t.Fatal("mute destroyed gain")
	}
	if _, edit, err := q.MediaEdit(c); err != nil || !edit.Silent {
		t.Fatal("muted non-unity gain could not export silent video")
	}
	q, err = q.SetClipGain(ref, 160)
	if err != nil {
		t.Fatal(err)
	}
	c, _ = q.Clip(ref)
	if !c.AudioMuted || c.GainPercent() != 160 || c.EffectiveGainPercent() != 0 {
		t.Fatal("gain edit unmuted clip")
	}
	path := filepath.Join(t.TempDir(), "muted.pixelcut.json")
	if err := q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Fingerprint() != q.Fingerprint() {
		t.Fatalf("mute persistence: %v", err)
	}
	bad := loaded.Clone()
	bad.Version = GainVersion
	if !errors.Is(bad.Validate(), ErrUnsupportedVersion) {
		t.Fatal("version 6 accepted mute field")
	}
	q, err = loaded.SetClipMuted(ref, false)
	c, _ = q.Clip(ref)
	if err != nil || c.AudioMuted || c.EffectiveGainPercent() != 160 {
		t.Fatal("unmute failed to restore current gain")
	}
	if _, edit, err := q.MediaEditWithMute(c, true); err != nil || !edit.Silent {
		t.Fatal("track mute did not override unsupported clip gain")
	}
	q.Tracks[0].Locked = true
	if _, err := q.SetClipMuted(ref, true); !errors.Is(err, ErrTrackLocked) {
		t.Fatal("locked track changed")
	}
	p.Assets[0].HasAudio = false
	p.Tracks[0].Clips[0].Gain = nil
	if _, err := p.SetClipMuted(ref, true); err == nil {
		t.Fatal("silent asset accepted mute")
	}
}

func TestClipMuteSurvivesSplitPasteAndDetach(t *testing.T) {
	p := fixture(t)
	v := ClipRef{TrackID: "V1", ClipID: "c"}
	a := ClipRef{TrackID: "A1", ClipID: "sound"}
	q, err := p.SetClipMuted(v, true)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.SetClipGain(v, 125)
	if err != nil {
		t.Fatal(err)
	}
	split, err := q.Split("V1", "c", "right", 3000000)
	if err != nil || !split.Tracks[0].Clips[1].AudioMuted || split.Tracks[0].Clips[1].GainPercent() != 125 {
		t.Fatalf("split lost mute/gain: %v", err)
	}
	b, err := q.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	p.Version = GainVersion
	paste, err := p.PasteClips("V1", b, 9000000, []string{"copy"})
	if err != nil || paste.Project.Version != Version || !paste.Project.Tracks[0].Clips[1].AudioMuted {
		t.Fatalf("paste lost mute: %v", err)
	}
	r, err := q.DetachAudio(v, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	vc, _ := r.Clip(v)
	ac, _ := r.Clip(a)
	if vc.AudioMuted || vc.Gain != nil || !ac.AudioMuted || ac.GainPercent() != 125 {
		t.Fatal("detach lost mute/gain ownership")
	}
	if _, err := r.SetClipMuted(v, true); err == nil {
		t.Fatal("detached video accepted mute")
	}
	r, err = r.ReattachAudio(v, a)
	if err != nil {
		t.Fatal(err)
	}
	vc, _ = r.Clip(v)
	if !vc.AudioMuted || vc.GainPercent() != 125 || len(r.Tracks[1].Clips) != 0 {
		t.Fatal("reattach lost mute/gain")
	}
}
