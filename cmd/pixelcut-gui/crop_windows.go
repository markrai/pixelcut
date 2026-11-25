package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

//go:embed crop.lua
var cropScript string

type cropRequest struct {
	serial uint64
	width,height int
	crop project.Crop
	edit bool
	filters map[string]string
	limitedOnly bool
}

type cropSample struct {
	Serial uint64 `json:"serial"`
	Crop project.Crop `json:"crop"`
	Ready bool `json:"ready"`
	Dragging bool `json:"dragging"`
	Error string `json:"error"`
	Action string `json:"action"`
}

type cropSession struct {
	clipID string
	asset project.Asset
	serial uint64
}

func writeCropScript() (string,error) {
	f,err:=os.CreateTemp("","pixelcut-crop-*.lua")
	if err!=nil {return "",err}
	_,err=f.WriteString(cropScript)
	closeErr:=f.Close()
	if err==nil {err=closeErr}
	if err!=nil {os.Remove(f.Name());return "",err}
	return f.Name(),nil
}

func cropOrFull(a project.Asset,c *project.Crop) project.Crop {
	if c!=nil {return *c}
	return project.Crop{Width:a.Width,Height:a.Height}
}

func (r cropRequest) payload() string {
	b,_:=json.Marshal(map[string]any{"serial":r.serial,"width":r.width,"height":r.height,"crop":r.crop,"edit":r.edit,"filters":r.filters,"limited_only":r.limitedOnly})
	return string(b)
}

func (c *previewClock) setCrop(a project.Asset, crop *project.Crop, editing bool) uint64 {
	return c.setComposition(a,crop,nil,editing)
}

func (c *previewClock) setComposition(a project.Asset, crop *project.Crop, tr *media.Transform, editing bool) uint64 {
	var filters map[string]string
	if (tr!=nil || crop!=nil) && !editing {
		var mc *media.Crop
		if crop!=nil {mc=&media.Crop{X:crop.X,Y:crop.Y,Width:crop.Width,Height:crop.Height}}
		graph,_,_,err:=media.VideoFilters(a.Width,a.Height,mc,tr)
		if err!=nil {c.cropError.Store(true);return 0}
		filters=map[string]string{}
		for _,rotation:=range []int{0,90,180,270} {
			f,err:=media.PreviewFilters(graph,rotation)
			if err!=nil {c.cropError.Store(true);return 0}
			filters[fmt.Sprint(rotation)]=f
		}
	}
	serial:=uint64(1)
	if old:=c.cropRequest.Load();old!=nil {serial=old.serial+1}
	c.cropRequest.Store(&cropRequest{serial:serial,width:a.Width,height:a.Height,crop:cropOrFull(a,crop),edit:editing,filters:filters,limitedOnly:tr!=nil && !editing})
	return serial
}

func (e *editor) beginOrApplyCrop() error {
	if e.cropSession!=nil {return e.applyVisualCrop()}
	c,ok:=e.selected()
	if !ok {return errors.New("Select a video clip first")}
	a,ok:=e.p.Asset(c.AssetID)
	if !ok {return errors.New("Source missing")}
	if !project.SourceAvailable(a.Path) {return errors.New("Source offline; use Relink")}
	if e.clock==nil || !e.previewActive() || e.clock.clip.ID!=c.ID {
		if err:=e.startPreview(true);err!=nil {return err}
	}
	if err:=e.pausePreview();err!=nil {return err}
	e.cropSession=&cropSession{clipID:c.ID,asset:a,serial:e.clock.setCrop(a,c.Crop,true)}
	change(e.controls[idCropVisual],"Apply crop")
	change(e.controls[idCropReset],"Cancel crop")
	e.tell("Drag the crop edges or corners. Shift locks aspect ratio; Enter applies; Esc cancels.")
	return nil
}

func (e *editor) endVisualCropUI() {
	e.cropSession=nil
	change(e.controls[idCropVisual],"Visual crop")
	change(e.controls[idCropReset],"Reset crop")
}

func (e *editor) cancelVisualCrop() {
	if e.cropSession==nil {return}
	c,ok:=e.selected()
	if ok && e.clock!=nil {
		e.clock.setComposition(e.cropSession.asset,c.Crop,c.Transform,false)
	}
	e.endVisualCropUI()
	e.fields()
	e.tell("Crop canceled.")
}

func (e *editor) applyVisualCrop() error {
	s:=e.cropSession
	if s==nil || e.clock==nil {return errors.New("Crop preview unavailable")}
	state:=e.clock.cropSample.Load()
	if state==nil || state.Serial!=s.serial || !state.Ready || state.Dragging {return errors.New("Wait for the crop preview or finish dragging")}
	c,ok:=e.selected()
	if !ok || c.ID!=s.clipID {return errors.New("Clip selection changed")}
	crop:=state.Crop
	if crop==cropOrFull(s.asset,nil) {c.Crop=nil} else {c.Crop=&crop}
	q:=e.p.Clone()
	q.Tracks[e.trackIndex()].Clips[e.clip]=c
	if err:=q.Validate();err!=nil {return err}
	e.undo=append(e.undo,e.p.Clone())
	if len(e.undo)>50 {e.undo=e.undo[1:]}
	e.redo=nil
	e.p=q
	e.dirty=true
	e.clock.setComposition(s.asset,c.Crop,c.Transform,false)
	e.endVisualCropUI()
	e.refresh()
	e.tell("Crop applied. Preview and export use the selected rectangle.")
	return nil
}

func (e *editor) resetVisualCrop() error {
	if e.cropSession!=nil {e.cancelVisualCrop();return nil}
	c,ok:=e.selected()
	if !ok {return errors.New("Select a video clip first")}
	if c.Crop==nil {return nil}
	c.Crop=nil
	q:=e.p.Clone()
	q.Tracks[e.trackIndex()].Clips[e.clip]=c
	if err:=e.commit(q);err!=nil {return err}
	return e.startPreview(true)
}

func (e *editor) updateVisualCrop() {
	if e.clock==nil {return}
	if e.clock.cropError.Swap(false) {e.tell("Crop preview IPC failed.")}
	state:=e.clock.cropSample.Load()
	if state==nil || e.clock.cropRequest.Load()==nil || state.Serial!=e.clock.cropRequest.Load().serial {return}
	if state.Error!="" {e.tell(state.Error);return}
	s:=e.cropSession
	if s==nil || state.Serial!=s.serial || !state.Ready {return}
	c:=state.Crop
	value:=fmt.Sprintf("%d,%d,%d,%d",c.X,c.Y,c.Width,c.Height)
	if read(e.controls[idCrop])!=value {change(e.controls[idCrop],value)}
	switch state.Action {
	case "apply":
		if err:=e.applyVisualCrop();err!=nil {e.tell(err.Error())}
	case "cancel":
		e.cancelVisualCrop()
	}
}
