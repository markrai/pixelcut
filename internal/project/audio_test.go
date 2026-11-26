package project

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetachReattachAudioRoundTrip(t *testing.T) {
	p := fixture(t)
	p.Version = TrackVersion
	before := p.Fingerprint()
	video := ClipRef{TrackID: "V1", ClipID: "c"}
	audio := ClipRef{TrackID: "A1", ClipID: "sound"}
	q, err := p.DetachAudio(video, audio.TrackID, audio.ClipID)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := q.Clip(video)
	a, _ := q.Clip(audio)
	if q.Version != Version || !v.AudioDetached || !audioAligned(v, a) || a.Crop != nil || a.Transform != nil || a.AudioDetached {
		t.Fatal("invalid detached pair")
	}
	if v.Crop == nil || !reflect.DeepEqual(v.Crop, p.Tracks[0].Clips[0].Crop) || q.Assets[0] != p.Assets[0] {
		t.Fatal("video or original asset changed")
	}
	_, ed, err := q.MediaEdit(v)
	if err != nil || !ed.Silent {
		t.Fatal("detached video exports source audio")
	}
	if got := q.MatchingAudio(video); !reflect.DeepEqual(got, []ClipRef{audio}) {
		t.Fatalf("matching audio: %+v", got)
	}
	path := filepath.Join(t.TempDir(), "detached.pixelcut.json")
	if err = q.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || !reflect.DeepEqual(loaded, q) {
		t.Fatalf("detached roundtrip: %v", err)
	}
	r, err := loaded.ReattachAudio(video, audio)
	if err != nil || len(r.Tracks[1].Clips) != 0 || r.Tracks[0].Clips[0].AudioDetached {
		t.Fatalf("reattach failed: %v", err)
	}
	r.Version = TrackVersion
	if r.Fingerprint() != before || p.Fingerprint() != before {
		t.Fatal("reattach changed original clip data")
	}
}

func TestDetachAndReattachRejectInvalidOperations(t *testing.T) {
	p := fixture(t)
	v := ClipRef{TrackID: "V1", ClipID: "c"}
	for _, tc := range []struct {
		track, id string
	}{{"V1", "sound"}, {"missing", "sound"}, {"A1", "c"}, {"A1", ""}} {
		before := p.Fingerprint()
		q, err := p.DetachAudio(v, tc.track, tc.id)
		if err == nil || q.Fingerprint() != before || p.Fingerprint() != before {
			t.Fatalf("invalid detach changed project: %+v", tc)
		}
	}
	q, err := p.DetachAudio(v, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.DetachAudio(v, "A1", "again"); err == nil {
		t.Fatal("repeated detach accepted")
	}
	r, _, err := q.AddTrack(AudioTrack)
	if err != nil {
		t.Fatal(err)
	}
	other := r.Clone()
	other.Tracks[0].Clips[0].AudioDetached = false
	if _, err := other.DetachAudio(v, "A1", "overlap"); err == nil {
		t.Fatal("overlapping destination accepted")
	}
	moved, err := r.MoveClipsTo("A1", "A2", []string{"sound"}, 1000000, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := moved.Project.Fingerprint()
	a := ClipRef{TrackID: "A2", ClipID: "sound"}
	rejected, err := moved.Project.ReattachAudio(v, a)
	if err == nil || rejected.Fingerprint() != before || len(moved.Project.MatchingAudio(v)) != 0 {
		t.Fatal("misaligned reattach accepted")
	}
	bad := q.Clone()
	bad.Version = TrackVersion
	if bad.Validate() == nil {
		t.Fatal("old schema accepted detached audio")
	}
	bad = q.Clone()
	bad.Tracks[1].Clips[0].AudioDetached = true
	if bad.Validate() == nil {
		t.Fatal("detached flag accepted on independent audio")
	}
	silent := p.Clone()
	silent.Assets[0].HasAudio = false
	if _, err := silent.DetachAudio(v, "A1", "sound"); err == nil {
		t.Fatal("silent source detached")
	}
}

func TestDetachedVideoSplitAndClipboard(t *testing.T) {
	p := fixture(t)
	q, err := p.DetachAudio(ClipRef{TrackID: "V1", ClipID: "c"}, "A1", "sound")
	if err != nil {
		t.Fatal(err)
	}
	r, err := q.Split("V1", "c", "right", 3000000)
	if err != nil || !r.Tracks[0].Clips[0].AudioDetached || !r.Tracks[0].Clips[1].AudioDetached {
		t.Fatal("split restored unwanted audio")
	}
	b, err := q.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	p.Version = TrackVersion
	pasted, err := p.PasteClips("V1", b, 10000000, []string{"copy"})
	if err != nil || pasted.Project.Version != Version || !pasted.Project.Tracks[0].Clips[1].AudioDetached {
		t.Fatal("clipboard restored unwanted audio or downgraded schema")
	}
}
