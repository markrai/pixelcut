package waveform

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	SampleRate = 8000
	TicksPerSecond int64 = 1000000
	MaxDuration int64 = 8*60*60*TicksPerSecond
	maxBins = 65536
)

type Peaks struct {
	framesPerBin int64
	frames int64
	size int
	tree []uint16
}

func ReadPCM(r io.Reader,duration int64) (*Peaks,error) {
	if duration<=0 || duration>MaxDuration {return nil,errors.New("waveform duration outside supported range")}
	frames:=(duration*SampleRate+TicksPerSecond-1)/TicksPerSecond
	step:=max(int64(80),(frames+maxBins-1)/maxBins)
	bins:=int((frames+step-1)/step)
	size:=1
	for size<bins {size*=2}
	p:=&Peaks{framesPerBin:step,frames:frames,size:size,tree:make([]uint16,size*2)}
	buf:=make([]byte,32768)
	var offset int64
	for offset<frames {
		n:=int(min(int64(len(buf)),(frames-offset)*4))
		n,err:=io.ReadFull(r,buf[:n])
		if n%4!=0 {return nil,errors.New("incomplete stereo PCM frame")}
		for i:=0;i<n;i+=4 {
			l:=int32(int16(binary.LittleEndian.Uint16(buf[i:])))
			r:=int32(int16(binary.LittleEndian.Uint16(buf[i+2:])))
			if l<0 {l=-l}
			if r<0 {r=-r}
			index:=size+int(offset/step)
			p.tree[index]=max(p.tree[index],uint16(max(l,r)))
			offset++
		}
		if err==io.EOF || err==io.ErrUnexpectedEOF {break}
		if err!=nil {return nil,err}
	}
	if offset==0 {return nil,errors.New("no audio samples decoded")}
	for i:=size-1;i>0;i-- {p.tree[i]=max(p.tree[i*2],p.tree[i*2+1])}
	return p,nil
}

func (p *Peaks) Peak(start,end int64) uint16 {
	if p==nil || end<=start || end<=0 || start>=MaxDuration {return 0}
	start=max(0,start)
	end=min(end,MaxDuration)
	first:=start*SampleRate/TicksPerSecond
	last:=(end*SampleRate+TicksPerSecond-1)/TicksPerSecond
	if first>=p.frames {return 0}
	last=min(last,p.frames)
	l:=p.size+int(first/p.framesPerBin)
	r:=p.size+int((last+p.framesPerBin-1)/p.framesPerBin)
	var peak uint16
	for l<r {
		if l%2!=0 {peak=max(peak,p.tree[l]);l++}
		if r%2!=0 {r--;peak=max(peak,p.tree[r])}
		l/=2;r/=2
	}
	return peak
}
