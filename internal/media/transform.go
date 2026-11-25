package media

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

type Transform struct {
	CanvasWidth int `json:"canvas_width"`
	CanvasHeight int `json:"canvas_height"`
	Scale int `json:"scale_percent"`
	X int `json:"x"`
	Y int `json:"y"`
}

type Placement struct {
	ScaledWidth, ScaledHeight int
	SourceX, SourceY, Width, Height int
	DestX, DestY int
}

func evenRound(v float64) int { return int(math.Round(v/2))*2 }

func (t Transform) Placement(w,h int) (Placement,error) {
	var p Placement
	if w<2 || h<2 || w>16384 || h>16384 || w%2!=0 || h%2!=0 ||
		t.CanvasWidth<2 || t.CanvasHeight<2 || t.CanvasWidth>8192 || t.CanvasHeight>8192 ||
		t.CanvasWidth%2!=0 || t.CanvasHeight%2!=0 || t.Scale<10 || t.Scale>400 ||
		t.X < -32768 || t.X>32768 || t.Y < -32768 || t.Y>32768 || t.X%2!=0 || t.Y%2!=0 {
		return p,errors.New("transform requires even dimensions and position, canvas 2–8192 px, scale 10–400%, and bounded offsets")
	}
	fit:=math.Min(float64(t.CanvasWidth)/float64(w),float64(t.CanvasHeight)/float64(h))
	factor:=fit*float64(t.Scale)/100
	sw,sh:=max(2,evenRound(float64(w)*factor)),max(2,evenRound(float64(h)*factor))
	if sw>16384 || sh>16384 || int64(sw)*int64(sh)>67108864 || int64(t.CanvasWidth)*int64(t.CanvasHeight)>33554432 {
		return p,errors.New("transform exceeds frame allocation budget")
	}
	x,y:=evenRound(float64(t.CanvasWidth-sw)/2)+t.X,evenRound(float64(t.CanvasHeight-sh)/2)+t.Y
	l,top:=max(0,x),max(0,y)
	r,b:=min(t.CanvasWidth,x+sw),min(t.CanvasHeight,y+sh)
	if r<=l || b<=top {return p,errors.New("transformed image must intersect the canvas")}
	return Placement{ScaledWidth:sw,ScaledHeight:sh,SourceX:l-x,SourceY:top-y,Width:r-l,Height:b-top,DestX:l,DestY:top},nil
}

func VideoFilters(width,height int,crop *Crop,t *Transform) (string,int,int,error) {
	w,h:=width,height
	filters:=[]string{}
	if crop!=nil {
		if err:=crop.Validate(Stream{Width:w,Height:h});err!=nil {return "",0,0,err}
		filters=append(filters,fmt.Sprintf("crop=%d:%d:%d:%d",crop.Width,crop.Height,crop.X,crop.Y))
		w,h=crop.Width,crop.Height
	}
	if t!=nil {
		p,err:=t.Placement(w,h)
		if err!=nil {return "",0,0,err}
		if p.ScaledWidth!=w || p.ScaledHeight!=h {
			filters=append(filters,fmt.Sprintf("scale=%d:%d:flags=lanczos",p.ScaledWidth,p.ScaledHeight))
		}
		if p.SourceX!=0 || p.SourceY!=0 || p.Width!=p.ScaledWidth || p.Height!=p.ScaledHeight {
			filters=append(filters,fmt.Sprintf("crop=%d:%d:%d:%d",p.Width,p.Height,p.SourceX,p.SourceY))
		}
		if p.Width!=t.CanvasWidth || p.Height!=t.CanvasHeight || p.DestX!=0 || p.DestY!=0 {
			filters=append(filters,fmt.Sprintf("pad=%d:%d:%d:%d:color=black",t.CanvasWidth,t.CanvasHeight,p.DestX,p.DestY))
		}
		filters=append(filters,"setsar=1")
		w,h=t.CanvasWidth,t.CanvasHeight
	}
	return strings.Join(filters,","),w,h,nil
}

func PreviewFilters(graph string, rotation int) (string,error) {
	if graph=="" {return "",nil}
	switch rotation {
	case 0:
	case 90: graph="transpose=clock,"+graph
	case 180: graph="hflip,vflip,"+graph
	case 270: graph="transpose=cclock,"+graph
	default: return "",errors.New("unsupported source rotation")
	}
	return "lavfi=["+graph+"],format=rotate=0",nil
}
