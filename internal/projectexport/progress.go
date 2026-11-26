package projectexport

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/markrai/pixelcut/internal/media"
)

type Stage string

const (
	StagePreflight Stage = "preflight"
	StageVideo     Stage = "video"
	StageVideoWait Stage = "video-finalize"
	StageAudio     Stage = "audio"
	StageAudioWait Stage = "audio-finalize"
	StageVerify    Stage = "verify"
	StagePublish   Stage = "publish"
	StageDone      Stage = "done"
)

type Progress struct {
	Stage       Stage
	Completed   int64
	Total       int64
	Warnings    []string
	AudioLevels media.AudioLevels
}

type Observer func(Progress)

func emit(observe Observer, stage Stage, completed, total int64) {
	if observe != nil {
		observe(Progress{Stage: stage, Completed: completed, Total: total})
	}
}

type progressWriter struct {
	dst       io.Writer
	observe   Observer
	stage     Stage
	unitBytes int64
	total     int64
	written   int64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	w.written += int64(n)
	if err == nil && n == len(p) {
		emit(w.observe, w.stage, w.written/w.unitBytes, w.total)
	}
	return n, err
}

func ValidateDestination(output string) (string, error) {
	if strings.TrimSpace(output) == "" {
		return "", errors.New("output path is required")
	}
	out, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(out), ".mp4") {
		return "", errors.New("output must have an .mp4 extension")
	}
	if _, err := os.Lstat(out); err == nil {
		return "", errors.New("output already exists; choose a new filename")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	dir, err := os.Stat(filepath.Dir(out))
	if err != nil {
		return "", err
	}
	if !dir.IsDir() {
		return "", errors.New("output parent must be a directory")
	}
	return out, nil
}
