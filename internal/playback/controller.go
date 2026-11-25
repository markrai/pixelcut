package playback

import (
	"math"
	"sync"

	"github.com/markrai/pixelcut/internal/project"
)

type State uint8

const (
	Loading State = iota
	Seeking
	Playing
	Paused
	Finished
	Failed
	Stopped
)

type Request struct {
	Serial     uint64
	SeekSerial uint64
	Tick       int64
	Paused     bool
	Speed      float64
}

type Sample struct {
	Serial   uint64
	Tick     int64
	Paused   bool
	Finished bool
}

type Status struct {
	State     State
	Tick      int64
	Paused    bool
	Confirmed bool
}

type Controller struct {
	mu      sync.Mutex
	span    project.Interval
	request Request
	status  Status
}

func New(span project.Interval, at int64, paused bool) *Controller {
	if !span.Valid() || !span.Contains(at) {
		return &Controller{status: Status{State: Failed, Tick: at, Paused: true}}
	}
	return &Controller{
		span:    span,
		request: Request{Tick: at, Paused: paused, Speed: 1},
		status:  Status{State: Loading, Tick: at, Paused: paused},
	}
}

func (c *Controller) Request() Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.request
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Controller) usable() bool {
	return c.status.State != Failed && c.status.State != Stopped
}

func (c *Controller) Usable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usable()
}

func (c *Controller) Queue(tick int64, seek, paused bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable() || (!seek && !paused && c.status.State == Finished) {
		return false
	}
	if seek && !c.span.Contains(tick) {
		return false
	}
	r := &c.request
	if seek && r.SeekSerial != 0 && r.Tick == tick && !c.status.Confirmed {
		seek = false
	}
	r.Serial++
	if seek {
		r.SeekSerial++
		r.Tick = tick
		c.status.Tick = tick
		c.status.State = Seeking
	} else if c.status.State != Loading && c.status.State != Seeking {
		if paused {
			c.status.State = Paused
		} else {
			c.status.State = Playing
		}
	}
	r.Paused = paused
	c.status.Paused = paused
	c.status.Confirmed = false
	return true
}

func (c *Controller) SetSpeed(speed float64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable() || speed <= 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return false
	}
	if c.request.Speed == speed {
		return true
	}
	c.request.Paused = c.status.Paused
	c.request.Speed = speed
	c.request.Serial++
	c.status.Confirmed = false
	return true
}

func (c *Controller) Report(s Sample) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable() || s.Serial != c.request.Serial || s.Tick < c.span.Start || s.Tick > c.span.End {
		return false
	}
	if c.status.State == Finished && !s.Finished {
		return false
	}
	state := Playing
	if s.Finished || s.Tick == c.span.End {
		state = Finished
		s.Tick = c.span.End
		s.Paused = true
	} else if s.Paused {
		state = Paused
	}
	c.status = Status{State: state, Tick: s.Tick, Paused: s.Paused, Confirmed: true}
	return true
}

func (c *Controller) Fail() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.State != Stopped {
		c.status.State = Failed
		c.status.Paused = true
		c.status.Confirmed = false
	}
}

func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.State = Stopped
	c.status.Paused = true
	c.status.Confirmed = false
}

func SourceToTimeline(c project.Clip, seconds float64) (int64, bool) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || c.Duration() == 0 || c.End() == 0 {
		return 0, false
	}
	if seconds <= project.Seconds(c.In) {
		return c.Start, true
	}
	if seconds >= project.Seconds(c.Out) {
		return c.End(), true
	}
	t := math.Round(seconds * float64(project.TicksPerSecond))
	if t >= float64(math.MaxInt64) {
		return c.End(), true
	}
	return c.Start + (max(c.In, min(c.Out, int64(t))) - c.In), true
}

func TimelineToSource(c project.Clip, tick int64) float64 {
	if c.Duration() == 0 || c.End() == 0 {
		return project.Seconds(c.In)
	}
	tick = max(c.Start, min(c.End()-1, tick))
	return project.Seconds(c.In + (tick - c.Start))
}
