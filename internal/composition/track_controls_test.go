package composition

import "testing"

func TestTrackPolicyCompositionAndAudition(t *testing.T) {
	p := fixture(t)
	p.Tracks[0].Hidden = true
	p.Tracks[0].Solo = true
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.At(20)
	if err != nil || len(f.Video) != 0 || len(f.Audio) != 1 || f.Audio[0].Ref.TrackID != "V1" || !f.Audio[0].Linked {
		t.Fatalf("hidden solo video should retain source audio: %+v %v", f, err)
	}
	ref, at, ok := e.AuditionTarget("V1", 0, true)
	if !ok || at != 10 || ref.ClipID != "early" {
		t.Fatal("hidden track audio gap start failed")
	}
	if _, _, ok := e.AuditionTarget("V1", 0, false); ok {
		t.Fatal("seek jumped gap")
	}
	if _, _, ok := e.AuditionTarget("A1", 20, true); ok {
		t.Fatal("non-solo audio auditioned")
	}
	p.Tracks[0].Muted = true
	p.Tracks[1].Solo = true
	e, err = Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err = e.At(20)
	if err != nil || len(f.Video) != 0 || len(f.Audio) != 1 || f.Audio[0].Ref.TrackID != "A1" {
		t.Fatalf("multiple solo and mute precedence: %+v %v", f, err)
	}
	if _, _, ok := e.AuditionTarget("V1", 20, true); ok {
		t.Fatal("fully suppressed clip auditioned")
	}
	p.Tracks[0].Hidden = false
	p.Tracks[0].Locked = true
	e, err = Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err = e.At(20)
	if err != nil || len(f.Video) != 1 || len(f.Audio) != 1 {
		t.Fatal("lock affected output or mute suppressed video")
	}
	if ref, _, ok := e.AuditionTarget("V1", 20, true); !ok || ref.ClipID != "early" {
		t.Fatal("muted video cannot be auditioned")
	}
	if e.Duration() != 55 {
		t.Fatal("track policy changed timeline duration")
	}
}

func TestAuditionSkipsDetachedHiddenVideo(t *testing.T) {
	p := fixture(t)
	p.Tracks[0].Hidden = true
	p.Tracks[0].Clips[1].AudioDetached = true
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := e.AuditionTarget("V1", 0, true); ok {
		t.Fatal("hidden detached or silent video auditioned")
	}
}
