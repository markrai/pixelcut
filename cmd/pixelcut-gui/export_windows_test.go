package main

import (
	"strings"
	"sync"
	"testing"

	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/projectexport"
)

func TestExportMenuIndependentOfClipSelection(t *testing.T) {
	p := project.New()
	items := exportMenuItems(p, false)
	for _, v := range items {
		if v.enabled {
			t.Fatal("empty project export enabled")
		}
	}
	p.Tracks[0].Clips = []project.Clip{{ID: "clip", Start: 0, Out: 1000000}}
	items = exportMenuItems(p, false)
	var rates []int
	for _, v := range items {
		if fps, ok := exportRate(v.id); ok && v.enabled {
			rates = append(rates, fps)
		}
		if v.id == idExport && v.enabled {
			t.Fatal("selected-clip action enabled without selection")
		}
	}
	if len(rates) != 5 {
		t.Fatalf("project export depends on clip selection: %v", rates)
	}
	for i, rate := range exportRates {
		if rates[i] != rate {
			t.Fatal("wrong FPS mapping")
		}
	}
	if _, ok := exportRate(exportFPSBase); ok {
		t.Fatal("menu header accepted")
	}
}

func TestExportProgressSnapshotAndLabels(t *testing.T) {
	j := &projectExportJob{}
	j.observe(projectexport.Progress{Stage: projectexport.StagePreflight, Warnings: []string{"warning"}})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := int64(0); n <= 100; n++ {
			j.observe(projectexport.Progress{Stage: projectexport.StageVideo, Completed: n, Total: 100})
		}
	}()
	for i := 0; i < 100; i++ {
		j.snapshot()
	}
	wg.Wait()
	p, warnings := j.snapshot()
	if len(warnings) != 1 || !strings.Contains(exportStatus(p, len(warnings)), "100/100 frames submitted") {
		t.Fatal("progress or warnings lost")
	}
	if strings.Contains(exportStatus(p, len(warnings)), "complete") {
		t.Fatal("submitted frames mislabeled as completion")
	}
	if !strings.Contains(exportStatus(projectexport.Progress{Stage: projectexport.StageAudio, Completed: 1200, Total: 2400}, 0), "50%") {
		t.Fatal("audio percentage incorrect")
	}
}
