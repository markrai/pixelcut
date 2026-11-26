package main

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/playback"
	"github.com/markrai/pixelcut/internal/playback/mpv"
	"github.com/markrai/pixelcut/internal/project"
)

type previewClock struct {
	clip        project.Clip
	transport   *playback.Controller
	unavailable atomic.Bool
	cropRequest atomic.Pointer[cropRequest]
	cropSample  atomic.Pointer[cropSample]
	cropError   atomic.Bool
}

func newPreviewClock(c project.Clip, marker int64, paused bool) *previewClock {
	span, _ := c.TimelineInterval()
	return &previewClock{clip: c.Clone(), transport: playback.New(span, marker, paused)}
}

func (c *previewClock) paused() bool {
	return c.transport.Status().Paused
}

func (c *previewClock) queue(tick int64, seek, paused bool) {
	c.transport.Queue(tick, seek, paused)
}

func (c *previewClock) setSpeed(speed float64) {
	c.transport.SetSpeed(speed)
}

func (c *previewClock) fail() {
	c.transport.Fail()
	c.unavailable.Store(true)
}

func previewPipeName(configPath string) string {
	return "\\\\.\\pipe\\pixelcut-" + filepath.Base(configPath)
}

func (c *previewClock) observe(ctx context.Context, pipe string, processDone <-chan struct{}) {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var f *os.File
	for {
		select {
		case <-ctx.Done():
			return
		case <-processDone:
			return
		default:
		}
		var err error
		f, err = os.OpenFile(pipe, os.O_RDWR, 0)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-processDone:
			return
		case <-deadline.C:
			c.fail()
			return
		case <-ticker.C:
		}
	}
	defer f.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = f.Close() })
	defer stopClose()
	p := mpv.NewIPC(f, f)
	var applied, appliedSeek, appliedCrop uint64
	appliedSpeed := 1.0
	var seekUntil time.Time
	var failures int
	for {
		select {
		case <-ctx.Done():
			return
		case <-processDone:
			return
		case <-ticker.C:
		}
		var source float64
		if !p.Property("time-pos", &source) {
			failures++
			if failures >= 80 {
				c.fail()
				return
			}
			continue
		}
		failures = 0
		if req := c.cropRequest.Load(); req != nil && req.serial != appliedCrop {
			var ready bool
			if p.Property("user-data/pixelcut/crop-ready", &ready) && ready {
				if _, err := p.Call("script-message", "pixelcut-crop", req.payload()); err != nil {
					c.cropError.Store(true)
				} else {
					appliedCrop = req.serial
				}
			}
		}
		if appliedCrop != 0 {
			var state cropSample
			if p.Property("user-data/pixelcut/crop", &state) && state.Serial == appliedCrop {
				c.cropSample.Store(&state)
			}
		}
		r := c.transport.Request()
		if r.Serial != applied {
			if r.Speed != appliedSpeed {
				if _, err := p.Call("set_property", "speed", r.Speed); err != nil {
					c.fail()
					return
				}
				appliedSpeed = r.Speed
			}
			if r.Paused {
				if _, err := p.Call("set_property", "pause", true); err != nil {
					c.fail()
					return
				}
			}
			if r.SeekSerial != appliedSeek {
				if _, err := p.Call("seek", playback.TimelineToSource(c.clip, r.Tick), "absolute+exact"); err != nil {
					c.fail()
					return
				}
				appliedSeek = r.SeekSerial
				seekUntil = time.Now().Add(2 * time.Second)
			}
			if !r.Paused {
				if _, err := p.Call("set_property", "pause", false); err != nil {
					c.fail()
					return
				}
			}
			applied = r.Serial
			continue
		}
		var seeking, paused, eof bool
		if !p.Property("seeking", &seeking) || seeking {
			continue
		}
		if !p.Property("pause", &paused) {
			continue
		}
		p.Property("eof-reached", &eof)
		tick, ok := playback.SourceToTimeline(c.clip, source)
		if !ok {
			continue
		}
		if !seekUntil.IsZero() {
			distance := math.Abs(float64(tick) - float64(r.Tick))
			if distance > 250000 && time.Now().Before(seekUntil) {
				continue
			}
			seekUntil = time.Time{}
		}
		if eof {
			tick = c.clip.End()
			paused = true
		}
		c.transport.Report(playback.Sample{Tick: tick, Paused: paused, Finished: eof, Serial: applied})
	}
}

type nativePreview struct {
	clock  *previewClock
	cancel context.CancelFunc
	done   chan struct{}
}

func launchNativePreview(engine media.Engine, host uintptr, a project.Asset, c project.Clip, marker int64, speed float64, hdr, paused, audioOnly, silent bool, finished func(error)) (*nativePreview, error) {
	source, ok := c.SourceAt(marker)
	if !ok {
		return nil, project.ErrInvalidClip
	}
	ctx, cancel := context.WithCancel(context.Background())
	bindings, err := os.CreateTemp("", "pixelcut-input-*.conf")
	if err != nil {
		cancel()
		return nil, err
	}
	name := bindings.Name()
	_, err = bindings.WriteString("SPACE cycle pause\nESC set pause yes\n")
	closeErr := bindings.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(name)
		cancel()
		return nil, err
	}
	pipe := previewPipeName(name)
	var script string
	var cmd *exec.Cmd
	if audioOnly {
		cmd, err = engine.AudioPreviewGain(ctx, a.Path, project.Seconds(source), project.Seconds(c.Out-source), pipe, paused, c.EffectiveGainPercent())
	} else {
		script, err = writeCropScript()
		if err == nil {
			cmd, err = engine.EmbeddedPreviewGain(ctx, a.Path, host, project.Seconds(source), project.Seconds(c.Out-source), hdr, name, pipe, paused, script, silent, c.EffectiveGainPercent())
		}
	}
	if err != nil {
		os.Remove(script)
		os.Remove(name)
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		os.Remove(script)
		os.Remove(name)
		cancel()
		return nil, err
	}
	done := make(chan struct{})
	clock := newPreviewClock(c, marker, paused)
	clock.setSpeed(speed)
	if !audioOnly {
		clock.setComposition(a, c.Crop, c.Transform, false)
	}
	processDone := make(chan struct{})
	clockDone := make(chan struct{})
	go func() { defer close(clockDone); clock.observe(ctx, pipe, processDone) }()
	go func() {
		err := cmd.Wait()
		close(processDone)
		select {
		case <-clockDone:
		case <-time.After(150 * time.Millisecond):
		}
		os.Remove(name)
		os.Remove(script)
		close(done)
		if ctx.Err() == nil {
			finished(err)
		}
	}()
	return &nativePreview{clock: clock, cancel: cancel, done: done}, nil
}
