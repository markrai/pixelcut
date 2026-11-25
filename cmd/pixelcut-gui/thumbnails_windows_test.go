package main

import (
	"testing"
	"time"
)

func TestThumbnailDemandDebounce(t *testing.T) {
	c := &thumbnailCache{entries: make(map[thumbnailKey]thumbnailEntry), wake: make(chan struct{}, 1)}
	a := thumbnailKey{path: "a", asset: "a", tick: 100, width: 48}
	b := thumbnailKey{path: "a", asset: "a", tick: 200, width: 48}
	c.request([]thumbnailKey{a, b})
	first := c.settled
	c.request([]thumbnailKey{a, b})
	if c.settled != first {
		t.Fatal("identical repaint delayed extraction")
	}
	if _, ok, wait := c.next(); ok || wait <= 0 {
		t.Fatal("unstable demand dispatched")
	}
	c.settled = time.Now().Add(-time.Second)
	if k, ok, _ := c.next(); !ok || k != a {
		t.Fatal("first missing key not scheduled")
	}
	c.store(a, []byte{1})
	if k, ok, _ := c.next(); !ok || k != b {
		t.Fatal("cached key decoded again")
	}
	c.request(nil)
	if _, ok, wait := c.next(); ok || wait != 0 {
		t.Fatal("drag did not suppress queued extraction")
	}
}

func TestThumbnailFallback(t *testing.T) {
	c := &thumbnailCache{entries: make(map[thumbnailKey]thumbnailEntry)}
	key := thumbnailKey{path: "a", asset: "a", tick: 100, width: 48}
	for _, v := range []struct {
		k thumbnailKey
		p byte
	}{
		{thumbnailKey{path: "a", asset: "a", tick: 80, width: 48}, 1},
		{thumbnailKey{path: "a", asset: "a", tick: 110, width: 48}, 2},
		{thumbnailKey{path: "b", asset: "a", tick: 100, width: 48}, 3},
		{thumbnailKey{path: "a", asset: "a", tick: 100, width: 24}, 4},
	} {
		c.store(v.k, []byte{v.p})
	}
	if p := c.fallback(key, 90, 120); len(p) != 1 || p[0] != 2 {
		t.Fatal("nearest eligible frame not reused")
	}
	if p := c.fallback(key, 95, 105); p != nil {
		t.Fatal("wrong asset, width, or time reused")
	}
	if p := c.fallback(key, 70, 90); len(p) != 1 || p[0] != 1 {
		t.Fatal("trim range ignored")
	}
}
