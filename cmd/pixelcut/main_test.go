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

func TestGPUDecodeCheckCommandArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"gpu-decode-check", "-h"}, &out); err != nil || !strings.Contains(out.String(), "start-frame") {
		t.Fatalf("decode check help: %v", err)
	}
	if err := run(context.Background(), []string{"gpu-decode-check"}, &out); err == nil {
		t.Fatal("missing source accepted")
	}
	out.Reset()
	if err := run(context.Background(), []string{"gpu-decode-check", "-frames", "301", "source.mp4"}, &out); err == nil || !strings.Contains(out.String(), "failed") {
		t.Fatal("invalid frame count did not return a failed report")
	}
}

func TestExportDecodeBackendArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"export-project", "-decode", "bogus", "-color", "hlg", "-fps", "30", "project.pixelcut.json", "out.mp4"}, &out); err == nil || !strings.Contains(err.Error(), "unknown decode backend") {
		t.Fatal("invalid decode backend accepted")
	}
	if err := run(context.Background(), []string{"export-video", "-h"}, &out); err != nil || !strings.Contains(out.String(), "experimental cuda") {
		t.Fatalf("decode flag missing from export help: %v", err)
	}
}

func TestGPUScaleCheckCommandArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"gpu-scale-check", "-h"}, &out); err != nil || !strings.Contains(out.String(), "width") || !strings.Contains(out.String(), "height") {
		t.Fatalf("scale check help: %v", err)
	}
	if err := run(context.Background(), []string{"gpu-scale-check"}, &out); err == nil {
		t.Fatal("missing source accepted")
	}
	out.Reset()
	if err := run(context.Background(), []string{"gpu-scale-check", "-width", "31", "-height", "24", "source.mp4"}, &out); err == nil || !strings.Contains(out.String(), "failed") || !strings.Contains(out.String(), "output dimensions") {
		t.Fatal("invalid dimensions did not return a failed report")
	}
}

func TestGPUPreviewScaleCheckCommandArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"gpu-preview-scale-check", "-h"}, &out); err != nil || !strings.Contains(out.String(), "start-frame") {
		t.Fatalf("preview scale help: %v", err)
	}
	out.Reset()
	if err := run(context.Background(), []string{"gpu-preview-scale-check", "-diagnose", "-width", "960", "-height", "540", "-frames", "121", "source.mp4"}, &out); err == nil || !strings.Contains(out.String(), "failed") {
		t.Fatal("invalid preview scale interval accepted")
	}
}
