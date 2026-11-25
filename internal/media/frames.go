package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const MaxIndexedFrames = 1000000
const frameTicksPerSecond = 1000000

type FrameIndex struct {
	ticks []int64
}

func (f *FrameIndex) Count() int {
	if f == nil {
		return 0
	}
	return len(f.ticks)
}

func (f *FrameIndex) Step(at, lo, hi int64, forward bool) (int64, bool) {
	if f == nil || lo >= hi {
		return 0, false
	}
	if forward {
		i := sort.Search(len(f.ticks), func(i int) bool { return f.ticks[i] > at && f.ticks[i] >= lo })
		if i < len(f.ticks) && f.ticks[i] < hi {
			return f.ticks[i], true
		}
	} else {
		i := sort.Search(len(f.ticks), func(i int) bool { return f.ticks[i] >= at || f.ticks[i] >= hi }) - 1
		if i >= 0 && f.ticks[i] >= lo {
			return f.ticks[i], true
		}
	}
	return 0, false
}

func (f *FrameIndex) Nearest(at, lo, hi int64) (int64, bool) {
	if f == nil || lo < 0 || lo > hi {
		return 0, false
	}
	at = max(lo, min(hi, at))
	i := sort.Search(len(f.ticks), func(i int) bool { return f.ticks[i] >= at })
	var best int64
	found := false
	for _, j := range []int{i - 1, i} {
		if j < 0 || j >= len(f.ticks) {
			continue
		}
		v := f.ticks[j]
		if v < lo || v > hi {
			continue
		}
		if !found || tickDistance(v, at) < tickDistance(best, at) {
			best, found = v, true
		}
	}
	return best, found
}

func tickDistance(a, b int64) int64 {
	if a >= b {
		return a - b
	}
	return b - a
}

func parseFrameTime(s string) (int64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) >= float64(math.MaxInt64)/frameTicksPerSecond/2 {
		return 0, errors.New("invalid frame timestamp")
	}
	return int64(math.Round(v * frameTicksPerSecond)), nil
}

func readFrameIndex(r io.Reader) (*FrameIndex, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4096), 1<<20)
	var ticks []int64
	var origin int64
	hasOrigin := false
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "|")
		switch parts[0] {
		case "frame":
			found := false
			for _, field := range parts[1:] {
				if s, ok := strings.CutPrefix(field, "best_effort_timestamp_time="); ok {
					v, err := parseFrameTime(s)
					if err != nil {
						return nil, err
					}
					if len(ticks) >= MaxIndexedFrames {
						return nil, errors.New("frame index exceeds one million frames")
					}
					ticks = append(ticks, v)
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("decoded frame has no timestamp")
			}
		case "format":
			for _, field := range parts[1:] {
				if s, ok := strings.CutPrefix(field, "start_time="); ok {
					v, err := parseFrameTime(s)
					if err != nil {
						return nil, err
					}
					origin, hasOrigin = v, true
					break
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !hasOrigin || len(ticks) == 0 {
		return nil, errors.New("no complete frame timestamp index")
	}
	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	out := ticks[:0]
	for _, v := range ticks {
		v -= origin
		if v < 0 {
			continue
		}
		if len(out) > 0 && v == out[len(out)-1] {
			return nil, errors.New("multiple frames share a timestamp")
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("no nonnegative frame timestamps")
	}
	return &FrameIndex{ticks: out}, nil
}

func (e Engine) Frames(ctx context.Context, path string) (*FrameIndex, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	job, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(job, e.FFprobe, "-v", "error", "-threads", "2", "-select_streams", "v:0", "-show_frames", "-show_format", "-show_entries", "frame=best_effort_timestamp_time:format=start_time", "-of", "compact=p=1:nk=0", "-i", p)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	index, err := readFrameIndex(stdout)
	if err != nil {
		cancel()
	}
	wait := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if wait != nil {
		return nil, fmt.Errorf("frame probe: %w", wait)
	}
	return index, nil
}
