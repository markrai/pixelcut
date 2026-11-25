package project

import "testing"

func TestPlaybackTarget(t *testing.T) {
	clips := []Clip{{Start: 800, In: 20, Out: 220}, {Start: 200, In: 10, Out: 310}, {Start: 1000, Out: 100}}
	for _, tc := range []struct {
		at     int64
		index  int
		target int64
		ok     bool
	}{
		{0, 1, 200, true}, {199, 1, 200, true}, {200, 1, 200, true}, {350, 1, 350, true}, {499, 1, 499, true},
		{500, 0, 800, true}, {650, 0, 800, true}, {800, 0, 800, true}, {999, 0, 999, true},
		{1000, 2, 1000, true}, {1099, 2, 1099, true}, {1100, -1, 1100, false}, {1200, -1, 1200, false},
	} {
		i, at, ok := PlaybackTarget(clips, tc.at)
		if i != tc.index || at != tc.target || ok != tc.ok {
			t.Fatalf("%+v: %d %d %t", tc, i, at, ok)
		}
	}
	if _, _, ok := PlaybackTarget(nil, 0); ok {
		t.Fatal("empty timeline playable")
	}
}
