package thumbnail

import "math"

const Height=32

func Width(w,h int) int {
	if w<=0 || h<=0 {return 48}
	return int(math.Max(24,math.Min(96,math.Round(float64(w)/float64(h)*Height/2)*2)))
}

type Tile struct {
	Left,Right int
	FullLeft int
	Source int64
}

func Tiles(left,right int,clipLeft,scale float64,in,out int64,width int) []Tile {
	if right<=left || width<=0 || scale<=0 || math.IsNaN(scale) || math.IsInf(scale,0) || math.IsNaN(clipLeft) || math.IsInf(clipLeft,0) || in<0 || out<=in {return nil}
	start:=math.Floor((float64(left)-clipLeft)/float64(width))
	if start<0 {start=0}
	x:=clipLeft+start*float64(width)
	if x>float64(right) || x<float64(left-width-1) {return nil}
	step:=float64(width)/scale*1e6
	bucket:=math.Max(1000,math.Pow(2,math.Floor(math.Log2(step))))
	result:=make([]Tile,0)
	for len(result)<96 && x<float64(right) {
		l,r:=max(left,int(math.Round(x))),min(right,int(math.Round(x+float64(width))))
		if r>l {
			tileStart:=math.Max(0,(x-clipLeft)/scale*1e6)
			tileEnd:=math.Min(float64(out-in),(x+float64(width)-clipLeft)/scale*1e6)
			offset:=(tileStart+math.Max(tileStart,tileEnd))/2
			t:=float64(in)+math.Max(0,offset)
			t=math.Floor(t/bucket)*bucket
			sample:=in
			if t>=float64(out) {sample=out-1} else if t>float64(in) {sample=int64(t)}
			result=append(result,Tile{Left:l,Right:r,FullLeft:int(math.Round(x)),Source:sample})
		}
		next:=x+float64(width)
		if next<=x {break}
		x=next
	}
	return result
}
