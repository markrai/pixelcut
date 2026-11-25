package project

import (
	"math"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestClipboardPreservesEditsAndSpacing(t *testing.T) {
	p := groupFixture(t)
	p.Tracks[0].Clips[0].Transform = &media.Transform{CanvasWidth: 1000, CanvasHeight: 1840, Scale: 100, Rotation: 180, FlipHorizontal: true}
	before := p.Clone()
	b, err := p.CopyClips("V1", []string{"d", "c"})
	if err != nil || b.Count() != 2 || b.Span() != 12000000 || b.OriginalEnd() != 13000000 {
		t.Fatalf("copy: %+v %v", b, err)
	}
	r, err := p.PasteClips("V1", b, 15000000, []string{"copy1", "copy2"})
	if err != nil {
		t.Fatal(err)
	}
	if r.FirstClip != 3 || !reflect.DeepEqual(r.IDs, []string{"copy1", "copy2"}) {
		t.Fatal("invalid paste selection")
	}
	for j, i := range []int{0, 2} {
		want := before.Tracks[0].Clips[i]
		want.ID = r.IDs[j]
		want.Start += 14000000
		if !reflect.DeepEqual(r.Project.Tracks[0].Clips[3+j], want) {
			t.Fatalf("paste lost edit: %+v", r.Project.Tracks[0].Clips[3+j])
		}
	}
	r.Project.Tracks[0].Clips[3].Crop.X = 2
	r.Project.Tracks[0].Clips[3].Transform.Rotation = 0
	p.Tracks[0].Clips[0].Crop.X = 20
	again, err := p.PasteClips("V1", b, 15000000, []string{"again1", "again2"})
	if err != nil || again.Project.Tracks[0].Clips[3].Crop.X != before.Tracks[0].Clips[0].Crop.X || again.Project.Tracks[0].Clips[3].Transform.Rotation != 180 {
		t.Fatal("clipboard aliased source or previous paste")
	}
	p.Tracks[0].Clips[0].Crop.X = before.Tracks[0].Clips[0].Crop.X
	if !reflect.DeepEqual(p, before) {
		t.Fatal("source mutated")
	}
}

func TestClipboardRejectsInvalidPlacement(t *testing.T) {
	p := groupFixture(t)
	before := p.Clone()
	b, err := p.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at  int64
		ids []string
	}{
		{1000000, []string{"new"}},
		{15000000, []string{"c"}},
		{15000000, nil},
		{-1, []string{"new"}},
		{math.MaxInt64, []string{"new"}},
	} {
		r, err := p.PasteClips("V1", b, tc.at, tc.ids)
		if err == nil || !reflect.DeepEqual(r.Project, before) {
			t.Fatalf("invalid paste accepted: %+v", tc)
		}
	}
	if _, err = p.PasteClips("V1", ClipClipboard{}, 0, nil); err == nil {
		t.Fatal("empty clipboard accepted")
	}
	if _, err = p.CopyClips("V1", []string{"missing"}); err == nil {
		t.Fatal("unknown clip copied")
	}
	q, err := p.RemoveAsset("a", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.PasteClips("V1", b, 0, []string{"new"}); err == nil {
		t.Fatal("removed asset restored implicitly")
	}
	q = p.Clone()
	q.Assets[0].Duration++
	if _, err = q.PasteClips("V1", b, 15000000, []string{"new"}); err == nil {
		t.Fatal("changed asset accepted")
	}
}

func TestCutPasteAndDuplicate(t *testing.T) {
	p := groupFixture(t)
	b, err := p.CopyClips("V1", []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	q, err := p.DeleteClips("V1", []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := q.PasteClips("V1", b, 8000000, []string{"new"})
	if err != nil || r.Project.Tracks[0].Clips[2].Start != 8000000 || len(r.Project.Assets) != 1 {
		t.Fatal("cut/paste failed")
	}
	r, err = p.DuplicateClips("V1", b, []string{"duplicate"})
	if err != nil || r.Start != 9000000 {
		t.Fatalf("duplicate in gap: %v %d", err, r.Start)
	}
	b, err = p.CopyClips("V1", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = p.DuplicateClips("V1", b, []string{"duplicate"})
	if err != nil || r.Start != 13000000 {
		t.Fatalf("duplicate fallback: %v %d", err, r.Start)
	}
	if len(p.Tracks[0].Clips) != 3 {
		t.Fatal("duplicate mutated original")
	}
}
