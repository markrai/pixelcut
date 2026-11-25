package main

import (
	"encoding/json"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
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
