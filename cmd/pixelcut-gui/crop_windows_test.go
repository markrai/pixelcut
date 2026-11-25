package main

import (
	"encoding/json"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/media"
)

func TestCropRequest(t *testing.T) {
	a:=project.Asset{Width:1080,Height:1920}
	c:=newPreviewClock(project.Clip{ID:"clip",Start:0,In:0,Out:1000000},0,true)
	if n:=c.setCrop(a,nil,false);n!=1 {t.Fatal(n)}
	if got:=c.cropRequest.Load().crop;got!=(project.Crop{Width:1080,Height:1920}) {t.Fatal(got)}
	rect:=project.Crop{X:40,Y:80,Width:1000,Height:1760}
	if n:=c.setCrop(a,&rect,true);n!=2 {t.Fatal(n)}
	rect.X=42
	var payload struct {
		Serial uint64
		Width,Height int
		Crop project.Crop
		Edit bool
	}
	if err:=json.Unmarshal([]byte(c.cropRequest.Load().payload()),&payload);err!=nil {t.Fatal(err)}
	if payload.Serial!=2 || payload.Width!=1080 || payload.Height!=1920 || payload.Crop.X!=40 || !payload.Edit {t.Fatalf("unexpected request %+v",payload)}
}

func TestCropOnlyUsesSharedFilterPlan(t *testing.T) {
	a:=project.Asset{Width:1080,Height:1920,Range:"pc"}
	c:=newPreviewClock(project.Clip{In:0,Out:1000000},0,true)
	rect:=&project.Crop{X:20,Y:60,Width:800,Height:1500}
	if c.setComposition(a,rect,nil,false)==0 {t.Fatal("crop plan rejected")}
	req:=c.cropRequest.Load()
	if req.limitedOnly || len(req.filters)!=4 {t.Fatalf("incorrect crop plan: %+v",req)}
	for k,want:=range map[string]string{
		"0":"lavfi=[crop=800:1500:20:60],format=rotate=0",
		"90":"lavfi=[transpose=clock,crop=800:1500:20:60],format=rotate=0",
		"180":"lavfi=[hflip,vflip,crop=800:1500:20:60],format=rotate=0",
		"270":"lavfi=[transpose=cclock,crop=800:1500:20:60],format=rotate=0",
	} {
		if req.filters[k]!=want {t.Fatalf("rotation %s: %q",k,req.filters[k])}
	}
	if c.setCrop(a,rect,true)==0 || len(c.cropRequest.Load().filters)!=0 {t.Fatal("crop edit must show full source")}
	if c.setCrop(a,nil,false)==0 || len(c.cropRequest.Load().filters)!=0 {t.Fatal("reset must remove filters")}
	tr:=&media.Transform{CanvasWidth:800,CanvasHeight:1500,Scale:50}
	if c.setComposition(a,rect,tr,false)==0 || !c.cropRequest.Load().limitedOnly {t.Fatal("transform range guard lost")}
}
