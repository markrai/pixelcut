package main

import (
	"strings"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestAssetColorLabel(t *testing.T) {
	if assetColorLabel(project.Asset{}) != "" {
		t.Fatal("legacy HLG media label changed")
	}
	a := project.Asset{ColorFamily: media.ColorSDR709, ColorDefaults: media.DefaultTransfer}
	label := assetColorLabel(a)
	for _, want := range []string{"SDR BT.709", "assumed tags", "bin only"} {
		if !strings.Contains(label, want) {
			t.Fatalf("missing label %q: %s", want, label)
		}
	}
	a.ColorFamily, a.ColorDefaults = media.ColorPQ, 0
	if label = assetColorLabel(a); !strings.Contains(label, "HDR PQ") || strings.Contains(label, "assumed") {
		t.Fatalf("PQ label: %s", label)
	}
}
