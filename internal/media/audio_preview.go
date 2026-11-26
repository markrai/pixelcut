package media

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

func (e Engine) AudioPreview(ctx context.Context, path string, start, duration float64, ipcServer string, paused bool) (*exec.Cmd, error) {
	return e.AudioPreviewGain(ctx, path, start, duration, ipcServer, paused, 100)
}

func (e Engine) AudioPreviewGain(ctx context.Context, path string, start, duration float64, ipcServer string, paused bool, gain int) (*exec.Cmd, error) {
	gainArgs, err := previewGainArgs(gain, false)
	if err != nil {
		return nil, err
	}
	if gain == 0 {
		return nil, errors.New("audio audition requires nonzero gain")
	}
	if math.IsNaN(start) || math.IsNaN(duration) || math.IsInf(start+duration, 0) || start < 0 || duration <= 0 || ipcServer == "" {
		return nil, errors.New("audio preview requires a positive source interval and IPC server")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	player, err := e.MPVPath()
	if err != nil {
		return nil, err
	}
	args := []string{"--no-config", "--vid=no", "--aid=auto", "--force-window=no", "--audio-display=no", "--start=" + seconds(start), "--end=" + seconds(start+duration), "--keep-open=yes", "--input-terminal=no", "--hr-seek=yes", "--input-ipc-server=" + ipcServer}
	args = append(args, gainArgs...)
	if paused {
		args = append(args, "--pause=yes")
	}
	cmd := exec.CommandContext(ctx, player, append(args, "--", p)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return cmd, nil
}
