package thumbnail

import "math"

const Height=32

func Width(w,h int) int {
	if w<=0 || h<=0 {return 48}
	return int(math.Max(24,math.Min(96,math.Round(float64(w)/float64(h)*Height/2)*2)))
}

type Tile struct {
	Left,Right int
	FullLeft,FullRight int
	Source int64
	Start,End int64
}

func Tiles(left,right int,clipLeft,scale float64,in,out int64,width int) []Tile {
	if right<=left || width<=0 || scale<=0 || math.IsNaN(scale) || math.IsInf(scale,0) || math.IsNaN(clipLeft) || math.IsInf(clipLeft,0) || in<0 || out<=in {return nil}
	step:=float64(width)/scale*1e6
	bucket:=math.Max(1000,math.Min(float64(int64(1)<<62),math.Pow(2,math.Round(math.Log2(step)))))
	span:=int64(bucket)
	visible:=float64(in)+(float64(left)-clipLeft)/scale*1e6
	start:=in
	if visible>=float64(out) {return nil}
	if visible>float64(in) {start=int64(visible)}
	grid:=start/span*span
	result:=make([]Tile,0)
	for grid<out && len(result)<96 {
		end:=out
		if grid<=math.MaxInt64-span && grid+span<end {end=grid+span}
		sample:=max(in,grid)
		x:=clipLeft+float64(grid-in)/1e6*scale
		nextX:=clipLeft+(float64(grid-in)+float64(span))/1e6*scale
		if x>=float64(right)+1 {break}
		l:=max(left,int(math.Round(math.Max(float64(left-1),math.Min(float64(right+1),x)))))
		r:=min(right,int(math.Round(math.Max(float64(left-1),math.Min(float64(right+1),nextX)))))
		if r>l {
			fl:=int(math.Round(math.Max(float64(left-2*width),math.Min(float64(right+2*width),x))))
			fr:=int(math.Round(math.Max(float64(left-2*width),math.Min(float64(right+2*width),nextX))))
			result=append(result,Tile{Left:l,Right:r,FullLeft:fl,FullRight:fr,Source:sample,Start:sample,End:end})
		}
		if end==out {break}
		grid=end
	}
	return result
}
