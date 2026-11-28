package main

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/service"
)

type stillPreviewRequest struct {
	ctx  context.Context
	plan composition.LayerPlan
}

type stillPreviewResult struct {
	frame   media.StillFrame
	sources map[string]mediacache.Source
}

func newStillWorker(engine media.Engine, disk *mediacache.Cache) *service.Latest[stillPreviewRequest, stillPreviewResult] {
	decoder := media.NewStillDecoderWithCache(engine, disk)
	return service.NewLatest(func(parent context.Context, req stillPreviewRequest) (stillPreviewResult, error) {
		ctx, cancel := context.WithTimeout(req.ctx, 90*time.Second)
		defer cancel()
		stop := context.AfterFunc(parent, cancel)
		defer stop()
		if err := ctx.Err(); err != nil {
			return stillPreviewResult{}, err
		}
		sources := make(map[string]mediacache.Source)
		for _, layer := range req.plan.DecodeLayers() {
			path := layer.Layer.Asset.Path
			if _, ok := sources[path]; ok {
				continue
			}
			identity, err := mediacache.Identify(path)
			if err != nil {
				return stillPreviewResult{}, fmt.Errorf("source unavailable: %w", err)
			}
			sources[path] = identity
		}
		frame, err := composition.RenderStill(ctx, decoder, req.plan, media.MaxStillSide)
		if err != nil {
			return stillPreviewResult{}, err
		}
		for path, identity := range sources {
			if !sourceMatches(path, identity) {
				return stillPreviewResult{}, fmt.Errorf("source changed during composite preview")
			}
		}
		return stillPreviewResult{frame: frame, sources: sources}, ctx.Err()
	})
}

const compositeSeekDelay = 80 * time.Millisecond

func (e *editor) showCompositeStill() error {
	e.stopPreview()
	if err := e.prepareCompositeView(e.playhead); err != nil {
		return err
	}
	return e.submitCompositeStill()
}

func (e *editor) prepareCompositeView(at int64) error {
	if e.layerSurface == 0 {
		return fmt.Errorf("composite preview surface unavailable")
	}
	wasActive := e.stillActive
	e.cancelCompositeRun()
	if e.stillCancel != nil {
		e.stillCancel()
		e.stillCancel = nil
	}
	e.stillSequence = 0
	e.stillDue = time.Time{}
	e.compositeResume = false
	e.stillFrame = nil
	e.stillSources = nil
	e.stillError = ""
	e.stillActive = true
	e.stillTime, e.playhead = at, at
	e.stillFingerprint = e.model.Fingerprint()
	if !wasActive {
		user.NewProc("ShowWindow").Call(e.layerSurface, 5)
		user.NewProc("SetWindowPos").Call(e.layerSurface, 0, 0, 0, 0, 0x0013)
	}
	user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
	e.syncPreviewLabel()
	e.updatePlayButton(true)
	e.refreshPlaybackMarker()
	return nil
}

func (e *editor) seekComposite(at int64, resume bool) error {
	if e.stillActive && e.compositeRun == nil && !e.compositeResume && !resume && e.stillTime == at && e.stillFingerprint == e.model.Fingerprint() && (e.stillFrame != nil || e.stillSequence != 0 || !e.stillDue.IsZero()) {
		return nil
	}
	if err := e.prepareCompositeView(at); err != nil {
		return err
	}
	e.compositeResume = resume && at < e.endTime()
	e.stillDue = time.Now().Add(compositeSeekDelay)
	if e.compositeResume {
		e.updatePlayButton(false)
		e.syncPreviewLabel()
		e.tell("Seeking composite playback...")
	} else {
		e.tell("Seeking composite frame...")
	}
	return nil
}

func (e *editor) submitCompositeStill() error {
	compiled, err := e.compiledComposition()
	if err != nil {
		return err
	}
	plan, err := compiled.PlanLayers(e.stillTime)
	if err != nil {
		return err
	}
	if e.stillWorker == nil {
		e.stillWorker = newStillWorker(e.engine, e.mediaDisk)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.stillCancel = cancel
	e.stillSequence = e.stillWorker.Submit(stillPreviewRequest{ctx: ctx, plan: plan})
	e.stillDue = time.Time{}
	e.tell("Rendering composite frame (SDR). First source indexing may take time.")
	return nil
}

func (e *editor) cancelCompositeStill() {
	e.cancelCompositeRun()
	if e.stillCancel != nil {
		e.stillCancel()
		e.stillCancel = nil
	}
	e.stillActive = false
	e.stillDue = time.Time{}
	e.compositeResume = false
	e.stillSequence = 0
	e.stillFrame = nil
	e.stillSources = nil
	e.stillFingerprint = ""
	e.stillError = ""
}

func (e *editor) updateCompositeStill() {
	if !e.stillActive || e.compositeRun != nil {
		return
	}
	if e.stillFingerprint != e.model.Fingerprint() || e.stillTime != e.playhead {
		e.hideLayerLayout()
		return
	}
	if e.compositeResume {
		if time.Now().Before(e.stillDue) || e.scrubbing || e.drag != nil || e.marquee != nil || e.menuOpen || e.busy || !e.compositeRetired() || !e.sourcePreviewRetired() {
			return
		}
		e.compositeResume = false
		if err := e.startCompositePlayback(); err != nil {
			e.stillError = err.Error()
			e.updatePlayButton(true)
			e.syncPreviewLabel()
			e.tell(err.Error())
			user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
		}
		return
	}
	if !e.stillDue.IsZero() && !time.Now().Before(e.stillDue) && e.drag == nil && e.marquee == nil && e.compositeRetired() && e.sourcePreviewRetired() {
		e.stillDue = time.Time{}
		if err := e.submitCompositeStill(); err != nil {
			e.stillError = err.Error()
			e.tell(err.Error())
			user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
		}
	}
	if e.stillSequence != 0 && e.stillWorker != nil {
		r := e.stillWorker.Result()
		if r.Sequence != e.stillSequence {
			return
		}
		e.stillSequence = 0
		if e.stillCancel != nil {
			e.stillCancel()
			e.stillCancel = nil
		}
		if r.Err != nil {
			e.stillError = r.Err.Error()
			e.tell("Composite frame failed: " + e.stillError)
		} else {
			e.stillFrame = &r.Value.frame
			e.compositeFrameTime = e.stillTime
			e.stillSources = r.Value.sources
			e.tell("Composite frame (SDR, paused). Play starts composite playback with audio; seek to inspect another frame.")
		}
		user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
	}
	for path, identity := range e.stillSources {
		if e.sourceChecked[path] && e.sourceVersions[path] != identity {
			e.hideLayerLayout()
			e.tell("Composite frame invalidated by a source change.")
			return
		}
	}
}

func (e *editor) drawCompositeStill(d drawItem) {
	w, h := d.Rect.Right-d.Rect.Left, d.Rect.Bottom-d.Rect.Top
	f := e.stillFrame
	if f == nil || !f.Valid() {
		message := "Rendering composite frame..."
		if e.compositeResume {
			message = "Seeking synchronized playback..."
		} else if e.compositeRun != nil {
			message = "Waiting for synchronized video..."
		}
		if e.stillError != "" {
			message = "Composite frame unavailable; see status."
		}
		textAt(d.DC, d.Rect, message, palette.muted, 1|4)
		return
	}
	if w <= 0 || h <= 0 {
		return
	}
	scale := math.Min(float64(w)/float64(f.Width), float64(h)/float64(f.Height))
	width, height := max(1, int32(math.Round(float64(f.Width)*scale))), max(1, int32(math.Round(float64(f.Height)*scale)))
	x, y := (w-width)/2, (h-height)/2
	info := bitmapInfo{Size: 40, Width: int32(f.Width), Height: -int32(f.Height), Planes: 1, BitCount: 32}
	gdi.NewProc("StretchDIBits").Call(d.DC, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, uintptr(f.Width), uintptr(f.Height), uintptr(unsafe.Pointer(&f.Pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0x00cc0020)
	runtime.KeepAlive(f)
	runtime.KeepAlive(&info)
}

func placementButtonLabel(t project.Track) string {
	if t.Kind == project.AudioTrack {
		return "Audio to " + t.ID
	}
	return "Video to " + t.ID
}

func placementStatus(t project.Track) string {
	if t.Kind == project.AudioTrack {
		return "Audio only added to " + t.ID + ". For video, choose a video track (V1, V2...)."
	}
	return "Video added to " + t.ID + " with linked audio when present."
}
