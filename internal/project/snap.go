package project

import (
	"errors"
	"math"
	"sort"
)

type SnapResult struct {
	Clip Clip
	Snapped bool
	Target int64
}

func clipEdge(c Clip, right bool) (int64,bool) {
	if c.Start<0 || c.In<0 || c.Out<=c.In || c.Start>math.MaxInt64-(c.Out-c.In) {return 0,false}
	if right {return c.Start+(c.Out-c.In),true}
	return c.Start,true
}

func (p Project) SnapDrag(trackID,clipID,mode string,delta,marker,tolerance int64) (SnapResult,error) {
	raw,rawErr:=p.DragClip(trackID,clipID,mode,delta)
	result:=SnapResult{Clip:raw}
	if tolerance<=0 {return result,rawErr}
	var original Clip
	var found bool
	targets:=[]int64{0}
	if marker>=0 {targets=append(targets,marker)}
	for _,track:=range p.Tracks {
		if track.ID!=trackID {continue}
		for _,c:=range track.Clips {
			if c.ID==clipID {original,found=c,true;continue}
			if edge,ok:=clipEdge(c,false);ok {targets=append(targets,edge)}
			if edge,ok:=clipEdge(c,true);ok {targets=append(targets,edge)}
		}
	}
	if !found {return result,rawErr}
	if mode!="move" && mode!="left" && mode!="right" {return result,errors.New("unknown drag mode")}
	type candidate struct {target,delta,distance int64;right bool}
	var candidates []candidate
	for _,right:=range []bool{false,true} {
		if (mode=="left" && right) || (mode=="right" && !right) {continue}
		edge,ok:=clipEdge(raw,right)
		base,valid:=clipEdge(original,right)
		if !ok || !valid {continue}
		for _,target:=range targets {
			d:=target-edge
			if d<0 {d=-d}
			if d<=tolerance {candidates=append(candidates,candidate{target:target,delta:target-base,distance:d,right:right})}
		}
	}
	sort.SliceStable(candidates,func(i,j int) bool {
		if candidates[i].distance!=candidates[j].distance {return candidates[i].distance<candidates[j].distance}
		return candidates[i].target<candidates[j].target
	})
	for _,candidate:=range candidates {
		c,err:=p.DragClip(trackID,clipID,mode,candidate.delta)
		if err!=nil {continue}
		edge,ok:=clipEdge(c,candidate.right)
		if ok && edge==candidate.target {return SnapResult{Clip:c,Snapped:true,Target:candidate.target},nil}
	}
	return result,rawErr
}
