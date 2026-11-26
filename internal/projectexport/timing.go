package projectexport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

const exportTimescale = 240000

func verifyPacketGrid(r io.Reader, fps int, frames int64) error {
	switch fps {
	case 24, 25, 30, 50, 60:
	default:
		return errors.New("invalid packet grid frame rate")
	}
	if frames <= 0 || frames > MaxDuration/project.TicksPerSecond*int64(fps) {
		return errors.New("invalid packet grid frame count")
	}
	step := int64(exportTimescale / fps)
	seen := make([]byte, (frames+7)/8)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4096), 65536)
	var count int64
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "|")
		if parts[0] != "packet" {
			return errors.New("unexpected packet probe record")
		}
		var pts, duration int64
		var havePTS, haveDuration bool
		for _, field := range parts[1:] {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			switch key {
			case "pts":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || havePTS {
					return errors.New("invalid packet PTS")
				}
				pts, havePTS = n, true
			case "duration":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || haveDuration {
					return errors.New("invalid packet duration")
				}
				duration, haveDuration = n, true
			}
		}
		if !havePTS || !haveDuration || pts < 0 || pts%step != 0 || duration != step {
			return errors.New("video packet is outside exact CFR grid")
		}
		n := pts / step
		if n >= frames {
			return errors.New("video packet exceeds planned duration")
		}
		mask := byte(1 << (n % 8))
		if seen[n/8]&mask != 0 {
			return errors.New("duplicate video presentation timestamp")
		}
		seen[n/8] |= mask
		count++
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if count != frames {
		return fmt.Errorf("video packet count %d != planned %d", count, frames)
	}
	return nil
}

func verifyPublishedTiming(ctx context.Context, engine media.Engine, plan Plan, path string, report media.Report) error {
	v, err := report.Video()
	if err != nil {
		return err
	}
	if v.TimeBase != "1/240000" || v.DurationTS == nil || *v.DurationTS != plan.VideoFrames*int64(exportTimescale/plan.Contract.FPS) {
		return errors.New("video stream time base or duration does not match exact CFR contract")
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(run, engine.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_packets", "-show_entries", "packet=pts,duration", "-of", "compact=p=1:nk=0", "-i", path)
	cmd.WaitDelay = 2 * time.Second
	var tail errorTail
	cmd.Stderr = &tail
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	err = verifyPacketGrid(stdout, plan.Contract.FPS, plan.VideoFrames)
	if err != nil {
		cancel()
	}
	wait := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if wait != nil || len(tail.data) != 0 {
		return fmt.Errorf("video timing probe failed: %v: %s", wait, tail.data)
	}
	return nil
}
