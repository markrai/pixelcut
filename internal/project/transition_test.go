package project

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func transitionFixture(t *testing.T) Project {
	t.Helper()
	p := New()
	p.Version = EnvelopeVersion
	p.Assets = []Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Duration: 8000000, Width: 1920, Height: 1080, HasAudio: true, Range: "tv"}}
	p.Tracks[0].Clips = []Clip{
		{ID: "incoming", AssetID: "a", Start: 2000000, In: 4000000, Out: 6000000},
		{ID: "outgoing", AssetID: "a", In: 0, Out: 2000000},
	}
	return p
}

func TestVideoTransitionModel(t *testing.T) {
	p := transitionFixture(t)
	ref := ClipRef{TrackID: "V1", ClipID: "incoming"}
	tr := VideoTransition{Kind: CrossDissolve, FromClipID: "outgoing", Duration: 500000}
	before := p.Fingerprint()
	q, err := p.SetVideoTransition(ref, &tr)
	if err != nil || q.Version != Version || !q.HasVideoTransitions() || p.Fingerprint() != before || q.EndTime() != p.EndTime() {
		t.Fatalf("transition edit: %v", err)
	}
	tr.Duration = 1
	if q.Tracks[0].Clips[0].TransitionIn.Duration != 500000 {
		t.Fatal("transition aliases edit input")
	}
	clone := q.Clone()
	clone.Tracks[0].Clips[0].TransitionIn.Duration = 2
	if q.Tracks[0].Clips[0].TransitionIn.Duration != 500000 {
		t.Fatal("transition aliases clone")
	}
	clip, ok := q.Clip(ref)
	if !ok || clip.TransitionIn == nil {
		t.Fatal("transition missing")
	}
	clip.TransitionIn.Duration = 3
	if q.Tracks[0].Clips[0].TransitionIn.Duration != 500000 {
		t.Fatal("transition aliases clip accessor")
	}
	if _, _, err := q.MediaEdit(q.Tracks[0].Clips[1]); err == nil {
		t.Fatal("isolated export ignored outgoing transition")
	}
	same, err := q.SetVideoTransition(ref, q.Tracks[0].Clips[0].TransitionIn)
	if err != nil || same.Fingerprint() != q.Fingerprint() {
		t.Fatal("no-op edit changed project")
	}
	path := filepath.Join(t.TempDir(), "transition.pixelcut.json")
	if err := q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(q, got) || got.Fingerprint() != q.Fingerprint() {
		t.Fatalf("transition round trip: %v", err)
	}
	removed, err := q.SetVideoTransition(ref, nil)
	if err != nil || removed.HasVideoTransitions() || removed.Version != Version || removed.EndTime() != q.EndTime() {
		t.Fatalf("remove transition: %v", err)
	}
}

func TestVideoTransitionValidation(t *testing.T) {
	p := transitionFixture(t)
	ref := ClipRef{TrackID: "V1", ClipID: "incoming"}
	valid := VideoTransition{Kind: CrossDissolve, FromClipID: "outgoing", Duration: 500000}
	for _, bad := range []VideoTransition{
		{Kind: "wipe", FromClipID: "outgoing", Duration: 500000},
		{Kind: CrossDissolve, FromClipID: "missing", Duration: 500000},
		{Kind: CrossDissolve, FromClipID: "incoming", Duration: 500000},
		{Kind: CrossDissolve, FromClipID: "outgoing", Duration: 0},
		{Kind: CrossDissolve, FromClipID: "outgoing", Duration: -1},
		{Kind: CrossDissolve, FromClipID: "outgoing", Duration: 2000001},
		{Kind: CrossDissolve, FromClipID: "outgoing", Duration: MaxVideoTransitionDuration + 1},
	} {
		got, err := p.SetVideoTransition(ref, &bad)
		if !errors.Is(err, ErrInvalidTransition) || got.Fingerprint() != p.Fingerprint() {
			t.Fatalf("invalid transition accepted: %+v %v", bad, err)
		}
	}
	gap := p.Clone()
	gap.Tracks[0].Clips[0].Start++
	if _, err := gap.SetVideoTransition(ref, &valid); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("gap accepted")
	}
	short := p.Clone()
	short.Assets = append(short.Assets, Asset{ID: "short", Path: filepath.Join(t.TempDir(), "short.mp4"), Duration: 2100000, Width: 1920, Height: 1080, Range: "tv"})
	short.Tracks[0].Clips[1].AssetID = "short"
	if _, err := short.SetVideoTransition(ref, &valid); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("missing source handle accepted")
	}
	locked := p.Clone()
	locked.Tracks[0].Locked = true
	if _, err := locked.SetVideoTransition(ref, &valid); !errors.Is(err, ErrTrackLocked) {
		t.Fatal("locked track accepted edit")
	}
	q, err := p.SetVideoTransition(ref, &valid)
	if err != nil {
		t.Fatal(err)
	}
	old := q.Clone()
	old.Version = EnvelopeVersion
	if !errors.Is(old.Validate(), ErrUnsupportedVersion) {
		t.Fatal("legacy schema accepted transition")
	}
	p.Tracks[0].Clips[0].Envelope = &VolumeEnvelope{Points: []VolumeKeyframe{{SourceTime: 0, Percent: 100}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("version 9 envelope compatibility: %v", err)
	}
	path := filepath.Join(t.TempDir(), "v9.pixelcut.json")
	if err := p.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Version != EnvelopeVersion || got.Fingerprint() != p.Fingerprint() {
		t.Fatalf("version 9 project migrated on load: %v", err)
	}
}

func TestVideoTransitionEditIntegrity(t *testing.T) {
	p := transitionFixture(t)
	q, err := p.SetVideoTransition(ClipRef{TrackID: "V1", ClipID: "incoming"}, &VideoTransition{Kind: CrossDissolve, FromClipID: "outgoing", Duration: 500000})
	if err != nil {
		t.Fatal(err)
	}
	before := q.Fingerprint()
	if _, err := q.DeleteClips("V1", []string{"outgoing"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("orphaned incoming transition")
	}
	if _, err := q.Split("V1", "incoming", "split", 3000000); err == nil {
		t.Fatal("split ignored transition")
	}
	if _, err := q.DragClip("V1", "incoming", "move", 100000); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("move broke adjacent cut")
	}
	for _, id := range []string{"incoming", "outgoing"} {
		if _, err := q.CopyClips("V1", []string{id}); err == nil {
			t.Fatal("partial transition copy accepted")
		}
	}
	copied, err := q.CopyClips("V1", []string{"incoming", "outgoing"})
	if err != nil {
		t.Fatal(err)
	}
	pasted, err := q.PasteClips("V1", copied, q.EndTime(), []string{"out2", "in2"})
	if err != nil {
		t.Fatal(err)
	}
	in, ok := pasted.Project.Clip(ClipRef{TrackID: "V1", ClipID: "in2"})
	if !ok || in.TransitionIn == nil || in.TransitionIn.FromClipID != "out2" {
		t.Fatal("clipboard did not remap transition")
	}
	moved, err := q.MoveClips("V1", []string{"incoming", "outgoing"}, 1000000, -1, 0)
	if err != nil || moved.Project.EndTime() != q.EndTime()+1000000 {
		t.Fatalf("pair move: %v", err)
	}
	if q.Fingerprint() != before {
		t.Fatal("edit mutated original project")
	}
}
