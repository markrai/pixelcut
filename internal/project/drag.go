package project

import (
	"errors"
	"math"
)

func (p Project) DragClip(trackID, clipID, mode string, delta int64) (Clip, error) {
	for ti,t:=range p.Tracks {
		if t.ID!=trackID { continue }
		for ci,c:=range t.Clips {
			if c.ID!=clipID { continue }
			a,ok:=p.Asset(c.AssetID)
			if !ok || c.In<0 || c.Out<=c.In || c.Out>a.Duration || c.Start<0 {
				return c,errors.New("invalid source clip")
			}
			switch mode {
			case "move":
				if delta < -c.Start { delta=-c.Start }
				if delta>math.MaxInt64-c.Start { delta=math.MaxInt64-c.Start }
				c.Start+=delta
			case "left":
				if delta < -c.In { delta=-c.In }
				if delta < -c.Start { delta=-c.Start }
				if delta>=c.Out-c.In { delta=c.Out-c.In-1 }
				if delta>math.MaxInt64-c.Start { return c,errors.New("timeline overflow") }
				c.In+=delta
				c.Start+=delta
			case "right":
				if delta>a.Duration-c.Out { delta=a.Duration-c.Out }
				if delta<=c.In-c.Out { delta=c.In-c.Out+1 }
				c.Out+=delta
			default:
				return c,errors.New("unknown drag mode")
			}
			q:=p.Clone()
			q.Tracks[ti].Clips[ci]=c
			return c,q.Validate()
		}
	}
	return Clip{},errors.New("clip not found")
}
