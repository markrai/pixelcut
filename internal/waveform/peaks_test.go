package waveform

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestStereoPeaks(t *testing.T) {
	var pcm bytes.Buffer
	for i:=0;i<800;i++ {
		l,r:=int16(0),int16(0)
		if i>=160 && i<240 {l,r=20000,-20000}
		if i>=480 && i<560 {r=-32768}
		binary.Write(&pcm,binary.LittleEndian,l)
		binary.Write(&pcm,binary.LittleEndian,r)
	}
	p,err:=ReadPCM(&pcm,100000)
	if err!=nil {t.Fatal(err)}
	for _,tc:=range []struct{start,end int64;want uint16}{
		{0,20000,0},{20000,30000,20000},{0,60000,20000},{60000,70000,32768},{0,100000,32768},{70000,100000,0},{100000,110000,0},
	} {
		if got:=p.Peak(tc.start,tc.end);got!=tc.want {t.Fatalf("%d–%d: got %d want %d",tc.start,tc.end,got,tc.want)}
	}
}

func TestWaveformBounds(t *testing.T) {
	if _,err:=ReadPCM(bytes.NewReader(nil),MaxDuration+1);err==nil {t.Fatal("unbounded duration accepted")}
	if _,err:=ReadPCM(bytes.NewReader([]byte{1,2,3}),10000);err==nil {t.Fatal("partial frame accepted")}
	p,err:=ReadPCM(bytes.NewReader([]byte{0,0,0,0}),MaxDuration)
	if err!=nil || len(p.tree)>maxBins*2 {t.Fatalf("unexpected allocation: %v",err)}
}
