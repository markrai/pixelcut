package project

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBatchImport(t *testing.T) {
	dir := t.TempDir()
	a, b, bad := filepath.Join(dir, "a.mp4"), filepath.Join(dir, "b.mp4"), filepath.Join(dir, "bad.mov")
	p := New()
	calls := 0
	inspect := func(_ context.Context, path string) (Asset, error) {
		calls++
		if path == bad {
			return Asset{}, errors.New("unsupported source")
		}
		return Asset{ID: filepath.Base(path), Path: path, Duration: 1000000, Width: 1080, Height: 1920}, nil
	}
	result, err := p.ImportPaths(context.Background(), []string{a, a, bad, b, b}, inspect)
	if err != nil || result.Added != 2 || result.Duplicates != 2 || len(result.Failures) != 1 || calls != 3 || result.Selected != 1 {
		t.Fatalf("batch: %+v calls=%d err=%v", result, calls, err)
	}
	if !reflect.DeepEqual(result.AssetIDs, []string{"a.mp4", "b.mp4"}) {
		t.Fatalf("placement order: %v", result.AssetIDs)
	}
	if len(p.Assets) != 0 || len(result.Project.Tracks[0].Clips) != 0 {
		t.Fatal("batch changed original or created clips")
	}
	again, err := result.Project.ImportPaths(context.Background(), []string{a, b}, inspect)
	if err != nil || again.Added != 0 || again.Duplicates != 2 || calls != 3 || !reflect.DeepEqual(again.Project, result.Project) {
		t.Fatal("duplicates were inspected or inserted again")
	}
}

func TestBatchImportCancellationAndLimit(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inspect := func(context.Context, string) (Asset, error) { t.Fatal("unexpected inspect"); return Asset{}, nil }
	if _, err := p.ImportPaths(ctx, []string{"file.mp4"}, inspect); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := p.ImportPaths(context.Background(), make([]string, MaxImportFiles+1), inspect); err == nil {
		t.Fatal("oversized batch accepted")
	}
}
