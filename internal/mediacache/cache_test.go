package mediacache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentCache(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(source, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Identify(source)
	if err != nil {
		t.Fatal(err)
	}
	k := MakeKey(s, "thumbnail", "ffmpeg", "100:32")
	c, err := Open(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Put(k, []byte("pixels")); err != nil {
		t.Fatal(err)
	}
	if err = c.SetLimit(256 * MiB); err != nil {
		t.Fatal(err)
	}
	c, err = Open(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := c.Get(k); !ok || string(b) != "pixels" {
		t.Fatal("restart miss")
	}
	if c.Stats().Limit != 256*MiB {
		t.Fatal("limit not restored")
	}
	for _, other := range []Key{MakeKey(s, "waveform", "ffmpeg", "100:32"), MakeKey(s, "thumbnail", "new ffmpeg", "100:32"), MakeKey(s, "thumbnail", "ffmpeg", "200:32")} {
		if _, ok := c.Get(other); ok {
			t.Fatal("key collision")
		}
	}
	if err = os.WriteFile(source, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Identify(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(MakeKey(s, "thumbnail", "ffmpeg", "100:32")); ok {
		t.Fatal("stale hit")
	}
	if err = os.WriteFile(filepath.Join(c.dir, k.name()), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(k); ok {
		t.Fatal("corruption accepted")
	}
	if err = c.Put(k, []byte("rebuilt")); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(c.dir, "unrelated.txt")
	if err = os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(keep); err != nil {
		t.Fatal("unrelated file removed")
	}
	if _, err = os.Stat(source); err != nil {
		t.Fatal("source removed")
	}
	if c.Stats().Entries != 0 {
		t.Fatal("clear failed")
	}
}

func TestEvictionAndIntegrity(t *testing.T) {
	c, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.limit = 200
	a, b, d := Key{1}, Key{2}, Key{3}
	data := bytes.Repeat([]byte{7}, 30)
	if err = c.Put(a, data); err != nil {
		t.Fatal(err)
	}
	if err = c.Put(b, data); err != nil {
		t.Fatal(err)
	}
	c.entries[a.name()] = entry{94, time.Now().Add(-time.Hour)}
	if err = c.Put(d, data); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(a); ok {
		t.Fatal("oldest not evicted")
	}
	if c.Stats().Bytes > 200 {
		t.Fatal("budget exceeded")
	}
	path := filepath.Join(c.dir, d.name())
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(d); ok {
		t.Fatal("checksum failure accepted")
	}
}
