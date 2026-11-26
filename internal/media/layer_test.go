package media

import "testing"

func TestPlanLayerSharedCanvas(t *testing.T) {
	canvas := CanvasSize{Width: 1920, Height: 1080}
	g, err := PlanLayer(canvas, 1080, 1920, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := g.Bounds
	if b.DestX != 656 || b.DestY != 0 || b.Width != 608 || b.Height != 1080 {
		t.Fatalf("portrait fit: %+v", b)
	}
	tr := &Transform{CanvasWidth: 1000, CanvasHeight: 1000, Scale: 50}
	g, err = PlanLayer(canvas, 1000, 1000, nil, tr)
	if err != nil {
		t.Fatal(err)
	}
	b = g.Bounds
	if b.DestX != 690 || b.DestY != 270 || b.Width != 540 || b.Height != 540 {
		t.Fatalf("nested canvas did not preserve transparent margins: %+v", b)
	}
	tr = &Transform{CanvasWidth: 1080, CanvasHeight: 1920, Scale: 200}
	g, err = PlanLayer(CanvasSize{1080, 1920}, 1080, 1920, nil, tr)
	if err != nil || g.Content.SourceX != 540 || g.Content.SourceY != 960 || g.Bounds.Width != 1080 || g.Bounds.Height != 1920 {
		t.Fatalf("zoom clipping: %+v %v", g, err)
	}
}

func TestLayerPresetCorners(t *testing.T) {
	for _, canvas := range []CanvasSize{{1920, 1080}, {1080, 1920}, {1080, 1080}, {3840, 2160}} {
		for _, preset := range []LayerPreset{LayerTopLeft, LayerTopRight, LayerBottomLeft, LayerBottomRight} {
			prev := &Transform{Rotation: 90, FlipHorizontal: true}
			tr, err := PresetTransform(canvas, 1080, 1920, prev, preset)
			if err != nil {
				t.Fatal(err)
			}
			if tr.Scale != 35 || tr.Rotation != 90 || !tr.FlipHorizontal {
				t.Fatal("preset lost scale or orientation")
			}
			p, err := tr.Placement(1080, 1920)
			if err != nil {
				t.Fatal(err)
			}
			margin := evenRound(float64(min(canvas.Width, canvas.Height)) * 0.04)
			left, top := margin, margin
			if preset == LayerTopRight || preset == LayerBottomRight {
				left = canvas.Width - p.Width - margin
			}
			if preset == LayerBottomLeft || preset == LayerBottomRight {
				top = canvas.Height - p.Height - margin
			}
			if p.DestX != left || p.DestY != top || p.SourceX != 0 || p.SourceY != 0 {
				t.Fatalf("corner %s: %+v", preset, p)
			}
		}
	}
	if _, err := PresetTransform(CanvasSize{1920, 1080}, 1080, 1920, nil, LayerPreset("unknown")); err == nil {
		t.Fatal("unknown preset accepted")
	}
	for _, bad := range []CanvasSize{{0, 1080}, {1919, 1080}, {8192, 8192}} {
		if _, err := PlanLayer(bad, 1080, 1920, nil, nil); err == nil {
			t.Fatalf("invalid canvas accepted: %+v", bad)
		}
	}
}
