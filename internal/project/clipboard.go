package project

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

type ClipClipboard struct {
	clips []Clip
	kind string
	assets map[string]Asset
	span int64
	originalEnd int64
}

func (c ClipClipboard) Count() int {return len(c.clips)}
func (c ClipClipboard) Span() int64 {return c.span}
func (c ClipClipboard) OriginalEnd() int64 {return c.originalEnd}

func cloneClipboardClip(c Clip) Clip {
	if c.Crop!=nil {v:=*c.Crop;c.Crop=&v}
	if c.Transform!=nil {v:=*c.Transform;c.Transform=&v}
	return c
}

func (p Project) CopyClips(trackID string,ids []string) (ClipClipboard,error) {
	var b ClipClipboard
	if err:=p.Validate();err!=nil {return b,err}
	i,selected,err:=p.selectedTrack(trackID,ids)
	if err!=nil {return b,err}
	b.kind=p.Tracks[i].Kind
	b.assets=map[string]Asset{}
	for _,c:=range p.Tracks[i].Clips {
		if !selected[c.ID] {continue}
		b.clips=append(b.clips,cloneClipboardClip(c))
		a,_:=p.Asset(c.AssetID)
		b.assets[a.ID]=a
		b.originalEnd=max(b.originalEnd,c.Start+(c.Out-c.In))
	}
	sort.Slice(b.clips,func(i,j int)bool{return b.clips[i].Start<b.clips[j].Start})
	origin:=b.clips[0].Start
	b.span=b.originalEnd-origin
	for i:=range b.clips {b.clips[i].Start-=origin}
	return b,nil
}

type ClipboardPlacement struct {
	Project Project
	FirstClip int
	IDs []string
	Start int64
}

func (p Project) PasteClips(trackID string,b ClipClipboard,at int64,ids []string) (ClipboardPlacement,error) {
	result:=ClipboardPlacement{Project:p,FirstClip:-1}
	if b.Count()==0 {return result,errors.New("clip clipboard is empty")}
	if len(ids)!=b.Count() {return result,errors.New("invalid clipboard clip IDs")}
	if at<0 || b.span<=0 || at>math.MaxInt64-b.span {return result,errors.New("clipboard placement outside timeline bounds")}
	if err:=p.Validate();err!=nil {return result,err}
	ti:=-1
	for i,t:=range p.Tracks {if t.ID==trackID && t.Kind==b.kind {ti=i;break}}
	if ti<0 {return result,errors.New("clipboard requires a track of the same kind")}
	for id,source:=range b.assets {
		a,ok:=p.Asset(id)
		if !ok || a!=source {return result,fmt.Errorf("clipboard source unavailable or changed: %s",source.Path)}
	}
	q:=p.Clone()
	if q.Tracks[ti].Kind=="audio" {q.Version=Version}
	first:=len(q.Tracks[ti].Clips)
	for i,c:=range b.clips {
		c=cloneClipboardClip(c)
		c.ID=ids[i]
		c.Start+=at
		q.Tracks[ti].Clips=append(q.Tracks[ti].Clips,c)
	}
	if err:=q.Validate();err!=nil {return result,err}
	return ClipboardPlacement{Project:q,FirstClip:first,IDs:append([]string(nil),ids...),Start:at},nil
}

func (p Project) DuplicateClips(trackID string,b ClipClipboard,ids []string) (ClipboardPlacement,error) {
	if b.Count()==0 {return ClipboardPlacement{Project:p,FirstClip:-1},errors.New("select clips to duplicate")}
	r,err:=p.PasteClips(trackID,b,b.OriginalEnd(),ids)
	if err==nil {return r,nil}
	end:=int64(0)
	for _,t:=range p.Tracks {
		if t.ID!=trackID {continue}
		for _,c:=range t.Clips {
			if edge,ok:=clipEdge(c,true);ok {end=max(end,edge)}
		}
	}
	if end<=b.OriginalEnd() {return r,err}
	return p.PasteClips(trackID,b,end,ids)
}
