package project

import (
	"errors"
	"math"
	"path/filepath"
	"testing"
)

func TestVolumeEnvelopeInterpolation(t *testing.T) {
	e := &VolumeEnvelope{Points: []VolumeKeyframe{{100, 100}, {200, 0}, {300, 80}}}
	for _, tc := range []struct {
		at, want int64
	}{
		{0, 1000000}, {100, 1000000}, {125, 750000}, {200, 0},
		{250, 400000}, {300, 800000}, {301, 800000},
	} {
		if got := e.Factor(tc.at); got != tc.want {
			t.Fatalf("at %d: got %d want %d", tc.at, got, tc.want)
		}
	}
	e = &VolumeEnvelope{Points: []VolumeKeyframe{{0, 0}, {math.MaxInt64, 100}}}
	if !e.Valid(math.MaxInt64) || e.Factor(math.MaxInt64/2) != 499999 || e.Factor(math.MaxInt64) != 1000000 {
		t.Fatal("long envelope interpolation overflowed")
	}
	var none *VolumeEnvelope
	if none.Factor(10) != EnvelopeScale {
		t.Fatal("missing envelope must be unity")
	}
}

func TestVolumeEnvelopeLifecycle(t *testing.T) {
	p := fixture(t)
	p.Version = FadeVersion
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	before := p.Fingerprint()
	q, err := p.SetClipEnvelope(ref, nil)
	if err != nil || q.Fingerprint() != before {
		t.Fatal("empty envelope changed old project")
	}
	points := []VolumeKeyframe{{2000000, 100}, {4000000, 20}, {7000000, 80}}
	q, err = p.SetClipEnvelope(ref, points)
	if err != nil || q.Version != Version || p.Fingerprint() != before {
		t.Fatalf("envelope transaction: %v", err)
	}
	points[0].Percent = 0
	c, _ := q.Clip(ref)
	if c.Envelope.Points[0].Percent != 100 {
		t.Fatal("caller owns stored envelope")
	}
	c.Envelope.Points[0].Percent = 0
	c, _ = q.Clip(ref)
	if c.Envelope.Points[0].Percent != 100 {
		t.Fatal("clip accessor aliases stored envelope")
	}
	path := filepath.Join(t.TempDir(), "envelope.pixelcut.json")
	if err := q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Fingerprint() != q.Fingerprint() {
		t.Fatalf("envelope persistence: %v", err)
	}
	old := q.Clone()
	old.Version = FadeVersion
	if !errors.Is(old.Validate(), ErrUnsupportedVersion) {
		t.Fatal("older schema accepted envelope")
	}
	for _, bad := range [][]VolumeKeyframe{
		{{-1, 0}}, {{10000001, 0}}, {{0, -1}}, {{0, 101}},
		{{0, 0}, {0, 1}}, {{2, 10}, {1, 10}},
		make([]VolumeKeyframe, MaxVolumeKeyframes+1),
	} {
		if _, err := q.SetClipEnvelope(ref, bad); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	trim, err := q.DragClip("V1", "c", "left", 2500000)
	if err != nil || trim.Envelope.Factor(4500000) != c.Envelope.Factor(4500000) || len(trim.Envelope.Points) != 3 {
		t.Fatalf("trim reshaped source-anchored envelope: %v", err)
	}
	split, err := q.Split("V1", "c", "right", 3000000)
	if err != nil {
		t.Fatal(err)
	}
	left, right := split.Tracks[0].Clips[0], split.Tracks[0].Clips[1]
	if left.Envelope.Factor(left.Out) != right.Envelope.Factor(right.In) {
		t.Fatal("split changed gain at seam")
	}
	right.Envelope.Points[1].Percent = 70
	if left.Envelope.Points[1].Percent != 20 || c.Envelope.Points[1].Percent != 20 {
		t.Fatal("split aliases keyframe arrays")
	}
	clipboard, err := q.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	paste, err := p.PasteClips("V1", clipboard, 9000000, []string{"copy"})
	if err != nil || paste.Project.Version != Version || !paste.Project.Tracks[0].Clips[1].HasEnvelope() {
		t.Fatalf("paste lost envelope: %v", err)
	}
	if _, _, err := q.MediaEdit(c); err == nil {
		t.Fatal("isolated export silently ignored envelope")
	}
	q, err = q.SetClipMuted(ref, true)
	if err != nil {
		t.Fatal(err)
	}
	c, _ = q.Clip(ref)
	if _, edit, err := q.MediaEdit(c); err != nil || !edit.Silent {
		t.Fatal("muted envelope did not export silent video")
	}
	q.Tracks[0].Locked = true
	if _, err := q.SetClipEnvelope(ref, nil); !errors.Is(err, ErrTrackLocked) {
		t.Fatal("locked envelope edit accepted")
	}
}

func TestVolumeEnvelopeAudioOwnership(t *testing.T) {
	p := fixture(t)
	v := ClipRef{TrackID: "V1", ClipID: "c"}
	a := ClipRef{TrackID: "A1", ClipID: "sound"}
	q, err := p.SetClipEnvelope(v, []VolumeKeyframe{{2000000, 80}})
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.DetachAudio(v, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	vc, _ := q.Clip(v)
	ac, _ := q.Clip(a)
	if vc.HasEnvelope() || !ac.HasEnvelope() || ac.Envelope.Factor(3000000) != 800000 {
		t.Fatal("detach lost envelope")
	}
	if _, err := q.SetClipEnvelope(v, []VolumeKeyframe{{2000000, 100}}); err == nil {
		t.Fatal("detached video accepted envelope")
	}
	q, err = q.SetClipEnvelope(a, []VolumeKeyframe{{2000000, 40}})
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.ReattachAudio(v, a)
	if err != nil {
		t.Fatal(err)
	}
	vc, _ = q.Clip(v)
	if !vc.HasEnvelope() || vc.Envelope.Factor(3000000) != 400000 {
		t.Fatal("reattach lost edited envelope")
	}
}
