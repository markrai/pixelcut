package project

import (
	"reflect"
	"testing"
)

func TestRemoveAsset(t *testing.T) {
	p := fixture(t)
	a := p.Assets[0]
	b := a
	b.ID = "b"
	p.Assets = append(p.Assets, b)
	p.Tracks[0].Clips = append(p.Tracks[0].Clips, Clip{ID: "other", AssetID: "b", Start: 9000000, In: 0, Out: 1000000})
	before := p.Clone()
	if _, err := p.RemoveAsset(a.ID, false); err == nil {
		t.Fatal("referenced removal allowed without consent")
	}
	q, err := p.RemoveAsset(a.ID, true)
	if err != nil || len(q.Assets) != 1 || q.Assets[0].ID != "b" || len(q.Tracks[0].Clips) != 1 || q.Tracks[0].Clips[0].ID != "other" {
		t.Fatalf("unexpected removal: %+v %v", q, err)
	}
	if !reflect.DeepEqual(p, before) {
		t.Fatal("original project mutated")
	}
	q, err = p.RemoveAsset("missing", true)
	if err == nil || !reflect.DeepEqual(q, p) {
		t.Fatal("missing asset removal accepted")
	}
	p.Tracks[0].Clips = p.Tracks[0].Clips[:1]
	q, err = p.RemoveAsset("b", false)
	if err != nil || len(q.Assets) != 1 || len(q.Tracks[0].Clips) != 1 {
		t.Fatal("unused asset removal failed")
	}
}

func TestSnapDrag(t *testing.T) {
	p := fixture(t)
	p.Tracks[0].Clips = append(p.Tracks[0].Clips, Clip{ID: "second", AssetID: "a", Start: 8000000, In: 0, Out: 1000000})
	for _, tc := range []struct {
		mode                                         string
		delta, marker, tolerance, target, start, out int64
		snapped                                      bool
	}{
		{"move", 1900000, -1, 200000, 8000000, 3000000, 7000000, true},
		{"move", 2100000, -1, 200000, 8000000, 3000000, 7000000, true},
		{"right", 1900000, -1, 200000, 8000000, 1000000, 9000000, true},
		{"left", 900000, 2000000, 200000, 2000000, 2000000, 7000000, true},
		{"move", -900000, -1, 200000, 0, 0, 7000000, true},
		{"move", 1900000, -1, 0, 0, 2900000, 7000000, false},
		{"right", 400000, 6500000, 50000, 0, 1000000, 7400000, false},
	} {
		r, err := p.SnapDrag("V1", "c", tc.mode, tc.delta, tc.marker, tc.tolerance)
		if err != nil || r.Snapped != tc.snapped || r.Target != tc.target || r.Clip.Start != tc.start || r.Clip.Out != tc.out {
			t.Fatalf("%+v: %+v %v", tc, r, err)
		}
	}
	if r, err := p.SnapDrag("V1", "c", "right", 2900000, 8900000, 200000); err == nil || r.Snapped {
		t.Fatal("invalid overlapping snap accepted")
	}
	if p.Tracks[0].Clips[0].Start != 1000000 {
		t.Fatal("snap mutated source")
	}
}
