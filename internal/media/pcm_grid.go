package media

import (
	"errors"
	"math"
	"strconv"
)

func pcmFloorFrames(ticks int64) int64 {
	q, r := ticks/1000000, ticks%1000000
	n := r * PCMRate
	tail := n / 1000000
	if n < 0 && n%1000000 != 0 {
		tail--
	}
	return q*PCMRate + tail
}

func PCMSourceFrame(sourceStart, timelineStart, runStart, offset int64) (int64, error) {
	if sourceStart < 0 || timelineStart < 0 || runStart < 0 || offset < 0 {
		return 0, errors.New("invalid audio sample position")
	}
	delta := runStart - timelineStart
	if delta > 0 && sourceStart > math.MaxInt64-delta {
		return 0, errors.New("audio source time overflow")
	}
	base := pcmFloorFrames(sourceStart + delta)
	if base > 0 && offset > math.MaxInt64-base {
		return 0, errors.New("audio source frame overflow")
	}
	frame := base + offset
	if frame < 0 {
		return 0, errors.New("negative audio source frame")
	}
	return frame, nil
}

func PCMDecodeArgs(path string, first, end int64) ([]string, error) {
	if path == "" || first < 0 || end <= first {
		return nil, errors.New("invalid PCM decode interval")
	}
	graph := "aresample=48000:async=1:first_pts=0,apad=whole_len=" + strconv.FormatInt(end, 10) + ",atrim=start_sample=" + strconv.FormatInt(first, 10) + ":end_sample=" + strconv.FormatInt(end, 10) + ",asetpts=N/SR/TB"
	return []string{"-v", "error", "-xerror", "-nostdin", "-threads", "2", "-filter_threads", "1", "-copyts", "-start_at_zero", "-i", path, "-map", "0:a:0", "-vn", "-sn", "-dn", "-af", graph, "-ac", "2", "-ar", "48000", "-c:a", "pcm_s16le", "-f", "s16le", "-"}, nil
}
