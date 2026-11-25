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

func (e Engine) EmbeddedPreview(ctx context.Context, path string, window uintptr, start, duration float64, hdr bool, inputConfig, ipcServer string) (*exec.Cmd, error) {
	if window == 0 || start < 0 || duration <= 0 {
		return nil, errors.New("preview requires a host window and positive source interval")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	player, err := findMPV(e.MPV)
	if err != nil {
		return nil, err
	}
	args := []string{"--no-config", "--vo=gpu-next", "--hwdec=no", "--wid=" + strconv.FormatUint(uint64(uint32(window)), 10), "--start=" + seconds(start), "--length=" + seconds(duration), "--keep-open=no", "--osc=yes", "--input-terminal=no"}
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
