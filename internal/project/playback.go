package project

func PlaybackTarget(clips []Clip, at int64) (int, int64, bool) {
	index := -1
	for i, c := range clips {
		if _, ok := c.TimelineInterval(); !ok {
			continue
		}
		if c.Contains(at) {
			return i, at, true
		}
		if c.Start > at && (index < 0 || c.Start < clips[index].Start) {
			index = i
		}
	}
	if index >= 0 {
		return index, clips[index].Start, true
	}
	return -1, at, false
}
