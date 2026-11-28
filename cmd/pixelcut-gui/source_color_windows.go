package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func assetColorLabel(a project.Asset) string {
	if !a.Classified() {
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
	if a.ImportOnly() {
		label += "; bin only"
	} else {
		label += "; HLG export"
	}
	return fmt.Sprintf(" [%s]", label)
}

const menuConfirmSDR = 2487

func needsSDRConfirmation(a project.Asset) bool {
	return a.ColorFamily == media.ColorSDR709 && a.ColorDefaults != 0 && !a.ColorConfirmed
}

func (e *editor) confirmSDRAsset() error {
	p := e.model.Project()
	i := e.model.AssetIndex()
	if i < 0 || i >= len(p.Assets) {
		return project.ErrAssetNotFound
	}
	a := p.Assets[i]
	if !needsSDRConfirmation(a) || !project.SourceAvailable(a.Path) {
		return fmt.Errorf("select an available SDR source with assumed color tags")
	}
	q, err := p.ConfirmSDRColor(a.ID)
	if err != nil {
		return err
	}
	var tags []string
	for _, tag := range []struct {
		bit  media.ColorDefaults
		name string
	}{
		{media.DefaultPrimaries, "BT.709 primaries"},
		{media.DefaultTransfer, "BT.709 transfer"},
		{media.DefaultMatrix, "BT.709 matrix"},
		{media.DefaultRange, "limited range"},
		{media.DefaultChroma, "left chroma"},
		{media.DefaultSAR, "square pixels"},
	} {
		if a.ColorDefaults&tag.bit != 0 {
			tags = append(tags, tag.name)
		}
	}
	message := fmt.Sprintf("%s\n\nMissing source color tags were interpreted as: %s.\n\nConfirm only if this is ordinary SDR footage. This does not change the source file. Whole-project HLG export maps SDR reference white to 75% HLG signal. Native SDR and selected SDR clip export are not yet supported.", filepath.Base(a.Path), strings.Join(tags, ", "))
	answer, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(message))), uintptr(unsafe.Pointer(u("Confirm SDR interpretation"))), 0x34|0x100)
	if answer != 6 {
		return nil
	}
	if err := e.commit(q); err != nil {
		return err
	}
	e.tell("SDR interpretation confirmed. You can now place this source on the timeline for composite preview and whole-project HLG export.")
	return nil
}
