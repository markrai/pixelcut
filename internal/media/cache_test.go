package media

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/markrai/pixelcut/internal/mediacache"
)

func TestCacheHitSkipsProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires Unix")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "probe")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 91\n"), 0700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "source.mp4")
	if err := os.WriteFile(src, []byte("not a video"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := mediacache.Identify(src)
	if err != nil {
		t.Fatal(err)
	}
	c, err := mediacache.Open(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{FFprobe: tool, FFmpeg: tool}
	id := mediacache.ToolIdentity(tool)
	if id == "" {
		t.Fatal("fixture tool identity unavailable")
	}
	key := mediacache.MakeKey(s, "metadata", id, importMetadataCacheContract)
	b, err := json.Marshal(Report{Streams: []Stream{sampleStream()}})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Put(key, b); err != nil {
		t.Fatal(err)
	}
	r, err := e.CachedImportReport(context.Background(), c, src)
	if err != nil || len(r.Streams) != 1 {
		t.Fatalf("cache hit invoked probe: %v", err)
	}
	if err = os.WriteFile(src, []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = e.CachedImportReport(context.Background(), c, src); err == nil {
		t.Fatal("changed source returned stale metadata")
	}
}
