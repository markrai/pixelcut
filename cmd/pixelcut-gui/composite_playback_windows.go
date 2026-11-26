package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/playback"
)

type compositePreviewEvent struct {
	value composition.CompositeFrame
	err   error
}

type compositePreviewRun struct {
	cancel context.CancelFunc
	events chan compositePreviewEvent
	done   chan struct{}
	clock  *playback.SampleClock
}

func launchCompositePreview(engine media.Engine, compiled *composition.Evaluator, at int64) (*compositePreviewRun, error) {
	clock, err := playback.NewSampleClock(at, compiled.Duration())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &compositePreviewRun{cancel: cancel, events: make(chan compositePreviewEvent, 1), done: make(chan struct{}), clock: clock}
	go func() {
		defer close(run.done)
		defer close(run.events)
		defer cancel()
		work, stop := context.WithCancel(ctx)
		defer stop()
		blocks := make(chan composition.PCMBlock, 2)
		ready := make(chan struct{})
		failures := make(chan error, 1)
		var wg sync.WaitGroup
		worker := func(fn func() error) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := fn(); err != nil && work.Err() == nil {
					select {
					case failures <- err:
					default:
					}
					stop()
				}
			}()
		}
		worker(func() error {
			defer close(blocks)
			decoder := media.NewPCMDecoder(work, engine)
			defer decoder.Close()
			for offset := int64(0); offset < clock.Total(); {
				n := int(min(int64(media.PCMBlockFrames), clock.Total()-offset))
				job, cancel := context.WithTimeout(work, 30*time.Second)
				block, err := composition.MixPCM(job, compiled, decoder, at, offset, n)
				cancel()
				if err != nil {
					return err
				}
				select {
				case blocks <- block:
					offset += int64(n)
				case <-work.Done():
					return work.Err()
				}
			}
			return nil
		})
		worker(func() error {
			return playPCM(work, clock, blocks, ready)
		})
		worker(func() error {
			decoder := media.NewStreamDecoder(work, engine)
			defer decoder.Close()
			first := true
			return composition.RenderClockedSequence(work, compiled, decoder, at, clock.Tick, func(ctx context.Context, value composition.CompositeFrame) error {
				select {
				case run.events <- compositePreviewEvent{value: value}:
					if first {
						close(ready)
						first = false
					}
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		})
		wg.Wait()
		select {
		case err := <-failures:
			select {
			case run.events <- compositePreviewEvent{err: err}:
			case <-ctx.Done():
			}
		default:
		}
	}()
	return run, nil
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

func (e *editor) sourcePreviewRetired() bool {
	if e.retiringPreview == nil {
		return true
	}
	select {
	case <-e.retiringPreview:
		e.retiringPreview = nil
		return true
	default:
		return false
	}
}

func (e *editor) startCompositePlayback() error {
	if e.compositeRun != nil {
		return nil
	}
	if e.playhead >= e.endTime() {
		return fmt.Errorf("no timeline remains ahead of the playhead")
	}
	if !e.stillActive {
		e.stopPreview()
	}
	if e.playhead >= e.endTime() {
		return fmt.Errorf("no timeline remains ahead of the playhead")
	}
	if !e.compositeRetired() || !e.sourcePreviewRetired() {
		return e.seekComposite(e.playhead, true)
	}
	compiled, err := e.compiledComposition()
	if err != nil {
		return err
	}
	if err = e.prepareCompositeView(e.playhead); err != nil {
		return err
	}
	run, err := launchCompositePreview(e.engine, compiled, e.playhead)
	if err != nil {
		return err
	}
	e.compositeRun = run
	e.syncPreviewLabel()
	e.updatePlayButton(false)
	e.tell("Composite playback (SDR, audio master). Preparing video and audio buffers.")
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

func (e *editor) freezeCompositePlayback() {
	e.compositeResume = false
	if run := e.compositeRun; run != nil && run.clock != nil {
		tick := run.clock.Tick()
		e.stillTime, e.playhead = tick, tick
		e.refreshPlaybackMarker()
	}
	e.cancelCompositeRun()
	if e.stillFrame == nil || e.compositeFrameTime != e.playhead {
		e.stillFrame = nil
		e.stillError = ""
		e.stillDue = time.Now().Add(compositeSeekDelay)
	}
	e.syncPreviewLabel()
	e.updatePlayButton(true)
	user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
}

func (e *editor) pauseCompositePlayback() {
	e.freezeCompositePlayback()
	if e.stillFrame == nil {
		if err := e.showCompositeStill(); err != nil {
			e.tell(err.Error())
		}
		return
	}
	e.tell("Composite preview paused at the audio-confirmed marker.")
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
	tick := run.clock.Tick()
	if tick != e.playhead {
		e.stillTime, e.playhead = tick, tick
		e.refreshPlaybackMarker()
	}
	if e.stillFrame != nil && !(composition.CompositeFrame{Time: e.compositeFrameTime, ValidUntil: e.compositeFrameUntil, End: e.compositeFrameEnd}).CurrentAt(tick) {
		e.stillFrame = nil
		user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 1)
	}
	select {
	case event, ok := <-run.events:
		if !ok {
			e.freezeCompositePlayback()
			if e.playhead == e.endTime() {
				e.tell("End of composite timeline.")
			} else {
				e.tell("Composite playback ended before the timeline end.")
			}
			return
		}
		if event.err != nil {
			e.freezeCompositePlayback()
			e.stillDue = time.Time{}
			e.stillError = event.err.Error()
			e.tell("Composite playback stopped: " + event.err.Error())
			return
		}
		f := event.value
		now := run.clock.Tick()
		if f.Time > now || f.Time > e.endTime() || !f.Frame.Valid() {
			e.stopPreview()
			e.tell("Composite playback returned an invalid frame.")
			return
		}
		if !f.CurrentAt(now) {
			return
		}
		for path, identity := range f.Sources {
			if e.sourceChecked[path] && e.sourceVersions[path] != identity {
				e.stopPreview()
				e.tell("Composite playback stopped: source changed.")
				return
			}
		}
		if now != e.playhead {
			e.stillTime, e.playhead = now, now
			e.refreshPlaybackMarker()
		}
		e.stillFrame = &f.Frame
		e.stillSources = f.Sources
		e.compositeFrameTime = f.Time
		e.compositeFrameUntil = f.ValidUntil
		e.compositeFrameEnd = f.End
		user.NewProc("InvalidateRect").Call(e.layerSurface, 0, 0)
	default:
	}
}
