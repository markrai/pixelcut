package thumbnail

import "testing"

func TestTiles(t *testing.T) {
	a:=Tiles(0,120,0,40,2000000,5000000,40)
	if len(a)!=3 || a[0].Left!=0 || a[2].Right!=120 {t.Fatalf("unexpected tiles %+v",a)}
	for i,v:=range a {
		if v.Source<2000000 || v.Source>=5000000 {t.Fatalf("source outside trim %+v",v)}
		if i>0 && v.Source<a[i-1].Source {t.Fatal("nonmonotonic source")}
	}
	b:=Tiles(20,100,-20,40,2000000,5000000,40)
	if len(b)!=2 || b[0].Left!=20 || b[1].Right!=100 || b[0].Source!=a[1].Source {t.Fatalf("scrolled tiles %+v",b)}
	if got:=Tiles(0,100000,0,40,0,1000000000,24);len(got)>96 {t.Fatal("unbounded tile demand")}
	if len(Tiles(0,100,0,0,0,100,24))!=0 {t.Fatal("invalid scale accepted")}
	if Width(1080,1920)!=24 || Width(1920,1080)!=56 {t.Fatal("incorrect thumbnail aspect")}
}
