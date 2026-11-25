package project

import (
	"errors"
	"fmt"
)

const MaxTracks=32

func (p Project) TrackIndex(id string) int {
	for i,t:=range p.Tracks {if t.ID==id {return i}}
	return -1
}

func (p Project) EndTime() int64 {
	var end int64
	for _,t:=range p.Tracks {
		for _,c:=range t.Clips {
			if n,ok:=clipEdge(c,true);ok {end=max(end,n)}
		}
	}
	return end
}

func (p Project) ValidateEditorLayout() error {
	if err:=p.Validate();err!=nil {return err}
	if len(p.Tracks)<2 || len(p.Tracks)>MaxTracks || p.Tracks[0].ID!="V1" || p.Tracks[0].Kind!="video" || p.Tracks[1].ID!="A1" || p.Tracks[1].Kind!="audio" {return errors.New("unsupported editor track layout")}
	if p.Version==LegacyVersion && (len(p.Tracks)!=2 || len(p.Tracks[1].Clips)!=0) {return errors.New("legacy editor projects require V1 and empty A1")}
	return nil
}

func (p Project) AddTrack(kind string) (Project,string,error) {
	if err:=p.ValidateEditorLayout();err!=nil {return p,"",err}
	if len(p.Tracks)>=MaxTracks {return p,"",errors.New("track limit reached")}
	prefix:="V"
	if kind=="audio" {prefix="A"} else if kind!="video" {return p,"",errors.New("invalid track kind")}
	id:=""
	for i:=2;i<=MaxTracks+1;i++ {
		candidate:=fmt.Sprintf("%s%d",prefix,i)
		if p.TrackIndex(candidate)<0 {id=candidate;break}
	}
	if id=="" {return p,"",errors.New("no free track ID")}
	q:=p.Clone()
	q.Version=Version
	q.Tracks=append(q.Tracks,Track{ID:id,Kind:kind,Clips:[]Clip{}})
	return q,id,q.ValidateEditorLayout()
}

func (p Project) RemoveTrack(id string) (Project,error) {
	if err:=p.ValidateEditorLayout();err!=nil {return p,err}
	i:=p.TrackIndex(id)
	if i<2 {return p,errors.New("base tracks V1 and A1 cannot be removed")}
	if len(p.Tracks[i].Clips)!=0 {return p,errors.New("remove or move the track's clips first")}
	q:=p.Clone()
	q.Tracks=append(q.Tracks[:i],q.Tracks[i+1:]...)
	return q,q.ValidateEditorLayout()
}

func (p Project) TransferClips(from,to string,ids []string) (Project,error) {
	if err:=p.ValidateEditorLayout();err!=nil {return p,err}
	src,selected,err:=p.selectedTrack(from,ids)
	if err!=nil {return p,err}
	dst:=p.TrackIndex(to)
	if dst<0 || src==dst || p.Tracks[src].Kind!=p.Tracks[dst].Kind {return p,errors.New("select a different track of the same kind")}
	q:=p.Clone()
	q.Version=Version
	kept:=q.Tracks[src].Clips[:0]
	for _,c:=range q.Tracks[src].Clips {
		if selected[c.ID] {q.Tracks[dst].Clips=append(q.Tracks[dst].Clips,c)} else {kept=append(kept,c)}
	}
	q.Tracks[src].Clips=kept
	if err=q.ValidateEditorLayout();err!=nil {return p,err}
	return q,nil
}
