package main

import (
	"fmt"
	"unsafe"
)

func (e *editor) showCompositeDiagnostics() error {
	if e.lastCompositeMetrics == nil {
		return fmt.Errorf("start composite playback before collecting diagnostics")
	}
	report := e.lastCompositeMetrics.Snapshot()
	message := report.String() + "\n\nProcess retirement counts are final only after workers finish. Ctrl+C copies this dialog."
	user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(message))), uintptr(unsafe.Pointer(u("Composite playback diagnostics"))), 0x40)
	return nil
}
