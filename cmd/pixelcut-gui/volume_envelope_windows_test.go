package main

import (
	"testing"

	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/timeline"
)

func TestVolumePointEditing(t *testing.T) {
	c := project.Clip{Start: 1000000, In: 2000000, Out: 6000000}
	view := timeline.View{Width: 1000, Zoom: 100}
	points, index, ok := insertVolumePoint(c, nil, 4000000, 25)
	if !ok || index != 1 || len(points) != 3 || points[0].Percent != 100 || points[2].Percent != 100 {
		t.Fatal("first keyframe did not seed unity edges")
	}
	x := volumePointX(c, view, points[index].SourceTime)
	y := volumeY(points[index].Percent, 10, 30)
	if volumeSource(c, view, x) != 4000000 || volumeHit(c, view, points, x, y, 10, 30) != index || volumePercent(y, 10, 30) != 25 {
		t.Fatal("keyframe graph mapping inconsistent")
	}
	moveVolumePoint(c, points, index, 7000000, 200)
	if points[index].SourceTime != 5999999 || points[index].Percent != 100 {
		t.Fatal("drag crossed neighbor or exceeded gain bounds")
	}
	moveVolumePoint(c, points, index, 0, -1)
	if points[index].SourceTime != 2000001 || points[index].Percent != 0 {
		t.Fatal("drag crossed left neighbor")
	}
}
