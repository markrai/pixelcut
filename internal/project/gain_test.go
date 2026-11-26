package project

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestGainVersionAndPersistence(t *testing.T) {
	p := fixture(t)
	p.Version = LayerVersion
	p.CanvasWidth, p.CanvasHeight = 1080, 1920
	opacity := 50
	p.Tracks[0].Clips[0].Opacity = &opacity
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	before := p.Fingerprint()
	q, err := p.SetClipGain(ref, 100)
	if err != nil || q.Fingerprint() != before || q.Version != LayerVersion {
		t.Fatal("unity gain unexpectedly migrated a version 5 project")
	}
	q, err = p.SetClipGain(ref, 0)
	if err != nil || q.Version != Version || p.Fingerprint() != before || q.Tracks[0].Clips[0].GainPercent() != 0 {
		t.Fatalf("zero gain edit: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gain.pixelcut.json")
	if err := q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Fingerprint() != q.Fingerprint() || loaded.Tracks[0].Clips[0].GainPercent() != 0 {
		t.Fatalf("gain save/load: %v", err)
	}
	bad := q.Clone()
	bad.Version = LayerVersion
	if !errors.Is(bad.Validate(), ErrUnsupportedVersion) {
		t.Fatal("version 5 accepted version 6 gain")
	}
	for _, gain := range []int{-1, 401} {
		if _, err := p.SetClipGain(ref, gain); err == nil {
			t.Fatal("invalid gain accepted")
		}
		bad := q.Clone()
		bad.Tracks[0].Clips[0].Gain = &gain
		if bad.Validate() == nil {
			t.Fatal("invalid persisted gain accepted")
		}
	}
	q, err = q.SetClipGain(ref, 100)
	if err != nil || q.Tracks[0].Clips[0].Gain != nil || q.Version != Version {
		t.Fatal("unity reset did not clear gain field or downgraded schema")
	}
	p.Tracks[0].Locked = true
	if _, err := p.SetClipGain(ref, 50); !errors.Is(err, ErrTrackLocked) {
		t.Fatal("locked track gain changed")
	}
	p.Tracks[0].Locked = false
	p.Assets[0].HasAudio = false
	if _, err := p.SetClipGain(ref, 50); err == nil {
		t.Fatal("silent source gain accepted")
	}
}

func TestGainSurvivesClipOperations(t *testing.T) {
	p := fixture(t)
	v := ClipRef{TrackID: "V1", ClipID: "c"}
	a := ClipRef{TrackID: "A1", ClipID: "sound"}
	q, err := p.SetClipGain(v, 175)
	if err != nil {
		t.Fatal(err)
	}
	clone := q.Clone()
	*clone.Tracks[0].Clips[0].Gain = 5
	if q.Tracks[0].Clips[0].GainPercent() != 175 {
		t.Fatal("gain aliases original")
	}
	split, err := q.Split("V1", "c", "right", 3000000)
	if err != nil {
		t.Fatal(err)
	}
	*split.Tracks[0].Clips[1].Gain = 20
	if split.Tracks[0].Clips[0].GainPercent() != 175 || q.Tracks[0].Clips[0].GainPercent() != 175 {
		t.Fatal("split gain aliases original")
	}
	clipboard, err := q.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	p.Version = LayerVersion
	paste, err := p.PasteClips("V1", clipboard, 9000000, []string{"copy"})
	if err != nil || paste.Project.Version != Version || paste.Project.Tracks[0].Clips[1].GainPercent() != 175 {
		t.Fatalf("paste lost gain: %v", err)
	}
	detached, err := q.DetachAudio(v, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	video, _ := detached.Clip(v)
	audio, _ := detached.Clip(a)
	if !video.AudioDetached || video.Gain != nil || audio.GainPercent() != 175 {
		t.Fatal("gain was not moved to detached audio")
	}
	if _, err := detached.SetClipGain(v, 20); err == nil {
		t.Fatal("detached video accepted audio gain")
	}
	detached, err = detached.SetClipGain(a, 25)
	if err != nil {
		t.Fatal(err)
	}
	reattached, err := detached.ReattachAudio(v, a)
	if err != nil || reattached.Tracks[0].Clips[0].GainPercent() != 25 || len(reattached.Tracks[1].Clips) != 0 {
		t.Fatalf("reattach lost edited audio gain: %v", err)
	}
	video, _ = reattached.Clip(v)
	if _, _, err := reattached.MediaEdit(video); err == nil {
		t.Fatal("selected-clip export silently ignored gain")
	}
	reattached, err = reattached.SetClipGain(v, 0)
	if err != nil {
		t.Fatal(err)
	}
	video, _ = reattached.Clip(v)
	_, edit, err := reattached.MediaEdit(video)
	if err != nil || !edit.Silent {
		t.Fatal("selected-clip export ignored zero gain")
	}
	if _, err := q.SetCompositionCanvas(media.CanvasSize{Width: 64, Height: 48}); err != nil {
		t.Fatal(err)
	}
}
