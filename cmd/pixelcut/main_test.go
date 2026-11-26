package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPlanExportCommandRequiresProjectAndExplicitContract(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"help"}, &out); err != nil || !strings.Contains(out.String(), "plan-export -color hlg -fps FPS") {
		t.Fatalf("export planning missing from help: %v", err)
	}
	out.Reset()
	if err := run(context.Background(), []string{"plan-export", "-color", "hlg", "-fps", "30"}, &out); err == nil {
		t.Fatal("missing project accepted")
	}
}

func TestExportVideoCommandRequiresDestination(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"export-video", "-color", "hlg", "-fps", "30", "project.pixelcut.json"}, &out); err == nil || !strings.Contains(err.Error(), "OUTPUT.mp4") {
		t.Fatalf("missing export destination not rejected: %v", err)
	}
}

func TestExportProjectCommandRequiresDestination(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"export-project", "-color", "hlg", "-fps", "30", "project.pixelcut.json"}, &out); err == nil || !strings.Contains(err.Error(), "OUTPUT.mp4") {
		t.Fatalf("missing project export destination not rejected: %v", err)
	}
}

func TestGPUInfoCommandArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"gpu-info", "-h"}, &out); err != nil || !strings.Contains(out.String(), "probe-devices") {
		t.Fatalf("GPU diagnostics help: %v", err)
	}
	if err := run(context.Background(), []string{"gpu-info", "unexpected"}, &out); err == nil {
		t.Fatal("unexpected GPU info argument accepted")
	}
}
