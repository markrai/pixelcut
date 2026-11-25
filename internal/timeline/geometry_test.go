package timeline

import (
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestGeometryBoundaries(t *testing.T) {
	v := View{Width: 400, Origin: 1000000, End: 10000000, Zoom: 100}
	if v.XAt(2000000) != 192 || v.TimeAt(192) != 2000000 {
		t.Fatal("time/pixel mapping")
	}
	c := project.Clip{Start: 500000, Out: 2000000}
	g, ok := v.Clip(c, true)
	if !ok || g.Left != Header || g.Right != 242 || g.MainHit(Header, LaneTop) != "move" {
		t.Fatalf("clipped edge geometry: %+v", g)
	}
	if g.MainHit(240, LaneTop) != "right" || g.MainHit(242, LaneTop) != "" || g.Hit(Header-1, LaneTop) {
		t.Fatal("trim/hit boundaries")
	}
	if !g.AudioHit(120, AudioTop) || g.AudioHit(120, AudioBottom) || !g.Intersects(100, AudioTop, 130, AudioTop+1) {
		t.Fatal("linked audio geometry")
	}
	g, _ = v.Clip(c, false)
	if g.Hit(120, AudioTop) || g.Intersects(100, AudioTop, 130, AudioTop+1) {
		t.Fatal("phantom audio hit")
	}
	if !v.DropTarget(Header, TrackTop) || v.DropTarget(400, TrackTop) || v.DropTarget(Header, TrackBottom) {
		t.Fatal("drop boundaries")
	}
	if _, ok := v.Span(0, 1000000); ok {
		t.Fatal("offscreen span visible")
	}
	if _, ok := v.Clip(project.Clip{Start: math.MaxInt64, Out: 10}, true); ok {
		t.Fatal("overflowing clip visible")
	}
	tiny, ok := v.Clip(project.Clip{Start: 2000000, Out: 1}, false)
	if !ok || tiny.Right-tiny.Left != 1 || !tiny.Hit(tiny.Left, LaneTop) {
		t.Fatal("one-pixel clip painting and hit testing disagree")
	}
}

func TestViewAndSnap(t *testing.T) {
	v := View{Width: 510, End: 10000000}
	if v.Scale() <= 0 || v.TimeAt(Header-1) != 0 {
		t.Fatal("fit view")
	}
	v.Zoom = 100
	v.Origin = 5000000
	z, origin := v.ZoomAt(5000000, 1.5)
	if z != 150 || origin < 0 || origin >= 5000000 {
		t.Fatal("zoom center")
	}
	if v.Scroll(true) <= v.Origin || v.Scroll(false) >= v.Origin {
		t.Fatal("scroll direction")
	}
	if SnapTolerance(100, true) != 80000 || SnapTolerance(1, true) != 2000000 || SnapTolerance(100, false) != 0 {
		t.Fatal("snap policy")
	}
	if DragStarted(3, -3) || !DragStarted(0, -4) {
		t.Fatal("drag threshold")
	}
	if RulerStep(100) != 1000000 || RulerLabel(12300000, 100000) != "00:12.300" {
		t.Fatal("ruler")
	}
}

func dragFixture(t *testing.T) project.Project {
	t.Helper()
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(t.TempDir(), "a.mp4"), Width: 1080, Height: 1920, Duration: 10000000, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{
		{ID: "first", AssetID: "a", Start: 1000000, In: 1000000, Out: 2000000},
		{ID: "second", AssetID: "a", Start: 4000000, In: 1000000, Out: 2000000},
		{ID: "third", AssetID: "a", Start: 7000000, In: 1000000, Out: 2000000},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEvaluateDrag(t *testing.T) {
	p := dragFixture(t)
	before := p.Fingerprint()
	req := DragRequest{Ref: project.ClipRef{TrackID: "V1", ClipID: "first"}, Mode: "move", Scale: 100, Pixels: 193, Marker: 3000000, Magnetic: true}
	r := EvaluateDrag(p, req)
	if !r.Valid || !r.Snapped || r.Clip.Start != 3000000 {
		t.Fatalf("magnetic drag: %+v", r)
	}
	req.Bypass = true
	r = EvaluateDrag(p, req)
	if !r.Valid || r.Snapped || r.Clip.Start != 2930000 {
		t.Fatalf("bypassed drag: %+v", r)
	}
	req.Pixels = 250
	if EvaluateDrag(p, req).Valid {
		t.Fatal("overlapping drag accepted")
	}
	req.GroupIDs = []string{"first", "second"}
	req.Pixels = 100
	r = EvaluateDrag(p, req)
	if !r.Valid || r.Project.Tracks[0].Clips[0].Start != 2000000 || r.Project.Tracks[0].Clips[1].Start != 5000000 {
		t.Fatal("group move")
	}
	req.GroupIDs = nil
	req.Mode, req.Ripple, req.Pixels = "right", true, 50
	r = EvaluateDrag(p, req)
	if !r.Valid || r.RippleDelta != 500000 || r.Project.Tracks[0].Clips[1].Start != 4500000 {
		t.Fatal("ripple trim")
	}
	req.Ref.ClipID = "missing"
	if EvaluateDrag(p, req).Valid || p.Fingerprint() != before {
		t.Fatal("invalid gesture or input mutation")
	}
}

func TestFrameSnapPriority(t *testing.T) {
	p := dragFixture(t)
	data := make([]byte, 24)
	for i, tick := range []uint64{1000000, 2033333, 2100000} {
		binary.LittleEndian.PutUint64(data[i*8:], tick)
	}
	f, err := media.DecodeFrameIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	req := DragRequest{Ref: project.ClipRef{TrackID: "V1", ClipID: "first"}, Mode: "right", Scale: 100, Pixels: 4, Marker: 2040000, Magnetic: true, Frames: f, SourceDuration: 10000000}
	r := EvaluateDrag(p, req)
	if !r.Valid || !r.Snapped || r.Clip.Out != 2033333 {
		t.Fatal("frame snapping lost priority")
	}
	req.Bypass = true
	r = EvaluateDrag(p, req)
	if !r.Valid || r.Snapped || r.Clip.Out != 2040000 {
		t.Fatal("frame bypass")
	}
}

func TestEditBoundaryNavigation(t *testing.T) {
	clips := []project.Clip{{Start: 8000000, In: 1000000, Out: 3000000}, {Start: 2000000, In: 0, Out: 3000000}}
	for _, tc := range []struct {
		at, want    int64
		forward, ok bool
	}{
		{0, 2000000, true, true}, {2000000, 5000000, true, true}, {5000000, 8000000, true, true},
		{8000000, 10000000, true, true}, {10000000, 0, true, false},
		{10000000, 8000000, false, true}, {8000000, 5000000, false, true}, {2000000, 0, false, true}, {0, 0, false, false},
	} {
		v, ok := EditBoundary(clips, tc.at, tc.forward)
		if v != tc.want || ok != tc.ok {
			t.Fatalf("%+v: %d %t", tc, v, ok)
		}
	}
}

func TestPrecisionRuler(t *testing.T) {
	for _, tc := range []struct {
		scale float64
		step  int64
	}{{1000, 100000}, {100, 1000000}, {10, 10000000}, {1, 100000000}} {
		if got := RulerStep(tc.scale); got != tc.step {
			t.Fatalf("scale %f step %d", tc.scale, got)
		}
	}
	if got := RulerLabel(12300000, 100000); got != "00:12.300" {
		t.Fatal(got)
	}
}
