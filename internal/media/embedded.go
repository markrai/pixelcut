package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func (e Engine) MPVPath() (string, error) { return findMPV(e.MPV) }

func (e Engine) EmbeddedPreview(ctx context.Context, path string, window uintptr, start, duration float64, hdr bool, inputConfig, ipcServer string, paused bool, script string) (*exec.Cmd, error) {
	return e.EmbeddedPreviewMode(ctx, path, window, start, duration, hdr, inputConfig, ipcServer, paused, script, false)
}

func (e Engine) EmbeddedPreviewMode(ctx context.Context, path string, window uintptr, start, duration float64, hdr bool, inputConfig, ipcServer string, paused bool, script string, silent bool) (*exec.Cmd, error) {
	return e.EmbeddedPreviewGain(ctx, path, window, start, duration, hdr, inputConfig, ipcServer, paused, script, silent, 100)
}

func (e Engine) EmbeddedPreviewGain(ctx context.Context, path string, window uintptr, start, duration float64, hdr bool, inputConfig, ipcServer string, paused bool, script string, silent bool, gain int) (*exec.Cmd, error) {
	gainArgs, err := previewGainArgs(gain, silent)
	if err != nil {
		return nil, err
	}
	if window == 0 || start < 0 || duration <= 0 {
		return nil, errors.New("preview requires a host window and positive source interval")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	player, err := e.MPVPath()
	if err != nil {
		return nil, err
	}
	args := []string{"--no-config", "--vo=gpu-next", "--hwdec=no", "--wid=" + strconv.FormatUint(uint64(uint32(window)), 10), "--start=" + seconds(start), "--end=" + seconds(start+duration), "--keep-open=yes", "--osc=no", "--input-terminal=no", "--input-default-bindings=no", "--input-builtin-bindings=no", "--input-builtin-drag-and-drop=no", "--hr-seek=yes", "--window-dragging=no", "--cursor-autohide=no"}
	args = append(args, gainArgs...)
	if script != "" {
		args = append(args, "--script="+script)
	}
	if paused {
		args = append(args, "--pause=yes")
	}
	if ipcServer != "" {
		args = append(args, "--input-ipc-server="+ipcServer)
	}
	if inputConfig != "" {
		args = append(args, "--input-conf="+inputConfig)
	}
	if hdr {
		args = append(args, "--target-colorspace-hint=yes")
	} else {
		args = append(args, "--target-prim=bt.709", "--target-trc=bt.1886", "--tone-mapping=auto")
	}
	cmd := exec.CommandContext(ctx, player, append(args, "--", p)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = ctx.Err(); err != nil {
		return nil, fmt.Errorf("preview canceled: %w", err)
	}
	return cmd, nil
}
