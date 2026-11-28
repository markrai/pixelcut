package projectexport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExportProcessFrameFailureHelper(t *testing.T) {
	if os.Getenv("PIXELCUT_TEST_FRAME_FAILURE") != "1" {
		t.Skip("decoder failure subprocess helper")
	}
	fmt.Fprintln(os.Stderr, "simulated decoder initialization failure")
	os.Exit(1)
}

func TestExportProcessReadFrameFailurePreservesDiagnostic(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Setenv("PIXELCUT_TEST_FRAME_FAILURE", "1")
	p := newExportProcess(ctx, exe, "-test.run=^TestExportProcessFrameFailureHelper$")
	defer p.close()
	reader, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.pipe = reader
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	err = p.readFrame(reader, make([]byte, 6))
	if !errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "simulated decoder initialization failure") || !p.waited || ctx.Err() != nil {
		t.Fatalf("lost decoder diagnostic or process not reaped: %v", err)
	}
	if err := p.abort(); err != nil {
		t.Fatalf("repeated cleanup: %v", err)
	}
}
