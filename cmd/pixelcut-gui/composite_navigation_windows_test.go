package main

import (
	"context"
	"testing"
	"time"
)

func TestCompositeNavigationCancellationClearsIntent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := editor{stillActive: true, compositeResume: true, stillDue: time.Now(), stillSequence: 42, stillCancel: cancel}
	if !e.playing() {
		t.Fatal("pending resume not represented as playing intent")
	}
	e.cancelCompositeStill()
	if e.playing() || e.stillActive || e.compositeResume || !e.stillDue.IsZero() || e.stillSequence != 0 || ctx.Err() == nil {
		t.Fatal("stale composite seek survived cancellation")
	}
}

func TestSourcePreviewRetirementGate(t *testing.T) {
	done := make(chan struct{})
	e := editor{retiringPreview: done}
	if e.sourcePreviewRetired() {
		t.Fatal("active source retirement accepted")
	}
	close(done)
	if !e.sourcePreviewRetired() || e.retiringPreview != nil {
		t.Fatal("completed source retirement not cleared")
	}
}
