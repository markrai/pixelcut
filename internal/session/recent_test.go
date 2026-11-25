package session

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRecentProjects(t *testing.T) {
	dir:=t.TempDir()
	var paths []string
	for i:=0;i<20;i++ {paths=TouchRecent(paths,filepath.Join(dir,fmt.Sprintf("%d.pixelcut.json",i)))}
	if len(paths)!=MaxRecent || filepath.Base(paths[0])!="19.pixelcut.json" {t.Fatal(paths)}
	path:=paths[4]
	paths=TouchRecent(paths,path)
	if paths[0]!=path || len(paths)!=MaxRecent {t.Fatal(paths)}
	if err:=SaveRecent(dir,paths);err!=nil {t.Fatal(err)}
	got,err:=LoadRecent(dir)
	if err!=nil || !reflect.DeepEqual(got,paths) {t.Fatalf("%v %v",got,err)}
	if err=SaveRecent(dir,nil);err!=nil {t.Fatal(err)}
	got,err=LoadRecent(dir)
	if err!=nil || len(got)!=0 {t.Fatal("clear failed")}
	if err=os.WriteFile(filepath.Join(dir,"recent.json"),[]byte("[\"relative.pixelcut.json\"]"),0600);err!=nil {t.Fatal(err)}
	if _,err=LoadRecent(dir);err==nil {t.Fatal("invalid list accepted")}
}
