package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVariableFrameIndex(t *testing.T) {
	f,err:=readFrameIndex(strings.NewReader("frame|best_effort_timestamp_time=2.000000|side_data_type=SEI\nframe|best_effort_timestamp_time=2.041667\nframe|best_effort_timestamp_time=2.100000\nframe|best_effort_timestamp_time=2.120000\nformat|start_time=2.000000\n"))
	if err!=nil || f.Count()!=4 {t.Fatalf("index: %v %v",f,err)}
	for _,tc:=range []struct{at,lo,hi,want int64;forward,ok bool}{
		{0,0,200000,41667,true,true},
		{41667,0,200000,100000,true,true},
		{100000,0,200000,41667,false,true},
		{200000,0,120000,100000,false,true},
		{100000,50000,120000,0,true,false},
		{41667,50000,120000,0,false,false},
		{-1,0,200000,0,true,true},
	} {
		got,ok:=f.Step(tc.at,tc.lo,tc.hi,tc.forward)
		if got!=tc.want || ok!=tc.ok {t.Fatalf("%+v: %d %t",tc,got,ok)}
	}
	if v,ok:=f.Nearest(75000,0,200000);!ok || v!=100000 {t.Fatal("nearest VFR timestamp")}
	if v,ok:=f.Nearest(110000,0,200000);!ok || v!=100000 {t.Fatal("tie must favor earlier frame")}
	if _,ok:=f.Nearest(60000,50000,90000);ok {t.Fatal("out-of-range frame")}
}

func TestInvalidFrameIndex(t *testing.T) {
	for _,s:=range []string{
		"frame|best_effort_timestamp_time=N/A\nformat|start_time=0",
		"frame|best_effort_timestamp_time=0\n",
		"frame|best_effort_timestamp_time=0\nframe|best_effort_timestamp_time=0\nformat|start_time=0",
		"frame|side_data_type=SEI\nformat|start_time=0",
	} {
		if _,err:=readFrameIndex(strings.NewReader(s));err==nil {t.Fatalf("accepted %q",s)}
	}
}

func TestDecodedFrameProbe(t *testing.T) {
	e:=Default()
	if _,err:=exec.LookPath(e.FFmpeg);err!=nil {t.Skip("ffmpeg unavailable")}
	if _,err:=exec.LookPath(e.FFprobe);err!=nil {t.Skip("ffprobe unavailable")}
	ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second)
	defer cancel()
	path:=filepath.Join(t.TempDir(),"vfr.mp4")
	cmd:=exec.CommandContext(ctx,e.FFmpeg,"-v","error","-f","lavfi","-i","testsrc2=size=64x64:rate=25:duration=0.4","-vf","select='not(eq(n,2)+eq(n,3)+eq(n,6))'","-fps_mode","vfr","-c:v","mpeg4",path)
	if out,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("fixture: %v %s",err,out)}
	f,err:=e.Frames(ctx,path)
	if err!=nil {t.Fatal(err)}
	if f.Count()!=7 {t.Fatalf("got %d frames",f.Count())}
	if v,ok:=f.Step(40000,0,400000,true);!ok || v!=160000 {t.Fatalf("VFR step %d %t",v,ok)}
	if v,ok:=f.Step(160000,0,400000,false);!ok || v!=40000 {t.Fatalf("reverse step %d %t",v,ok)}
}
