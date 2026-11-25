package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWaveformDelayedAudio(t *testing.T) {
	if _,err:=exec.LookPath("ffmpeg");err!=nil {t.Skip("ffmpeg unavailable")}
	src:=filepath.Join(t.TempDir(),"delayed.mp4")
	runFF(t,"-v","error","-f","lavfi","-i","color=size=64x64:rate=10:duration=1","-itsoffset","0.25","-f","lavfi","-i","sine=frequency=440:sample_rate=48000:duration=0.5","-map","0:v","-map","1:a","-c:v","libx264","-c:a","aac",src)
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
	defer cancel()
	p,err:=Default().Waveform(ctx,src,1000000)
	if err!=nil {t.Fatal(err)}
	if got:=p.Peak(0,100000);got!=0 {t.Fatalf("leading silence missing: %d",got)}
	if got:=p.Peak(350000,450000);got<1000 {t.Fatalf("delayed tone missing: %d",got)}
	if got:=p.Peak(850000,950000);got!=0 {t.Fatalf("trailing silence missing: %d",got)}
}
