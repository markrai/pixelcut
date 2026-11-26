package main

import (
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestFadeMenuChoices(t *testing.T) {
	items := fadeItems(250000, 100000)
	for i, item := range items {
		got, ok := fadeChoice(item.id)
		if !ok || got != clipFadeChoices[i] || item.checked != (got == 250000) {
			t.Fatal("incorrect fade menu mapping")
		}
	}
	if _, ok := fadeChoice(clipFadeBase - 1); ok {
		t.Fatal("invalid fade menu choice accepted")
	}
	for _, id := range []int{clipFadeInMenu, clipFadeOutMenu, clipFadeReset} {
		v, ok := findMenuItem(clipAudioItems(project.Clip{FadeIn: 100000}), id)
		if !ok || !v.enabled {
			t.Fatal("fade action unavailable")
		}
	}
}
