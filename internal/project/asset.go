package project

import "errors"

func (p Project) AssetReferences(id string) int {
	n:=0
	for _,track:=range p.Tracks {
		for _,clip:=range track.Clips {if clip.AssetID==id {n++}}
	}
	return n
}

func (p Project) RemoveAsset(id string, removeClips bool) (Project,error) {
	if err:=p.Validate();err!=nil {return p,err}
	if _,ok:=p.Asset(id);!ok {return p,errors.New("asset not found")}
	if !removeClips && p.AssetReferences(id)>0 {return p,errors.New("asset is used by timeline clips")}
	q:=p.Clone()
	assets:=q.Assets[:0]
	for _,a:=range q.Assets {if a.ID!=id {assets=append(assets,a)}}
	q.Assets=assets
	for i:=range q.Tracks {
		clips:=q.Tracks[i].Clips[:0]
		for _,c:=range q.Tracks[i].Clips {if c.AssetID!=id {clips=append(clips,c)}}
		q.Tracks[i].Clips=clips
	}
	if err:=q.Validate();err!=nil {return p,err}
	return q,nil
}
