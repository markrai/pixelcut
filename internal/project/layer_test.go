package project

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestLayerSettingsCompatibilityAndPersistence(t *testing.T) {
	p := fixture(t)
	p.Version = ControlsVersion
	p.Tracks[0].Muted = true
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	before := p.Fingerprint()
	if p.CompositionCanvas() != (media.CanvasSize{Width: 1000, Height: 1840}) {
		t.Fatal("automatic canvas did not use base crop geometry")
	}
	c, _ := p.Clip(ref)
	if c.OpacityPercent() != 100 {
		t.Fatal("legacy opacity changed")
	}
	q, err := p.SetClipOpacity(ref, 0)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.SetCompositionCanvas(media.CanvasSize{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.ApplyLayerPreset(ref, media.LayerBottomRight)
	if err != nil {
		t.Fatal(err)
	}
	c, _ = q.Clip(ref)
	if q.Version != Version || c.OpacityPercent() != 0 || c.Transform == nil || !q.Tracks[0].Muted || p.Fingerprint() != before {
		t.Fatal("layer edit changed original or lost existing state")
	}
	path := filepath.Join(t.TempDir(), "layer.pixelcut.json")
	if err = q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || !reflect.DeepEqual(loaded, q) {
		t.Fatalf("layer settings round trip: %v", err)
	}
	old := q.Clone()
	old.Version = ControlsVersion
	if old.Validate() == nil {
		t.Fatal("older schema accepted new layer fields")
	}
	if _, _, err := q.MediaEdit(c); err == nil {
		t.Fatal("isolated export silently ignored opacity")
	}
	r, err := q.SetClipOpacity(ref, 100)
	if err != nil {
		t.Fatal(err)
	}
	c, _ = r.Clip(ref)
	if c.Opacity != nil || c.OpacityPercent() != 100 {
		t.Fatal("opaque default not restored")
	}
	if _, _, err := r.MediaEdit(c); err != nil {
		t.Fatal(err)
	}
}

func TestLayerOpacityCloneSplitPasteAndLocks(t *testing.T) {
	p := fixture(t)
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	q, err := p.SetClipOpacity(ref, 25)
	if err != nil {
		t.Fatal(err)
	}
	clone := q.Clone()
	*clone.Tracks[0].Clips[0].Opacity = 0
	if q.Tracks[0].Clips[0].OpacityPercent() != 25 {
		t.Fatal("opacity aliased between snapshots")
	}
	split, err := q.Split("V1", "c", "right", 3000000)
	if err != nil {
		t.Fatal(err)
	}
	*split.Tracks[0].Clips[1].Opacity = 50
	if split.Tracks[0].Clips[0].OpacityPercent() != 25 {
		t.Fatal("split opacity aliased")
	}
	b, err := q.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	p.Version = ControlsVersion
	pasted, err := p.PasteClips("V1", b, 9000000, []string{"paste"})
	if err != nil || pasted.Project.Version != Version || pasted.Project.Tracks[0].Clips[1].OpacityPercent() != 25 {
		t.Fatalf("clipboard opacity: %v", err)
	}
	q, err = q.ToggleTrackControl("V1", Lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.SetClipOpacity(ref, 50); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("locked opacity: %v", err)
	}
	if _, err = q.ApplyLayerPreset(ref, media.LayerFit); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("locked placement: %v", err)
	}
	if _, err = p.SetClipOpacity(ref, 101); err == nil {
		t.Fatal("invalid opacity accepted")
	}
	if _, err = p.SetCompositionCanvas(media.CanvasSize{Width: 1080}); err == nil {
		t.Fatal("partial canvas accepted")
	}
	audio, err := p.DetachAudio(ref, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = audio.SetClipOpacity(ClipRef{TrackID: "A1", ClipID: "sound"}, 50); err == nil {
		t.Fatal("audio opacity accepted")
	}
}
