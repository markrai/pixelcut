package timeline

import (
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestStackGeometryAndReveal(t *testing.T) {
	p := dragFixture(t)
	var err error
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err = p.AddTrack(project.AudioTrack)
	if err != nil {
		t.Fatal(err)
	}
	s := LayoutStack(p, 250, 0)
	want := []string{"V2", "V1", "A1", "A2"}
	for i, id := range want {
		l := s.Lanes[i]
		if l.TrackID != id {
			t.Fatalf("lane %d: %s", i, l.TrackID)
		}
		if i > 0 && s.Lanes[i-1].Bottom != l.Top {
			t.Fatal("lane gap or overlap")
		}
	}
	for _, tc := range []struct {
		y  int32
		id string
	}{
		{TrackTop - 1, ""},
		{TrackTop, "V2"},
		{ContentBottom - 1, "V2"},
		{ContentBottom, "V1"},
		{s.Bottom, ""},
	} {
		l, ok := s.At(tc.y)
		if ok != (tc.id != "") || l.TrackID != tc.id {
			t.Fatalf("hit %d: %+v %t", tc.y, l, ok)
		}
	}
	s = LayoutStack(p, 250, s.Reveal("A2"))
	l, ok := s.Find("A2")
	if !ok || s.Scroll != s.MaxScroll || l.Bottom != s.Bottom || l.Top < TrackTop {
		t.Fatalf("reveal last track: %+v %+v", s, l)
	}
	if LayoutStack(p, 250, 100000).Scroll != s.MaxScroll || LayoutStack(p, 250, -100).Scroll != 0 {
		t.Fatal("scroll not clamped")
	}
	if s.Reveal("missing") != s.Scroll {
		t.Fatal("missing track changed scroll")
	}
}

func TestStackClipHitUsesLaneOffset(t *testing.T) {
	p := dragFixture(t)
	p, _, err := p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	s := LayoutStack(p, 420, 0)
	l, ok := s.Find("V1")
	if !ok {
		t.Fatal("missing V1")
	}
	v := View{Width: 800, Zoom: 100}
	g, visible := v.Clip(p.Tracks[0].Clips[0], true)
	if !visible {
		t.Fatal("clip not visible")
	}
	g.OffsetY = l.Offset()
	x := g.Left + 20
	if g.MainHit(x, LaneTop+l.Offset()) != "move" || g.Hit(x, LaneTop) {
		t.Fatal("main lane hit does not use offset")
	}
	if !g.AudioHit(x, AudioTop+l.Offset()) || g.AudioHit(x, AudioTop) {
		t.Fatal("linked audio hit does not use offset")
	}
	if !g.Intersects(x, LaneTop+l.Offset(), x+1, LaneTop+l.Offset()+1) || g.Intersects(x, LaneTop, x+1, LaneTop+1) {
		t.Fatal("marquee does not use offset")
	}
	if l.MainContains(AudioTop+l.Offset()) {
		t.Fatal("linked audio row accepted as an independent drop target")
	}
}

func TestCrossTrackGesture(t *testing.T) {
	p := dragFixture(t)
	p, _, err := p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	ti := p.TrackIndex("V2")
	p.Tracks[ti].Clips = []project.Clip{{ID: "target", AssetID: "a", Start: 3000000, Out: 1000000}}
	before := p.Fingerprint()
	req := DragRequest{
		Ref: project.ClipRef{TrackID: "V1", ClipID: "first"}, TargetTrack: "V2",
		Mode: "move", GroupIDs: []string{"first", "second"}, Scale: 100,
		Pixels: 102, Marker: -1, Magnetic: true,
	}
	r := EvaluateDrag(p, req)
	if !r.Valid || !r.Snapped || r.Target != 3000000 || r.Clip.Start != 2000000 {
		t.Fatalf("destination snap: %+v", r)
	}
	if ref, ok := r.Project.LocateClip("second"); !ok || ref.TrackID != "V2" {
		t.Fatal("group transfer lost second clip")
	}
	second, _ := r.Project.Clip(project.ClipRef{TrackID: "V2", ClipID: "second"})
	if second.Start != 5000000 || len(r.Project.Tracks[0].Clips) != 1 {
		t.Fatal("group spacing or source track changed incorrectly")
	}
	if err := r.Project.Validate(); err != nil {
		t.Fatal(err)
	}
	req.Bypass = true
	if EvaluateDrag(p, req).Valid {
		t.Fatal("unsnapped destination overlap accepted")
	}
	req.TargetTrack = "A1"
	if EvaluateDrag(p, req).Valid {
		t.Fatal("cross-kind transfer accepted")
	}
	req.TargetTrack, req.Mode = "V2", "right"
	if EvaluateDrag(p, req).Valid {
		t.Fatal("cross-track trim accepted")
	}
	req.Mode, req.GroupIDs, req.Pixels = "move", nil, 0
	r = EvaluateDrag(p, req)
	if !r.Valid || r.Clip.Start != 1000000 || len(r.Project.Tracks[0].Clips) != 2 {
		t.Fatal("vertical-only transfer failed")
	}
	if p.Fingerprint() != before {
		t.Fatal("gesture mutated committed project")
	}
}
