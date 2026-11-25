package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/mediacache"
)

func sourceMatches(path string, want mediacache.Source) bool {
	s, err := mediacache.Identify(path)
	return err == nil && s == want
}

func (e *editor) checkSourceVersions(force bool) {
	if !force && time.Since(e.lastSourceCheck) < 2*time.Second {
		return
	}
	e.lastSourceCheck = time.Now()
	next := make(map[string]mediacache.Source, len(e.p.Assets))
	for _, a := range e.p.Assets {
		if _, ok := next[a.Path]; ok {
			continue
		}
		if s, err := mediacache.Identify(a.Path); err == nil {
			next[a.Path] = s
		}
	}
	if !maps.Equal(e.sourceVersions, next) {
		e.sourceVersions = next
		e.mediaListDirty = true
		e.invalidateTimeline()
	}
	if !force && e.mediaListDirty && e.canOpenMenu() {
		e.refreshMediaList()
	}
}

func (e *editor) openMediaCache() {
	if e.recentDir == "" {
		return
	}
	c, err := mediacache.Open(filepath.Join(e.recentDir, "media-cache-v1"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "PixelCut media cache:", err)
		return
	}
	e.mediaDisk = c
}

func (e *editor) restartMediaWorkers() {
	e.thumbnails = newThumbnailCache(e.engine, e.mediaDisk)
	e.waveforms = newWaveformCache(e.engine, e.mediaDisk)
	e.thumbnailRevision = 0
	e.waveformRevision = 0
	e.invalidateTimeline()
}

const (
	cacheMenuClear = 4001 + iota
	cacheMenuSmall
	cacheMenuDefault
	cacheMenuLarge
)

func (e *editor) showMediaCache() error {
	if !e.canOpenMenu() {
		return nil
	}
	if e.mediaDisk == nil {
		return fmt.Errorf("persistent cache unavailable; using in-memory cache")
	}
	stats := e.mediaDisk.Stats()
	items := []menuItem{
		item(4000, fmt.Sprintf("%.1f MiB / %d MiB (%d entries)", float64(stats.Bytes)/float64(mediacache.MiB), stats.Limit/mediacache.MiB, stats.Entries), false),
		separator(),
		{id: cacheMenuSmall, label: "Limit: 256 MiB", enabled: true, checked: stats.Limit == 256*mediacache.MiB},
		{id: cacheMenuDefault, label: "Limit: 1 GiB", enabled: true, checked: stats.Limit == mediacache.DefaultLimit},
		{id: cacheMenuLarge, label: "Limit: 4 GiB", enabled: true, checked: stats.Limit == 4096*mediacache.MiB},
		separator(), item(cacheMenuClear, "Clear media cache...", true),
	}
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idMediaCache], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate Cache button")
	}
	id := e.popup(items, point{r.Left, r.Bottom})
	if id == 0 {
		return nil
	}
	if id == cacheMenuClear {
		return e.clearMediaCache()
	}
	var limit int64
	switch id {
	case cacheMenuSmall:
		limit = 256 * mediacache.MiB
	case cacheMenuDefault:
		limit = mediacache.DefaultLimit
	case cacheMenuLarge:
		limit = 4096 * mediacache.MiB
	default:
		return nil
	}
	disk := e.mediaDisk
	e.tell("Updating media cache limit...")
	e.startJob(func(ctx context.Context) result {
		if err := ctx.Err(); err != nil {
			return result{err: err}
		}
		err := disk.SetLimit(limit)
		return result{err: err, message: fmt.Sprintf("Media cache limit: %d MiB.", limit/mediacache.MiB)}
	})
	return nil
}

func (e *editor) clearMediaCache() error {
	if e.mediaDisk == nil {
		return fmt.Errorf("persistent cache unavailable")
	}
	msg := "Clear cached thumbnails, waveforms, frame timestamps and media metadata?\n\nProject files, recovery data and source footage will not be removed. Visible media will be regenerated as needed."
	r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(msg))), uintptr(unsafe.Pointer(u("Clear media cache"))), 0x134)
	if r != 6 {
		return nil
	}
	t, w, disk := e.thumbnails, e.waveforms, e.mediaDisk
	if t != nil {
		t.cancel()
	}
	if w != nil {
		w.cancel()
	}
	e.frameCache = nil
	e.tell("Clearing media cache...")
	e.startJob(func(ctx context.Context) result {
		if t != nil {
			<-t.done
		}
		if w != nil {
			<-w.done
		}
		err := disk.Clear()
		return result{cacheCleared: true, err: err, message: "Media cache cleared; visible media will regenerate."}
	})
	return nil
}
