package media

import "testing"

func TestCanvasPlacement(t *testing.T) {
	for _, tc := range []struct {
		transform Transform
		want      Placement
	}{
		{Transform{CanvasWidth: 320, CanvasHeight: 240, Scale: 50}, Placement{ScaledWidth: 160, ScaledHeight: 120, Width: 160, Height: 120, DestX: 80, DestY: 60}},
		{Transform{CanvasWidth: 320, CanvasHeight: 240, Scale: 50, X: 100, Y: -80}, Placement{ScaledWidth: 160, ScaledHeight: 120, SourceY: 20, Width: 140, Height: 100, DestX: 180}},
		{Transform{CanvasWidth: 320, CanvasHeight: 240, Scale: 200}, Placement{ScaledWidth: 640, ScaledHeight: 480, SourceX: 160, SourceY: 120, Width: 320, Height: 240}},
		{Transform{CanvasWidth: 240, CanvasHeight: 320, Scale: 100}, Placement{ScaledWidth: 240, ScaledHeight: 180, Width: 240, Height: 180, DestY: 70}},
	} {
		got, err := tc.transform.Placement(320, 240)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %+v, want %+v, err %v", tc.transform, got, tc.want, err)
		}
	}
	for _, tr := range []Transform{
		{CanvasWidth: 319, CanvasHeight: 240, Scale: 100},
		{CanvasWidth: 320, CanvasHeight: 240, Scale: 0},
		{CanvasWidth: 320, CanvasHeight: 240, Scale: 100, X: 1},
		{CanvasWidth: 320, CanvasHeight: 240, Scale: 100, X: 1000},
		{CanvasWidth: 8192, CanvasHeight: 8192, Scale: 400},
	} {
		if _, err := tr.Placement(320, 240); err == nil {
			t.Fatalf("accepted invalid transform %+v", tr)
		}
	}
}

func TestTransformGraph(t *testing.T) {
	crop := &Crop{20, 20, 200, 160}
	tr := &Transform{CanvasWidth: 320, CanvasHeight: 240, Scale: 50, X: 10, Y: -20}
	graph, w, h, err := VideoFilters(320, 240, crop, tr)
	want := "crop=200:160:20:20,scale=150:120:flags=lanczos,pad=320:240:96:40:color=black,setsar=1"
	if err != nil || graph != want || w != 320 || h != 240 {
		t.Fatalf("%q %dx%d %v", graph, w, h, err)
	}
	for _, tc := range []struct {
		r      int
		prefix string
	}{{0, ""}, {90, "transpose=clock,"}, {180, "hflip,vflip,"}, {270, "transpose=cclock,"}} {
		got, err := PreviewFilters(graph, tc.r)
		if err != nil || got != "lavfi=["+tc.prefix+want+"],format=rotate=0" {
			t.Fatalf("rotation %d: %q %v", tc.r, got, err)
		}
	}
	if _, err := PreviewFilters(graph, 45); err == nil {
		t.Fatal("accepted arbitrary rotation")
	}
}

func TestOrientationGraph(t *testing.T) {
	for _, tc := range []struct {
		rotation  int
		h, v      bool
		want      string
		w, height int
	}{
		{0, true, false, "hflip,setsar=1", 320, 240},
		{0, false, true, "vflip,setsar=1", 320, 240},
		{90, false, false, "transpose=clock,setsar=1", 240, 320},
		{180, false, false, "hflip,vflip,setsar=1", 320, 240},
		{270, false, false, "transpose=cclock,setsar=1", 240, 320},
		{90, true, true, "transpose=clock,hflip,vflip,setsar=1", 240, 320},
	} {
		tr := &Transform{CanvasWidth: tc.w, CanvasHeight: tc.height, Scale: 100, Rotation: tc.rotation, FlipHorizontal: tc.h, FlipVertical: tc.v}
		graph, w, h, err := VideoFilters(320, 240, nil, tr)
		if err != nil || graph != tc.want || w != tc.w || h != tc.height {
			t.Fatalf("%+v: %q %dx%d %v", tr, graph, w, h, err)
		}
	}
	tr := &Transform{CanvasWidth: 160, CanvasHeight: 200, Scale: 100, Rotation: 90, FlipHorizontal: true}
	graph, w, h, err := VideoFilters(320, 240, &Crop{20, 20, 200, 160}, tr)
	if err != nil || graph != "crop=200:160:20:20,transpose=clock,hflip,setsar=1" || w != 160 || h != 200 {
		t.Fatalf("unexpected combined orientation: %q %dx%d %v", graph, w, h, err)
	}
	tr.Rotation = 45
	if _, err := tr.Placement(200, 160); err == nil {
		t.Fatal("accepted non-cardinal rotation")
	}
}
