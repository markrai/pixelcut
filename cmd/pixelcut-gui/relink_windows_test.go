package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

func TestRelinkPlanAtomicity(t *testing.T) {
	p := project.New()
	dir := t.TempDir()
	plan := &relinkPlan{}
	for _, id := range []string{"a", "b"} {
		old := project.Asset{ID: id, Path: filepath.Join(dir, id+"-old.mp4"), Duration: 1000000, Width: 1080, Height: 1920, Range: "tv"}
		p.Assets = append(p.Assets, old)
		a := old
		a.Path = filepath.Join(dir, id+"-new.mp4")
		if err := os.WriteFile(a.Path, []byte(id), 0600); err != nil {
			t.Fatal(err)
		}
		stamp, err := mediacache.Identify(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		plan.changes = append(plan.changes, verifiedRelink{asset: a, original: old, stamp: stamp})
	}
	before := p.Clone()
	q, err := applyRelinkPlan(p, plan)
	if err != nil || q.Assets[0].Path != plan.changes[0].asset.Path || q.Assets[1].Path != plan.changes[1].asset.Path {
		t.Fatalf("plan: %v", err)
	}
	if !reflect.DeepEqual(p, before) {
		t.Fatal("source project mutated")
	}
	if err = os.WriteFile(plan.changes[1].asset.Path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	q, err = applyRelinkPlan(p, plan)
	if err == nil || !reflect.DeepEqual(q, before) {
		t.Fatal("partial relink after source change")
	}
	if err = os.WriteFile(plan.changes[0].original.Path, []byte("restored"), 0600); err != nil {
		t.Fatal(err)
	}
	q, err = applyRelinkPlan(p, &relinkPlan{changes: plan.changes[:1]})
	if err == nil || !reflect.DeepEqual(q, before) {
		t.Fatal("reappearing original overwritten")
	}
}
