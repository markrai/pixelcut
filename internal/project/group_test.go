package project

import (
	"math"
	"reflect"
	"testing"
)

func groupFixture(t *testing.T) Project {
	p := fixture(t)
	p.Tracks[0].Clips = append(p.Tracks[0].Clips,
		Clip{ID: "b", AssetID: "a", Start: 8000000, In: 0, Out: 1000000},
		Clip{ID: "d", AssetID: "a", Start: 12000000, In: 0, Out: 1000000})
	return p
}

func TestGroupMove(t *testing.T) {
	p := groupFixture(t)
	before := p.Clone()
	ids := []string{"c", "b"}
	r, err := p.MoveClips("V1", ids, 2900000, -1, 200000)
	if err != nil || !r.Snapped || r.Target != 12000000 || r.Delta != 3000000 {
		t.Fatalf("snap: %+v %v", r, err)
	}
	for i := 0; i < 2; i++ {
		want := before.Tracks[0].Clips[i]
		want.Start += 3000000
		if !reflect.DeepEqual(r.Project.Tracks[0].Clips[i], want) {
			t.Fatal("group timing or edits changed")
		}
	}
	if !reflect.DeepEqual(r.Project.Tracks[0].Clips[2], before.Tracks[0].Clips[2]) {
		t.Fatal("unselected clip moved")
	}
	r, err = p.MoveClips("V1", ids, -2000000, -1, 0)
	if err != nil || r.Delta != -1000000 {
		t.Fatal("group clamping failed")
	}
	if r, err = p.MoveClips("V1", ids, 3500000, -1, 0); err == nil || r.Project.Tracks[0].Clips[1].Start != 11500000 {
		t.Fatal("overlap must produce rejected ghost")
	}
	if _, err = p.MoveClips("V1", []string{"c", "missing"}, 0, 0, 1); err == nil {
		t.Fatal("missing selection accepted")
	}
	if _, err = p.MoveClips("V1", ids, math.MaxInt64, 0, 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, before) {
		t.Fatal("source project mutated")
	}
}

func TestGroupDeleteAndRange(t *testing.T) {
	p := groupFixture(t)
	q, err := p.DeleteClips("V1", []string{"b", "c"})
	if err != nil || len(q.Tracks[0].Clips) != 1 || q.Tracks[0].Clips[0].Start != 12000000 || len(q.Assets) != 1 {
		t.Fatal("group delete rippled or removed assets")
	}
	if _, err = p.DeleteClips("V1", []string{"b", "b"}); err == nil {
		t.Fatal("duplicate selection accepted")
	}
	clips := p.Tracks[0].Clips
	unordered := []Clip{clips[2], clips[0], clips[1]}
	if got := RangeClipIDs(unordered, "d", "c"); !reflect.DeepEqual(got, []string{"c", "b", "d"}) {
		t.Fatalf("range %v", got)
	}
	if got := RangeClipIDs(unordered, "missing", "b"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("missing anchor %v", got)
	}
}
