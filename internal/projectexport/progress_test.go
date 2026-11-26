package projectexport

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func checkExportProgress(events []Progress, plan Plan) error {
	var video, audio int64
	var stages []Stage
	for _, p := range events {
		if len(stages) == 0 || stages[len(stages)-1] != p.Stage {
			stages = append(stages, p.Stage)
		}
		switch p.Stage {
		case StageVideo:
			if p.Total != plan.VideoFrames || p.Completed < video || p.Completed > p.Total {
				return errors.New("invalid video progress")
			}
			video = p.Completed
		case StageAudio:
			if p.Total != plan.OutputAudioFrames || p.Completed < audio || p.Completed > p.Total {
				return errors.New("invalid audio progress")
			}
			audio = p.Completed
		}
	}
	want := []Stage{StagePreflight, StageVideo, StageVideoWait, StageVerify, StageAudio, StageAudioWait, StageVerify, StagePublish, StageDone}
	if fmt.Sprint(stages) != fmt.Sprint(want) || video != plan.VideoFrames || audio != plan.OutputAudioFrames {
		return fmt.Errorf("incomplete export progress: %v video=%d audio=%d", stages, video, audio)
	}
	return nil
}

func TestProgressWriterCountsSubmittedUnits(t *testing.T) {
	var dst bytes.Buffer
	var events []Progress
	w := &progressWriter{dst: &dst, observe: func(p Progress) { events = append(events, p) }, stage: StageAudio, unitBytes: 4, total: 5}
	for _, n := range []int{8, 12} {
		if _, err := w.Write(make([]byte, n)); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) != 2 || events[0].Completed != 2 || events[1].Completed != 5 || dst.Len() != 20 {
		t.Fatalf("incorrect progress: %+v", events)
	}
}

func TestValidateDestination(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "export.MP4")
	if got, err := ValidateDestination(good); err != nil || got != good {
		t.Fatalf("valid output rejected: %v", err)
	}
	if err := os.WriteFile(good, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", good, filepath.Join(dir, "bad.mov"), filepath.Join(dir, "missing", "out.mp4"), filepath.Join(good, "out.mp4")} {
		if _, err := ValidateDestination(path); err == nil {
			t.Fatalf("invalid destination accepted: %q", path)
		}
	}
	if b, err := os.ReadFile(good); err != nil || string(b) != "original" {
		t.Fatal("destination validation modified existing file")
	}
}
