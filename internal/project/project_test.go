package project

import (
	"path/filepath"
	"reflect"
	"testing"
)

func fixture(t *testing.T) Project {
	t.Helper()
	p := New()
	p.Assets = append(p.Assets, Asset{ID: "a", Path: filepath.Join(t.TempDir(), "source.mp4"), Duration: 10000000, Width: 1080, Height: 1920, HasAudio: true})
	p.Tracks[0].Clips = append(p.Tracks[0].Clips, Clip{ID: "c", AssetID: "a", Start: 1000000, In: 2000000, Out: 7000000, Crop: &Crop{40, 40, 1000, 1840}})
	return p
}

func TestRoundTrip(t *testing.T) {
	p := fixture(t)
	path := filepath.Join(t.TempDir(), "test.pixelcut.json")
	if err := p.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	q, err := Load(path)
	if err != nil || !reflect.DeepEqual(p, q) {
		t.Fatalf("round trip: %+v %v", q, err)
	}
	if p.SaveAs(path) == nil {
		t.Fatal("overwrote existing project")
	}
}

func TestSplitAndEdit(t *testing.T) {
	p := fixture(t)
	q, err := p.Split("V1", "c", "right", 3000000)
	if err != nil {
		t.Fatal(err)
	}
	left, right := q.Tracks[0].Clips[0], q.Tracks[0].Clips[1]
	if left.Out != 4000000 || right.In != 4000000 || right.Start != 3000000 || p.Tracks[0].Clips[0].Out != 7000000 {
		t.Fatal("incorrect non-destructive split")
	}
	_, e, err := q.MediaEdit(right)
	if err != nil || !e.Trim || e.Start != 4 || e.Duration != 3 || e.Crop.Width != 1000 {
		t.Fatalf("unexpected edit %+v %v", e, err)
	}
	right.Crop.X = 42
	if left.Crop.X != 40 || p.Tracks[0].Clips[0].Crop.X != 40 {
		t.Fatal("crop aliased after split")
	}
}

func TestRejectInvalidProject(t *testing.T) {
	p := fixture(t)
	for _, mutate := range []func(*Project){
		func(q *Project) { q.Version++ },
		func(q *Project) { q.Tracks[0].Clips[0].Out = 11000000 },
		func(q *Project) { q.Tracks[0].Clips[0].Crop.X = 41 },
		func(q *Project) { q.Tracks[0].Clips[0].AssetID = "missing" },
		func(q *Project) { c := q.Tracks[0].Clips[0]; c.ID = "d"; q.Tracks[0].Clips = append(q.Tracks[0].Clips, c) },
	} {
		q := p.Clone()
		mutate(&q)
		if q.Validate() == nil {
			t.Fatal("invalid project accepted")
		}
	}
}
