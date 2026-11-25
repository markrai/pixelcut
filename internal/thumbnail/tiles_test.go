package thumbnail

import "testing"

func TestTiles(t *testing.T) {
	a:=Tiles(0,120,0,40,2000000,5000000,40)
	if len(a)<2 || a[0].Left!=0 || a[len(a)-1].Right!=120 {t.Fatalf("unexpected tiles %+v",a)}
	for i,v:=range a {
		if v.Source<2000000 || v.Source>=5000000 || v.End<=v.Start {t.Fatalf("source outside trim %+v",v)}
		if i>0 && (v.Source<=a[i-1].Source || v.Left!=a[i-1].Right) {t.Fatal("noncontiguous tiles")}
	}
	b:=Tiles(20,100,-20,40,2000000,5000000,40)
	if len(b)<2 || b[0].Left!=20 || b[len(b)-1].Right!=100 {t.Fatalf("scrolled tiles %+v",b)}
	for _,tile:=range b {
		found:=false
		for _,old:=range a {if tile.Source==old.Source {found=true}}
		if !found {t.Fatalf("scroll changed source key %+v",tile)}
	}
	if got:=Tiles(0,100000,0,40,0,1000000000,24);len(got)>96 {t.Fatal("unbounded tile demand")}
	if len(Tiles(0,100,0,0,0,100,24))!=0 {t.Fatal("invalid scale accepted")}
	if Width(1080,1920)!=24 || Width(1920,1080)!=56 {t.Fatal("incorrect thumbnail aspect")}
}

func TestTrimKeepsInteriorSourceKeys(t *testing.T) {
	original:=Tiles(0,400,0,40,0,10000000,40)
	for _,in:=range []int64{100000,500000,1500000,3200000} {
		trimmed:=Tiles(0,int(float64(10000000-in)*40/1e6),0,40,in,10000000,40)
		for _,tile:=range trimmed[1:] {
			found:=false
			for _,old:=range original {if old.Source==tile.Source {found=true}}
			if !found {t.Fatalf("left trim %d rebuilt interior key %d",in,tile.Source)}
		}
	}
	short:=Tiles(0,270,0,40,0,6750000,40)
	for i,v:=range short {if v.Source!=original[i].Source {t.Fatal("right trim changed source key")}}
}

func TestFitScaleWithinLevel(t *testing.T) {
	a:=Tiles(0,400,0,40,0,10000000,40)
	b:=Tiles(0,420,0,42,0,10000000,40)
	if len(a)!=len(b) {t.Fatal("small fit resize changed tile count")}
	for i,v:=range a {if v.Source!=b[i].Source {t.Fatal("small fit resize changed key")}}
}
