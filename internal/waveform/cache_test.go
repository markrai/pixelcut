package waveform

import (
	"bytes"
	"testing"
)

func TestPeakCacheRoundTrip(t *testing.T) {
	p,err:=ReadPCM(bytes.NewReader([]byte{0,128,0,0,0,0,1,0}),100000)
	if err!=nil {t.Fatal(err)}
	b,err:=p.MarshalBinary()
	if err!=nil {t.Fatal(err)}
	q,err:=DecodePeaks(b,100000)
	if err!=nil || q.Peak(0,100000)!=32768 {t.Fatal("waveform cache lost peak")}
	if _,err=DecodePeaks(b,200000);err==nil {t.Fatal("wrong duration accepted")}
	b[17]=255
	if _,err=DecodePeaks(b,100000);err==nil {t.Fatal("invalid peak accepted")}
}
