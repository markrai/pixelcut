package timeline

import (
	"math"

	"github.com/markrai/pixelcut/internal/project"
)

const (
	Header        int32 = 92
	RulerBottom   int32 = 32
	TrackTop      int32 = 33
	TrackBottom   int32 = 97
	LaneTop       int32 = 39
	LaneBottom    int32 = 91
	AudioTop      int32 = 104
	AudioBottom   int32 = 140
	ContentBottom int32 = 145
)

type View struct {
	Width  int32
	Origin int64
	End    int64
	Zoom   float64
}

func (v View) Scale() float64 {
	if v.Zoom > 0 && !math.IsInf(v.Zoom, 0) {
		return v.Zoom
	}
	return math.Max(1e-9, math.Max(1, float64(v.Width)-float64(Header)-18)/math.Max(5, project.Seconds(v.End)*1.05))
}

func (v View) TimeAt(x int32) int64 {
	t := float64(v.Origin) + (float64(x)-float64(Header))/v.Scale()*float64(project.TicksPerSecond)
	if t < 0 {
		return 0
	}
	if t >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(math.Round(t))
}

func (v View) PixelAt(t int64) float64 {
	return float64(Header) + project.Seconds(t-v.Origin)*v.Scale()
}

func (v View) XAt(t int64) int32 {
	return int32(math.Round(max(-100000, min(100000, v.PixelAt(t)))))
}

func (v View) ZoomAt(marker int64, factor float64) (float64, int64) {
	zoom := max(.05, min(1000, v.Scale()*factor))
	offset := int64(max(0, float64(v.Width-Header))/zoom*float64(project.TicksPerSecond)/2)
	return zoom, max(0, marker-offset)
}

func (v View) Scroll(forward bool) int64 {
	step := int64(math.Min(float64(math.MaxInt64/2), math.Max(0, float64(v.Width-Header)/v.Scale()*float64(project.TicksPerSecond)/5)))
	if !forward {
		return max(0, v.Origin-step)
	}
	if v.Origin <= math.MaxInt64-step {
		return v.Origin + step
	}
	return v.Origin
}

type Span struct {
	Left, Right int32
}

func (v View) Span(start, end int64) (Span, bool) {
	if v.Width <= Header || end <= start {
		return Span{}, false
	}
	l, r := v.XAt(start), v.XAt(end)
	if r <= Header || l >= v.Width {
		return Span{}, false
	}
	l, r = max(Header, l), min(v.Width, r)
	if r <= l {
		r = l + 1
	}
	return Span{Left: l, Right: r}, true
}

type ClipGeometry struct {
	Span
	RawLeft, RawRight int32
	LinkedAudio       bool
}

func (v View) Clip(c project.Clip, linkedAudio bool) (ClipGeometry, bool) {
	i, ok := c.TimelineInterval()
	if !ok {
		return ClipGeometry{}, false
	}
	s, ok := v.Span(i.Start, i.End)
	return ClipGeometry{Span: s, RawLeft: v.XAt(i.Start), RawRight: v.XAt(i.End), LinkedAudio: linkedAudio}, ok
}

func (g ClipGeometry) MainHit(x, y int32) string {
	if x < g.Left || x >= g.Right || y < LaneTop || y >= LaneBottom {
		return ""
	}
	if g.RawRight-g.RawLeft >= 18 {
		if x-g.RawLeft < 7 {
			return "left"
		}
		if g.RawRight-x <= 7 {
			return "right"
		}
	}
	return "move"
}

func (g ClipGeometry) AudioHit(x, y int32) bool {
	return g.LinkedAudio && x >= g.Left && x < g.Right && y >= AudioTop && y < AudioBottom
}

func (g ClipGeometry) Hit(x, y int32) bool {
	return g.MainHit(x, y) != "" || g.AudioHit(x, y)
}

func (g ClipGeometry) Intersects(left, top, right, bottom int32) bool {
	if g.Left > right || g.Right <= left {
		return false
	}
	return (top < LaneBottom && bottom >= LaneTop) || (g.LinkedAudio && top < AudioBottom && bottom >= AudioTop)
}

func (v View) DropTarget(x, y int32) bool {
	return x >= Header && x < v.Width && y >= TrackTop && y < TrackBottom
}

func DragStarted(dx, dy int32) bool {
	return math.Abs(float64(dx)) >= 4 || math.Abs(float64(dy)) >= 4
}

func SnapTolerance(scale float64, enabled bool) int64 {
	if !enabled || scale <= 0 || math.IsNaN(scale) {
		return 0
	}
	return int64(math.Min(2000000, math.Round(8/scale*float64(project.TicksPerSecond))))
}

func EditView(marker, end int64, zoom, dragScale float64) (int64, float64) {
	if zoom == 0 && dragScale >= .05 && dragScale <= 1000 {
		zoom = dragScale
	}
	return min(marker, end), zoom
}
