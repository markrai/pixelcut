package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func fixture(t *testing.T) State {
	t.Helper()
	p:=project.New()
	p.Assets=append(p.Assets,project.Asset{ID:"a",Path:filepath.Join(t.TempDir(),"missing-source.mp4"),Duration:10000000,Width:1080,Height:1920,Range:"tv",HasAudio:true})
	p.Tracks[0].Clips=append(p.Tracks[0].Clips,project.Clip{ID:"c",AssetID:"a",In:1000000,Out:9000000,Crop:&project.Crop{X:20,Y:40,Width:1000,Height:1800},Transform:&media.Transform{CanvasWidth:1000,CanvasHeight:1800,Scale:100,Rotation:180,FlipHorizontal:true}})
	return State{Project:p,Asset:0,Clip:0,Playhead:2000000,Origin:500000,Zoom:42,Dirty:true,ProjectPath:filepath.Join(t.TempDir(),"original.pixelcut.json")}
}

func TestRecoveryRoundTripAndFallback(t *testing.T) {
	dir:=t.TempDir()
	s,got,err:=Open(dir)
	if err!=nil || got!=nil {t.Fatalf("new store: %v",err)}
	first:=fixture(t)
	if err=s.Save(first);err!=nil {t.Fatal(err)}
	second:=first
	second.Playhead=3000000
	if err=s.Save(second);err!=nil {t.Fatal(err)}
	if err=s.Save(second);err!=nil || s.sequence!=2 {t.Fatal("unchanged state rewrote snapshot")}
	_,got,err=Open(dir)
	if err!=nil || got==nil || !reflect.DeepEqual(*got,second) {t.Fatalf("round trip: %v %+v",err,got)}
	if err=os.WriteFile(slot(dir,s.active),[]byte("{broken"),0600);err!=nil {t.Fatal(err)}
	fallback,got,err:=Open(dir)
	if err!=nil || got==nil || !reflect.DeepEqual(*got,first) {t.Fatalf("fallback: %v %+v",err,got)}
	third:=first
	third.Dirty=false
	if err=fallback.Save(third);err!=nil {t.Fatal(err)}
	_,got,err=Open(dir)
	if err!=nil || !reflect.DeepEqual(*got,third) {t.Fatalf("repair: %v %+v",err,got)}
}

func TestRecoveryRejectsInvalidState(t *testing.T) {
	s,_,err:=Open(t.TempDir())
	if err!=nil {t.Fatal(err)}
	valid:=fixture(t)
	if err=s.Save(valid);err!=nil {t.Fatal(err)}
	invalid:=valid
	invalid.Clip=20
	if err=s.Save(invalid);err==nil {t.Fatal("invalid selection accepted")}
	invalid=valid
	invalid.Project=project.Project{Version:project.Version}
	if err=s.Save(invalid);err==nil {t.Fatal("unsupported track layout accepted")}
	_,got,err:=Open(s.dir)
	if err!=nil || !reflect.DeepEqual(*got,valid) {t.Fatal("valid recovery was damaged")}
}

func TestRecoveryFailedWriteKeepsLastSnapshot(t *testing.T) {
	s,_,err:=Open(t.TempDir())
	if err!=nil {t.Fatal(err)}
	first:=fixture(t)
	if err=s.Save(first);err!=nil {t.Fatal(err)}
	next:=slot(s.dir,1)
	if err=os.Mkdir(next,0700);err!=nil {t.Fatal(err)}
	changed:=first
	changed.Playhead++
	if err=s.Save(changed);err==nil {t.Fatal("unexpected successful replacement")}
	_,got,err:=Open(s.dir)
	if err!=nil || !reflect.DeepEqual(*got,first) {t.Fatal("failed save destroyed latest valid state")}
}

func TestRecoveryBothCorrupt(t *testing.T) {
	dir:=t.TempDir()
	for i:=0;i<2;i++ {if err:=os.WriteFile(slot(dir,i),[]byte("invalid"),0600);err!=nil {t.Fatal(err)}}
	if s,state,err:=Open(dir);err==nil || s!=nil || state!=nil {t.Fatal("corrupt state silently accepted")}
}

func TestGroupSelectionRecovery(t *testing.T) {
	s,_,err:=Open(t.TempDir())
	if err!=nil {t.Fatal(err)}
	state:=fixture(t)
	state.Project.Tracks[0].Clips=append(state.Project.Tracks[0].Clips,project.Clip{ID:"second",AssetID:"a",Start:9000000,Out:1000000})
	state.SelectedClips=[]string{"c","second"}
	if err=s.Save(state);err!=nil {t.Fatal(err)}
	_,got,err:=Open(s.dir)
	if err!=nil || got==nil || !reflect.DeepEqual(*got,state) {t.Fatalf("group recovery: %+v %v",got,err)}
	state.SelectedClips=[]string{"c","missing"}
	if err=s.Save(state);err==nil {t.Fatal("invalid group saved")}
}

func TestSaveBaselineRecovery(t *testing.T) {
	s,_,err:=Open(t.TempDir())
	if err!=nil {t.Fatal(err)}
	state:=fixture(t)
	state.ProjectRevision=state.Project.Fingerprint()
	state.SavedFingerprint=state.Project.Fingerprint()
	if err=s.Save(state);err!=nil {t.Fatal(err)}
	_,got,err:=Open(s.dir)
	if err!=nil || got==nil || !reflect.DeepEqual(*got,state) {t.Fatalf("save baseline recovery: %+v %v",got,err)}
	state.ProjectRevision="invalid"
	if err=s.Save(state);err==nil {t.Fatal("invalid revision accepted")}
}
