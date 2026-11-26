package main

import "testing"

func TestMainToolbarSettingsAndExportLayout(t *testing.T) {
	for _, width := range []int{1040, 1064, 1108, 1184, 1920} {
		seen := make(map[int]bool)
		right := 0
		for i, item := range mainToolbarItems {
			x := mainToolbarX(item.id, item.x, item.width, width)
			if seen[item.id] || x < right || x+item.width > width {
				t.Fatalf("toolbar overlap, duplicate or clipping at width %d: %+v", width, item)
			}
			seen[item.id] = true
			right = x + item.width
			if item.id == idPreferences && (i == 0 || mainToolbarItems[i-1].id != idKeys || item.label != "Settings") {
				t.Fatal("Settings must be beside Shortcuts in the main toolbar")
			}
		}
		if !seen[idPreferences] || !seen[idExportMenu] {
			t.Fatal("missing main toolbar action")
		}
	}
}
