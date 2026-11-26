package project

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFadeModelAndEditSemantics(t *testing.T) {
	p := fixture(t)
	p.Version = MuteVersion
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	before := p.Fingerprint()
	q, err := p.SetClipFades(ref, 0, 0)
	if err != nil || q.Fingerprint() != before {
		t.Fatal("no-op fade changed legacy project")
	}
	q, err = p.SetClipFades(ref, 1000000, 2000000)
	if err != nil || q.Version != Version || p.Fingerprint() != before {
		t.Fatalf("fade edit: %v", err)
	}
	c, _ := q.Clip(ref)
	if !c.HasFades() || c.FadeIn != 1000000 || c.FadeOut != 2000000 {
		t.Fatal("wrong fade settings")
	}
	path := filepath.Join(t.TempDir(), "fade.pixelcut.json")
	if err := q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Fingerprint() != q.Fingerprint() {
		t.Fatalf("fade round trip: %v", err)
	}
	old := q.Clone()
	old.Version = MuteVersion
	if !errors.Is(old.Validate(), ErrUnsupportedVersion) {
		t.Fatal("old schema accepted fades")
	}
	for _, v := range []int64{-1, MaxAudioFade + 1} {
		if _, err := q.SetClipFades(ref, v, 0); err == nil {
			t.Fatal("invalid fade accepted")
		}
	}
	split, err := q.Split("V1", "c", "right", 3000000)
	if err != nil {
		t.Fatal(err)
	}
	left, right := split.Tracks[0].Clips[0], split.Tracks[0].Clips[1]
	if left.FadeIn != c.FadeIn || left.FadeOut != 0 || right.FadeIn != 0 || right.FadeOut != c.FadeOut {
		t.Fatal("split added an internal fade or lost an outer fade")
	}
	trim, err := q.DragClip("V1", "c", "right", -c.Duration()+1)
	if err != nil || trim.Duration() != 1 || trim.FadeIn != c.FadeIn || trim.FadeOut != c.FadeOut {
		t.Fatalf("trim lost configured fade: %v", err)
	}
	b, err := q.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	paste, err := p.PasteClips("V1", b, 9000000, []string{"copy"})
	if err != nil || paste.Project.Version != Version || paste.Project.Tracks[0].Clips[1].FadeOut != c.FadeOut {
		t.Fatalf("paste lost fade: %v", err)
	}
	if _, _, err := q.MediaEdit(c); err == nil {
		t.Fatal("selected-clip export silently ignored fade")
	}
	q, err = q.SetClipMuted(ref, true)
	if err != nil {
		t.Fatal(err)
	}
	c, _ = q.Clip(ref)
	if _, edit, err := q.MediaEdit(c); err != nil || !edit.Silent {
		t.Fatal("muted fade could not export silent video")
	}
	q.Tracks[0].Locked = true
	if _, err := q.SetClipFades(ref, 0, 0); !errors.Is(err, ErrTrackLocked) {
		t.Fatal("locked fade changed")
	}
}

func TestFadeDetachReattach(t *testing.T) {
	p := fixture(t)
	v := ClipRef{TrackID: "V1", ClipID: "c"}
	a := ClipRef{TrackID: "A1", ClipID: "sound"}
	q, err := p.SetClipFades(v, 100000, 250000)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.DetachAudio(v, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	vc, _ := q.Clip(v)
	ac, _ := q.Clip(a)
	if vc.HasFades() || ac.FadeIn != 100000 || ac.FadeOut != 250000 {
		t.Fatal("detach did not transfer fades")
	}
	q, err = q.SetClipFades(a, 500000, 0)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.ReattachAudio(v, a)
	if err != nil {
		t.Fatal(err)
	}
	vc, _ = q.Clip(v)
	if vc.FadeIn != 500000 || vc.FadeOut != 0 {
		t.Fatal("reattach lost updated fades")
	}
}
