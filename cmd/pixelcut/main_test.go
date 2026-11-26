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
