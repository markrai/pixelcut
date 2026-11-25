package project

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func candidateFile(t *testing.T,dir,name string) string {
	t.Helper()
	path:=filepath.Join(dir,name)
	if err:=os.WriteFile(path,[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
	return path
}

func TestRelinkPreservesClips(t *testing.T) {
	p:=groupFixture(t)
	p.Assets[0].Range="tv"
	p.Tracks[0].Clips[0].Transform=&media.Transform{CanvasWidth:1080,CanvasHeight:1920,Scale:100}
	before:=p.Clone()
	a:=p.Assets[0]
	a.Path=candidateFile(t,t.TempDir(),"moved.mp4")
	a.ID="new"
	q,err:=p.RelinkAsset("a",a,false)
	if err!=nil {t.Fatal(err)}
	if q.Assets[0].ID!="a" || q.Assets[0].Path!=a.Path || !reflect.DeepEqual(q.Tracks,p.Tracks) || !reflect.DeepEqual(p,before) {t.Fatal("relink changed clips or original")}
	for _,change:=range []func(*Asset){
		func(a *Asset){a.Duration--},
		func(a *Asset){a.Width,a.Height=a.Height,a.Width},
		func(a *Asset){a.HasAudio=false},
		func(a *Asset){a.Range="pc"},
		func(a *Asset){a.Range=""},
		func(a *Asset){a.Path=filepath.Join(t.TempDir(),"missing.mp4")},
	} {
		b:=a;change(&b)
		got,err:=p.RelinkAsset("a",b,false)
		if err==nil || !reflect.DeepEqual(got,before) {t.Fatalf("incompatible candidate accepted: %+v",b)}
	}
	a.Duration=8000000
	q,err=p.RelinkAsset("a",a,true)
	if err!=nil || !reflect.DeepEqual(q.Tracks,p.Tracks) {t.Fatalf("compatible replacement: %v",err)}
	a.Duration=1000000
	if _,err=p.RelinkAsset("a",a,true);err==nil {t.Fatal("replacement invalidated existing trim")}
}

func TestRelinkRejectsDuplicateAndOnline(t *testing.T) {
	p:=fixture(t)
	a:=p.Assets[0]
	a.Range="tv"
	a.Path=candidateFile(t,t.TempDir(),"candidate.mp4")
	other:=a;other.ID="other"
	p.Assets=append(p.Assets,other)
	if _,err:=p.RelinkAsset("a",a,false);err==nil {t.Fatal("duplicate accepted")}
	p.Assets=p.Assets[:1]
	if err:=os.WriteFile(p.Assets[0].Path,[]byte("original"),0600);err!=nil {t.Fatal(err)}
	if _,err:=p.RelinkAsset("a",a,false);err==nil {t.Fatal("online source relinked without replacement")}
	if _,err:=p.RelinkAsset("a",a,true);err!=nil {t.Fatal(err)}
}

func TestFolderRelinkCandidates(t *testing.T) {
	p:=fixture(t)
	dir:=t.TempDir()
	a:=p.Assets[0]
	found:=candidateFile(t,dir,"SOURCE.MP4")
	b:=a;b.ID="b";b.Path=filepath.Join(t.TempDir(),"absent.mp4")
	c:=a;c.ID="c2";c.Path=filepath.Join(t.TempDir(),"ambiguous.mp4")
	d:=c;d.ID="d";d.Path=filepath.Join(t.TempDir(),"AMBIGUOUS.MP4")
	e:=a;e.ID="e";e.Path=candidateFile(t,t.TempDir(),"online.mp4")
	candidateFile(t,dir,"ambiguous.mp4")
	p.Assets=append(p.Assets,b,c,d,e)
	got,issues,err:=p.FindRelinkCandidates(context.Background(),dir)
	if err!=nil || len(got)!=1 || got[0]!=(RelinkCandidate{"a",found}) || len(issues)!=3 {t.Fatalf("candidates: %+v issues: %+v %v",got,issues,err)}
	ctx,cancel:=context.WithCancel(context.Background());cancel()
	if _,_,err=p.FindRelinkCandidates(ctx,dir);err==nil {t.Fatal("cancellation ignored")}
}
