package project

import (
	"reflect"
	"testing"
)

func TestRippleDelete(t *testing.T) {
	p:=groupFixture(t)
	before:=p.Clone()
	q,err:=p.RippleDelete("V1",[]string{"c","b"})
	if err!=nil || len(q.Tracks[0].Clips)!=1 || q.Tracks[0].Clips[0].Start!=6000000 {t.Fatalf("delete: %+v %v",q,err)}
	if len(q.Assets)!=len(p.Assets) {t.Fatal("source removed")}
	q,err=p.RippleDelete("V1",[]string{"b"})
	if err!=nil || q.Tracks[0].Clips[0].Start!=1000000 || q.Tracks[0].Clips[1].Start!=11000000 {t.Fatal("unrelated gaps changed")}
	if _,err=p.RippleDelete("V1",[]string{"missing"});err==nil {t.Fatal("missing selection accepted")}
	if !reflect.DeepEqual(p,before) {t.Fatal("source mutated")}
}

func TestRemoveGap(t *testing.T) {
	p:=groupFixture(t)
	q,n,err:=p.RemoveGap("V1",7000000)
	if err!=nil || n!=2000000 || q.Tracks[0].Clips[1].Start!=6000000 || q.Tracks[0].Clips[2].Start!=10000000 {t.Fatalf("gap: %d %+v %v",n,q,err)}
	q,n,err=p.RemoveGap("V1",0)
	if err!=nil || n!=1000000 || q.Tracks[0].Clips[0].Start!=0 {t.Fatal("leading gap")}
	for _,at:=range []int64{1000000,8000000,13000000} {
		if _,_,err=p.RemoveGap("V1",at);err==nil {t.Fatalf("invalid gap %d",at)}
	}
}

func TestRippleTrim(t *testing.T) {
	p:=groupFixture(t)
	before:=p.Clone()
	for _,tc:=range []struct{mode string;delta,in,out,shift int64}{
		{"left",1000000,3000000,7000000,-1000000},
		{"left",-1000000,1000000,7000000,1000000},
		{"right",-1000000,2000000,6000000,-1000000},
		{"right",3000000,2000000,10000000,3000000},
	} {
		r,err:=p.RippleTrim("V1","c",tc.mode,tc.delta,-1,0)
		if err!=nil {t.Fatal(err)}
		c:=r.Project.Tracks[0].Clips[0]
		if c.Start!=1000000 || c.In!=tc.in || c.Out!=tc.out || r.Project.Tracks[0].Clips[1].Start!=8000000+tc.shift || r.Project.Tracks[0].Clips[2].Start!=12000000+tc.shift {t.Fatalf("%+v: %+v",tc,r)}
		if !reflect.DeepEqual(c.Crop,before.Tracks[0].Clips[0].Crop) {t.Fatal("crop changed")}
	}
	r,err:=p.RippleTrim("V1","c","right",-900000,5000000,200000)
	if err!=nil || !r.Snapped || r.Target!=5000000 || r.Delta!=-1000000 {t.Fatalf("snap: %+v %v",r,err)}
	r,err=p.RippleTrim("V1","c","left",90000000,-1,0)
	if err!=nil || r.Project.Tracks[0].Clips[0].Out-r.Project.Tracks[0].Clips[0].In!=1 {t.Fatal("empty clip permitted")}
	if !reflect.DeepEqual(p,before) {t.Fatal("source mutated")}
}
