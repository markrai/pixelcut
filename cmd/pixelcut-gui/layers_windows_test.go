package main

import (
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func TestLayerMenuEligibility(t *testing.T) {
	items := timelineMenuItems(timelineMenuState{count: 1})
	for _, id := range []int{menuLayerOpacity, menuLayerPreset, menuLayerLayout} {
		v, ok := findMenuItem(items, id)
		if !ok || !v.enabled {
			t.Fatalf("layer action missing: %d", id)
		}
	}
	for _, state := range []timelineMenuState{{count: 2}, {count: 1, audioTrack: true}, {}} {
		for _, id := range []int{menuLayerOpacity, menuLayerPreset} {
			if _, ok := findMenuItem(timelineMenuItems(state), id); ok {
				t.Fatal("layer edit offered without a single video clip")
			}
		}
	}
	a := editorstate.Availability{ClipCount: 1, TrackLocked: true}
	items = eligibleItems(timelineMenuItems(timelineMenuState{count: 1}), a)
	for _, id := range []int{menuLayerOpacity, menuLayerPreset} {
		v, ok := findMenuItem(items, id)
		if !ok || v.enabled {
			t.Fatal("locked track layer action enabled")
		}
	}
	if v, ok := findMenuItem(items, menuLayerLayout); !ok || !v.enabled {
		t.Fatal("layout inspection blocked by track lock")
	}
	opacity := 0
	items = layerOpacityItems(project.Clip{Opacity: &opacity})
	for i, percent := range layerOpacityChoices {
		v, ok := findMenuItem(items, layerOpacityBase+i)
		if !ok || !v.enabled || v.checked != (percent == 0) {
			t.Fatal("opacity menu lost checked state")
		}
	}
}
