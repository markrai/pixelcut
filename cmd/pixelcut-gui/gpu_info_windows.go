package main

import (
	"context"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
)

func (e *editor) showGPUInfo() error {
	engine := e.engine
	e.stopPreview()
	e.tell("Inspecting FFmpeg GPU capabilities and attempting device initialization...")
	e.startJob(func(ctx context.Context) result {
		report, err := engine.ProbeAcceleration(ctx, true)
		if err != nil {
			return result{err: err}
		}
		return result{gpuInfo: &report}
	})
	return nil
}

func (e *editor) presentGPUInfo(report media.AccelerationReport) {
	e.tell("GPU diagnostics complete. See selected decode backend in the report.")
	message := report.String() + "\n\nCtrl+C copies this dialog."
	user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(message))), uintptr(unsafe.Pointer(u("PixelCut GPU diagnostics"))), 0x40)
}
