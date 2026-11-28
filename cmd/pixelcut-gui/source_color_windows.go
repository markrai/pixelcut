package main

import (
	"fmt"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func assetColorLabel(a project.Asset) string {
	if !a.ImportOnly() {
		return ""
	}
	label := string(a.ColorFamily)
	if a.ColorFamily == media.ColorSDR709 {
		label = "SDR BT.709"
	} else if a.ColorFamily == media.ColorPQ {
		label = "HDR PQ"
	}
	if a.ColorDefaults != 0 {
		label += ", assumed tags"
	}
	return fmt.Sprintf(" [%s; bin only]", label)
}
