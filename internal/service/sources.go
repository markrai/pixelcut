package service

import (
	"context"

	"github.com/markrai/pixelcut/internal/mediacache"
)

type SourceScan struct {
	Epoch uint64
	Paths []string
}

type SourceResult struct {
	Epoch   uint64
	Present map[string]mediacache.Source
	Checked map[string]bool
}

func ScanSources(ctx context.Context, scan SourceScan) (SourceResult, error) {
	r := SourceResult{Epoch: scan.Epoch, Present: make(map[string]mediacache.Source), Checked: make(map[string]bool)}
	for _, path := range scan.Paths {
		if err := ctx.Err(); err != nil {
			return SourceResult{}, err
		}
		if r.Checked[path] {
			continue
		}
		r.Checked[path] = true
		if s, err := mediacache.Identify(path); err == nil {
			r.Present[path] = s
		}
	}
	return r, nil
}
