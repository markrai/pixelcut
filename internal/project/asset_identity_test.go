package project

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestImportAssetIdentity(t *testing.T) {
	dir:=t.TempDir()
	path:=filepath.Join(dir,"Source.mp4")
	if err:=os.WriteFile(path,[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
	a:=Asset{ID:"a",Path:path,Duration:1000000,Width:1080,Height:1920,Range:"tv"}
	p,index,added,err:=New().AddAsset(a)
	if err!=nil || !added || index!=0 {t.Fatalf("first import %d %t %v",index,added,err)}
	alias:=filepath.Join(dir,"alias.mp4")
	if err:=os.Link(path,alias);err!=nil {t.Fatal(err)}
	for _,name:=range []string{path,dir+string(os.PathSeparator)+"."+string(os.PathSeparator)+"Source.mp4",alias} {
		b:=a;b.ID="b";b.Path=name
		q,index,added,err:=p.AddAsset(b)
		if err!=nil || added || index!=0 || !reflect.DeepEqual(q,p) {t.Fatalf("duplicate %s: %d %t %v",name,index,added,err)}
	}
	if runtime.GOOS=="windows" {
		name:=filepath.Join(dir,"source.MP4")
		if _,err:=os.Stat(name);err==nil {
			if i,ok:=p.FindAssetPath(name);!ok || i!=0 {t.Fatal("case alias not recognized")}
		}
	}
	other:=filepath.Join(dir,"other","Source.mp4")
	if err:=os.MkdirAll(filepath.Dir(other),0700);err!=nil {t.Fatal(err)}
	if err:=os.WriteFile(other,[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
	b:=a;b.ID="b";b.Path=other
	q,index,added,err:=p.AddAsset(b)
	if err!=nil || !added || index!=1 || len(q.Assets)!=2 {t.Fatal("distinct file rejected")}
}

func TestMergeDuplicateAssetsPreservesClips(t *testing.T) {
	p:=fixture(t)
	b:=p.Assets[0];b.ID="duplicate"
	p.Assets=append(p.Assets,b)
	c:=p.Tracks[0].Clips[0];c.ID="second";c.AssetID=b.ID;c.Start=9000000
	p.Tracks[0].Clips=append(p.Tracks[0].Clips,c)
	before:=p.Clone()
	q,remap,err:=p.MergeDuplicateAssets()
	if err!=nil || len(q.Assets)!=1 || remap[b.ID]!=p.Assets[0].ID || len(q.Tracks[0].Clips)!=2 {t.Fatalf("merge failed: %+v %v",remap,err)}
	c.AssetID=p.Assets[0].ID
	if !reflect.DeepEqual(q.Tracks[0].Clips[1],c) || !reflect.DeepEqual(p,before) {t.Fatal("merge changed clip or source project")}
	p.Assets[1].Duration++
	q,remap,err=p.MergeDuplicateAssets()
	if err!=nil || len(remap)!=0 || len(q.Assets)!=2 {t.Fatal("conflicting metadata silently merged")}
}
