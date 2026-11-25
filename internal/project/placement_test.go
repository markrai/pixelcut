package project

import (
	"math"
	"reflect"
	"testing"
)

func TestAssetPlacement(t *testing.T) {
	p:=fixture(t)
	before:=p.Clone()
	a:=p.Assets[0]
	a.ID="short"
	a.Duration=1000000
	p.Assets=append(p.Assets,a)
	r,err:=p.PlaceAssets("V1",[]string{"short"},[]string{"new"},5900000,-1,200000)
	if err!=nil || !r.Snapped || r.Target!=6000000 || r.Start!=6000000 || r.End!=7000000 || r.FirstClip!=1 {t.Fatalf("snap to adjacent clip: %+v %v",r,err)}
	if len(r.Project.Tracks[0].Clips)!=2 || r.Project.Tracks[0].Clips[1].AssetID!="short" {t.Fatal("missing placed clip")}
	if _,err=p.PlaceAssets("V1",[]string{"short"},[]string{"new"},5900000,-1,0);err==nil {t.Fatal("overlap accepted with snapping disabled")}
	r,err=p.PlaceAssets("V1",[]string{"short","short"},[]string{"new1","new2"},8000000,8100000,200000)
	if err!=nil || r.Start!=8100000 || r.Project.Tracks[0].Clips[2].Start!=9100000 || r.End!=10100000 {t.Fatalf("contiguous batch: %+v %v",r,err)}
	r,err=p.PlaceAssets("V1",[]string{"short"},[]string{"new"},100000,-1,200000)
	if err!=nil || r.Start!=0 || r.End!=1000000 || !r.Snapped {t.Fatalf("snap to leading gap: %+v %v",r,err)}
	if _,err=p.PlaceAssets("V1",[]string{"short"},[]string{"c"},8000000,-1,0);err==nil {t.Fatal("duplicate clip ID accepted")}
	if _,err=p.PlaceAssets("V1",[]string{"missing"},[]string{"new"},0,0,1);err==nil {t.Fatal("missing asset accepted")}
	if !reflect.DeepEqual(p.Tracks,before.Tracks) {t.Fatal("placement mutated source")}
	r,err=p.PlaceAssets("V1",[]string{"short"},[]string{"edge"},math.MaxInt64,0,0)
	if err!=nil || r.End!=math.MaxInt64 {t.Fatalf("overflow guard: %+v %v",r,err)}
}
