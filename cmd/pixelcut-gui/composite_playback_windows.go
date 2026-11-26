package main

import (
	"context"
	"fmt"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
)

type compositePreviewEvent struct {
	value composition.CompositeFrame
	err   error
}

type compositePreviewRun struct {
	cancel context.CancelFunc
	events chan compositePreviewEvent
	done   chan struct{}
}

func launchCompositePreview(engine media.Engine, compiled *composition.Evaluator, at int64) *compositePreviewRun {
	ctx, cancel := context.WithCancel(context.Background())
	run := &compositePreviewRun{cancel: cancel, events: make(chan compositePreviewEvent, 1), done: make(chan struct{})}
	go func() {
		defer close(run.done)
		defer close(run.events)
		defer cancel()
		decoder := media.NewStreamDecoder(ctx, engine)
		defer decoder.Close()
		err := composition.RenderSequence(ctx, compiled, decoder, at, func(ctx context.Context, value composition.CompositeFrame) error {
			select {
			case run.events <- compositePreviewEvent{value: value}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil && ctx.Err() == nil {
			select {
			case run.events <- compositePreviewEvent{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return run
}

func (e *editor) compositeRetired() bool {
	if e.retiringComposite == nil {
		return true
	}
	select {
	case <-e.retiringComposite:
		e.retiringComposite = nil
		return true
	default:
		return false
	}
}

func (e *editor) startCompositePlayback() error {
	if e.compositeRun != nil {
		return nil
	}
	if !e.compositeRetired() {
		return fmt.Errorf("previous composite decoders are closing; retry Play")
	}
	if e.playhead >= e.endTime() {
		return fmt.Errorf("no timeline remains ahead of the playhead")
	}
	if e.layerSurface == 0 {
		return fmt.Errorf("composite preview surface unavailable")
	}
	compiled, err := composition.Compile(e.model.Project())
	if err != nil {
		return err
	}
	e.stopPreview()
	if e.playhead >= compiled.Duration() {
		return fmt.Errorf("no timeline remains ahead of the playhead")
	}
	e.stillActive = true
	e.stillTime = e.playhead
	e.stillFingerprint = e.model.Fingerprint()
	e.compositeRun = launchCompositePreview(e.engine, compiled, e.playhead)
	user.NewProc("ShowWindow").Call(e.layerSurface, 5)
	user.NewProc("SetWindowPos").Call(e.layerSurface, 0, 0, 0, 0, 0x0013)
	user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
	e.syncPreviewLabel()
	e.updatePlayButton(false)
	e.tell("Composite playback (SDR, silent, 10 fps target). Indexing sources on first use.")
	return nil
}

func (e *editor) cancelCompositeRun() {
	if e.compositeRun == nil {
		return
	}
	run := e.compositeRun
	e.compositeRun = nil
	run.cancel()
	e.retiringComposite = run.done
}

func (e *editor) pauseCompositePlayback() {
	e.cancelCompositeRun()
	if e.stillFrame == nil {
		e.stillError = "Paused before first frame."
	}
	e.syncPreviewLabel()
	e.updatePlayButton(true)
	user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
	e.tell("Composite preview paused. Play resumes from the displayed marker.")
}

func (e *editor) updateCompositePlayback() {
	run := e.compositeRun
	if run == nil {
		return
	}
	if e.stillFingerprint != e.model.Fingerprint() || e.stillTime != e.playhead {
		e.stopPreview()
		return
	}
	for path, identity := range e.stillSources {
		if e.sourceChecked[path] && e.sourceVersions[path] != identity {
			e.stopPreview()
			e.tell("Composite playback stopped: source changed.")
			return
		}
	}
	if e.drag != nil || e.scrubbing || e.marquee != nil {
		return
	}
	select {
	case event, ok := <-run.events:
		if !ok {
			e.pauseCompositePlayback()
			e.tell("Composite playback ended.")
			return
		}
		if event.err != nil {
			e.pauseCompositePlayback()
			e.tell("Composite playback stopped: " + event.err.Error())
			return
		}
		f := event.value
		if f.Time < e.playhead || f.Time > e.endTime() || !f.Frame.Valid() {
			e.stopPreview()
			e.tell("Composite playback returned an invalid frame.")
			return
		}
		for path, identity := range f.Sources {
			if e.sourceChecked[path] && e.sourceVersions[path] != identity {
				e.stopPreview()
				e.tell("Composite playback stopped: source changed.")
				return
			}
		}
		e.stillFrame = &f.Frame
		e.stillSources = f.Sources
		e.stillTime, e.playhead = f.Time, f.Time
		e.refreshPlaybackMarker()
		user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 0)
		if f.End {
			e.pauseCompositePlayback()
			e.tell("End of composite timeline.")
		}
	default:
	}
}
