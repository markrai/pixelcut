package project

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTrackControlsRoundTrip(t *testing.T) {
	p := fixture(t)
	p.Version = TrackVersion
	before := p.Fingerprint()
	q := p
	var err error
	for _, c := range []TrackControl{Visibility, Mute, Solo, Lock} {
		q, err = q.ToggleTrackControl("V1", c)
		if err != nil {
			t.Fatal(err)
		}
	}
	if q.Version != Version || !q.Tracks[0].Hidden || !q.Tracks[0].Muted || !q.Tracks[0].Solo || !q.Tracks[0].Locked {
		t.Fatal("track controls not persisted")
	}
	if q.TrackPolicies()["V1"] != (TrackPolicy{}) || q.TrackPolicies()["A1"] != (TrackPolicy{}) {
		t.Fatal("mute and visibility must take precedence over solo")
	}
	if p.Fingerprint() != before || p.Version != TrackVersion {
		t.Fatal("toggle mutated source project")
	}
	path := filepath.Join(t.TempDir(), "controls.pixelcut.json")
	if err = q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || !reflect.DeepEqual(loaded, q) {
		t.Fatalf("track controls roundtrip: %v", err)
	}
	legacy := q.Clone()
	legacy.Version = AudioVersion
	if legacy.Validate() == nil {
		t.Fatal("older schema accepted track controls")
	}
	if _, err = q.ToggleTrackControl("A1", Visibility); err == nil {
		t.Fatal("audio-only track accepted visibility")
	}
	if _, err = q.ToggleTrackControl("missing", Lock); err == nil {
		t.Fatal("unknown track accepted")
	}
	for _, v := range []int{LegacyVersion, TrackVersion, AudioVersion} {
		old := p.Clone()
		old.Version = v
		if err = old.Validate(); err != nil {
			t.Fatal(err)
		}
		if old.TrackPolicies()["V1"] != (TrackPolicy{Video: true, Audio: true}) {
			t.Fatal("legacy tracks do not default to enabled")
		}
	}
}

func TestLockedTrackOperations(t *testing.T) {
	p := fixture(t)
	p, _, err := p.AddTrack(VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.ToggleTrackControl("V1", Lock)
	if err != nil {
		t.Fatal(err)
	}
	before := p.Fingerprint()
	ids := []string{"c"}
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	if _, err = p.Split("V1", "c", "right", 3000000); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("split: %v", err)
	}
	if _, err = p.DragClip("V1", "c", "move", 1000000); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("drag: %v", err)
	}
	if _, err = p.MoveClipsTo("V1", "V2", ids, 0, -1, 0); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("move: %v", err)
	}
	if _, err = p.TransferClips("V1", "V2", ids); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("transfer: %v", err)
	}
	if _, err = p.DeleteClips("V1", ids); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("delete: %v", err)
	}
	if _, err = p.RippleDelete("V1", ids); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("ripple delete: %v", err)
	}
	if _, err = p.RippleTrim("V1", "c", "right", 1000000, -1, 0); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("ripple trim: %v", err)
	}
	if _, _, err = p.RemoveGap("V1", 0); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("gap: %v", err)
	}
	if _, err = p.PlaceAssets("V1", []string{"a"}, []string{"new"}, 20000000, -1, 0); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("place: %v", err)
	}
	if _, err = p.DetachAudio(ref, "A1", "sound"); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("detach: %v", err)
	}
	if _, err = p.RemoveAsset("a", true); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("remove source: %v", err)
	}
	if _, err = p.RelinkAsset("a", p.Assets[0], true); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("replace source: %v", err)
	}
	b, err := p.CopyClips("V1", ids)
	if err != nil {
		t.Fatal("copy should remain available on a locked track")
	}
	if _, err = p.PasteClips("V1", b, 20000000, []string{"copy"}); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("paste: %v", err)
	}
	placed, err := p.PasteClips("V2", b, 20000000, []string{"copy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = placed.Project.MoveClipsTo("V2", "V1", []string{"copy"}, 0, -1, 0); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("locked destination: %v", err)
	}
	if p.Fingerprint() != before {
		t.Fatal("rejected operations mutated the source")
	}
}

func TestLockedAudioDestinationAndReattach(t *testing.T) {
	p := fixture(t)
	ref := ClipRef{TrackID: "V1", ClipID: "c"}
	p, err := p.ToggleTrackControl("A1", Lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.DetachAudio(ref, "A1", "sound"); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("detach onto locked track: %v", err)
	}
	p, err = p.ToggleTrackControl("A1", Lock)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.DetachAudio(ref, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.ToggleTrackControl("A1", Lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.ReattachAudio(ref, ClipRef{TrackID: "A1", ClipID: "sound"}); !errors.Is(err, ErrTrackLocked) {
		t.Fatalf("reattach from locked track: %v", err)
	}
}
