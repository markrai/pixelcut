package main

import (
	"strings"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestCompositeStillMenuAndPlacementLabels(t *testing.T) {
	for _, items := range [][]menuItem{timelineMenuItems(timelineMenuState{}), trackMenuItems(project.New(), "V1", nil)} {
		v, ok := findMenuItem(items, menuCompositeStill)
		if !ok || !v.enabled {
			t.Fatal("composite frame command unavailable")
		}
	}
	p := project.New()
	if placementButtonLabel(p.Tracks[0]) != "Video to V1" || placementButtonLabel(p.Tracks[1]) != "Audio to A1" {
		t.Fatal("placement type not explicit")
	}
	if !strings.Contains(placementStatus(p.Tracks[1]), "Audio only") {
		t.Fatal("audio-only placement not explained")
	}
}
