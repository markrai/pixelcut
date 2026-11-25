package timeline

import "github.com/markrai/pixelcut/internal/project"

const (
	VideoLaneHeight int32 = ContentBottom - TrackTop
	AudioLaneHeight int32 = TrackBottom - TrackTop
	FooterHeight    int32 = 24
)

type Lane struct {
	TrackID     string
	Kind        string
	Top, Bottom int32
}

func (l Lane) Offset() int32 { return l.Top - TrackTop }

func (l Lane) MainContains(y int32) bool {
	return y >= l.Top && y < l.Top+AudioLaneHeight
}

type Stack struct {
	Lanes             []Lane
	Scroll, MaxScroll int32
	Bottom            int32
}

func LayoutStack(p project.Project, height, scroll int32) Stack {
	s := Stack{Bottom: max(TrackTop, height-FooterHeight)}
	top := TrackTop
	add := func(t project.Track, height int32) {
		s.Lanes = append(s.Lanes, Lane{TrackID: t.ID, Kind: t.Kind, Top: top, Bottom: top + height})
		top += height
	}
	for i := len(p.Tracks) - 1; i >= 0; i-- {
		if p.Tracks[i].Kind == project.VideoTrack {
			add(p.Tracks[i], VideoLaneHeight)
		}
	}
	for _, t := range p.Tracks {
		if t.Kind == project.AudioTrack {
			add(t, AudioLaneHeight)
		}
	}
	s.MaxScroll = max(0, top-s.Bottom)
	s.Scroll = max(0, min(scroll, s.MaxScroll))
	for i := range s.Lanes {
		s.Lanes[i].Top -= s.Scroll
		s.Lanes[i].Bottom -= s.Scroll
	}
	return s
}

func (s Stack) At(y int32) (Lane, bool) {
	if y < TrackTop || y >= s.Bottom {
		return Lane{}, false
	}
	for _, l := range s.Lanes {
		if y >= l.Top && y < l.Bottom {
			return l, true
		}
	}
	return Lane{}, false
}

func (s Stack) Find(id string) (Lane, bool) {
	for _, l := range s.Lanes {
		if l.TrackID == id {
			return l, true
		}
	}
	return Lane{}, false
}

func (s Stack) Reveal(id string) int32 {
	l, ok := s.Find(id)
	if !ok {
		return s.Scroll
	}
	if l.Top < TrackTop || l.Bottom-l.Top > s.Bottom-TrackTop {
		return max(0, min(s.MaxScroll, s.Scroll+l.Top-TrackTop))
	}
	if l.Bottom > s.Bottom {
		return max(0, min(s.MaxScroll, s.Scroll+l.Bottom-s.Bottom))
	}
	return s.Scroll
}
