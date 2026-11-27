package projectexport

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

type errorTail struct {
	data []byte
}

func (b *errorTail) Write(p []byte) (int, error) {
	const limit = 8192
	n := len(p)
	if n >= limit {
		b.data = append(b.data[:0], p[n-limit:]...)
		return n, nil
	}
	if excess := len(b.data) + n - limit; excess > 0 {
		copy(b.data, b.data[excess:])
		b.data = b.data[:len(b.data)-excess]
	}
	b.data = append(b.data, p...)
	return n, nil
}

type exportProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	tail   errorTail
	pipe   io.Closer
	waited bool
}

func newExportProcess(ctx context.Context, executable string, args ...string) *exportProcess {
	run, cancel := context.WithCancel(ctx)
	p := &exportProcess{cmd: exec.CommandContext(run, executable, args...), cancel: cancel}
	p.cmd.Stderr = &p.tail
	p.cmd.WaitDelay = 2 * time.Second
	return p
}

func (p *exportProcess) wait() error {
	p.waited = true
	err := p.cmd.Wait()
	p.cancel()
	if err != nil {
		return fmt.Errorf("FFmpeg: %w: %s", err, p.tail.data)
	}
	return nil
}

func (p *exportProcess) abort() error {
	p.cancel()
	if p.pipe != nil {
		_ = p.pipe.Close()
	}
	if !p.waited && p.cmd.Process != nil {
		return p.wait()
	}
	return nil
}

func (p *exportProcess) close() {
	_ = p.abort()
}
