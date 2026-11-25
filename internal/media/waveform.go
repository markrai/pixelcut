package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/markrai/pixelcut/internal/waveform"
)

func (e Engine) Waveform(ctx context.Context, path string, duration int64) (*waveform.Peaks, error) {
	if duration <= 0 || duration > waveform.MaxDuration {
		return nil, errors.New("waveform duration outside supported range")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.FFmpeg, "-v", "error", "-nostdin", "-threads", "2", "-i", p, "-map", "0:a:0", "-vn", "-sn", "-dn", "-af", fmt.Sprintf("aresample=%d:async=1:first_pts=0", waveform.SampleRate), "-ac", "2", "-ar", fmt.Sprint(waveform.SampleRate), "-t", seconds(float64(duration)/float64(waveform.TicksPerSecond)), "-c:a", "pcm_s16le", "-f", "s16le", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	peaks, readErr := waveform.ReadPCM(pipe, duration)
	if readErr != nil {
		cancel()
	}
	if readErr == nil {
		var discard [4096]byte
		for {
			_, err = pipe.Read(discard[:])
			if err != nil {
				break
			}
		}
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		return nil, fmt.Errorf("waveform decode: %w: %s", waitErr, stderr.String())
	}
	if readErr != nil {
		return nil, readErr
	}
	return peaks, nil
}
