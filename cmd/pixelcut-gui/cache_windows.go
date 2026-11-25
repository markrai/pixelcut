package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/service"
)

func sourceMatches(path string, want mediacache.Source) bool {
	s, err := mediacache.Identify(path)
	return err == nil && s == want
}

func (e *editor) sourcesReady() bool {
	return e.sourceRequest == e.sourceDone
}

func (e *editor) checkSourceVersions(force bool) {
	if e.sourceScanner == nil {
		e.sourceScanner = service.NewLatest(service.ScanSources)
	}
	paths := e.sourcePaths
	fingerprint := e.model.Fingerprint()
	if fingerprint != e.sourceFingerprint {
		paths = make([]string, 0, len(e.model.Project().Assets))
		for _, a := range e.model.Project().Assets {
			paths = append(paths, a.Path)
		}
		e.sourceFingerprint = fingerprint
	}
	changed := !slices.Equal(paths, e.sourcePaths)
	if changed {
		e.sourcePaths = paths
		e.sourceEpoch++
		e.sourceChecked = nil
		e.sourceVersions = nil
		e.mediaListDirty = true
		e.invalidateTimeline()
	}
	r := e.sourceScanner.Result()
	if r.Sequence != 0 && r.Sequence > e.sourceDone {
		e.sourceDone = r.Sequence
		if r.Err == nil && r.Value.Epoch == e.sourceEpoch {
			if !maps.Equal(e.sourceVersions, r.Value.Present) || !maps.Equal(e.sourceChecked, r.Value.Checked) {
				e.sourceVersions, e.sourceChecked = r.Value.Present, r.Value.Checked
				e.mediaListDirty = true
				e.invalidateTimeline()
			}
		}
	}
	if !force && e.restorePending && e.sourcesReady() {
		e.restorePending = false
		if e.restoreView == e.view() && e.restoreFingerprint == e.model.Fingerprint() && !e.busy && !e.previewActive() {
			e.finishRecovery()
		}
	}
	if changed || (e.sourcesReady() && (force || time.Since(e.lastSourceCheck) >= 2*time.Second)) {
		e.lastSourceCheck = time.Now()
		e.sourceRequest = e.sourceScanner.Submit(service.SourceScan{Epoch: e.sourceEpoch, Paths: paths})
	}
	e.pruneRetiredWorkers()
	if e.mediaWorkersPending && !e.busy && len(e.retiredWorkers) < 4 {
		e.restartMediaWorkers()
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

func (e *editor) retireMediaWorkers() {
	e.mediaWorkersPending = false
	if e.thumbnails != nil {
		e.thumbnails.cancel()
		e.retiredWorkers = append(e.retiredWorkers, e.thumbnails.done)
		e.thumbnails = nil
	}
	if e.waveforms != nil {
		e.waveforms.cancel()
		e.retiredWorkers = append(e.retiredWorkers, e.waveforms.done)
		e.waveforms = nil
	}
	e.pruneRetiredWorkers()
}

func (e *editor) pruneRetiredWorkers() {
	pending := e.retiredWorkers[:0]
	for _, done := range e.retiredWorkers {
		select {
		case <-done:
		default:
			pending = append(pending, done)
		}
	}
	e.retiredWorkers = pending
}

func (e *editor) restartMediaWorkers() {
	e.retireMediaWorkers()
	if len(e.retiredWorkers) >= 4 {
		e.mediaWorkersPending = true
		return
	}
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
	disk := e.mediaDisk
	e.retireMediaWorkers()
	retired := append([]<-chan struct{}(nil), e.retiredWorkers...)
	e.frameCache = nil
	e.tell("Clearing media cache...")
	e.startJob(func(ctx context.Context) result {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := service.WaitAll(ctx, retired); err != nil {
			return result{cacheCleared: true, err: fmt.Errorf("cache clear deferred: workers still stopping: %w", err)}
		}
		err := disk.Clear()
		return result{cacheCleared: true, err: err, message: "Media cache cleared; visible media will regenerate."}
	})
	return nil
}
