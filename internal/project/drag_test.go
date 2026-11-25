package project

import "testing"

func TestDragClip(t *testing.T) {
	p := fixture(t)
	for _, tc := range []struct {
		mode                  string
		delta, start, in, out int64
	}{
		{"move", 2000000, 3000000, 2000000, 7000000},
		{"move", -2000000, 0, 2000000, 7000000},
		{"left", 1000000, 2000000, 3000000, 7000000},
		{"left", -3000000, 0, 1000000, 7000000},
		{"right", 9000000, 1000000, 2000000, 10000000},
		{"right", -9000000, 1000000, 2000000, 2000001},
	} {
		c, err := p.DragClip("V1", "c", tc.mode, tc.delta)
		if err != nil || c.Start != tc.start || c.In != tc.in || c.Out != tc.out {
			t.Fatalf("%s: %+v %v", tc.mode, c, err)
		}
	}
	if p.Tracks[0].Clips[0].Start != 1000000 || p.Tracks[0].Clips[0].In != 2000000 {
		t.Fatal("drag mutated source project")
	}
	p.Tracks[0].Clips = append(p.Tracks[0].Clips, Clip{ID: "second", AssetID: "a", Start: 6000000, In: 0, Out: 2000000})
	if _, err := p.DragClip("V1", "c", "move", 1000000); err == nil {
		t.Fatal("overlapping move accepted")
	}
	if _, err := p.DragClip("V1", "c", "right", 1000000); err == nil {
		t.Fatal("overlapping trim accepted")
	}
}
