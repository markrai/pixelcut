package media

type PreviewDecoderStats struct {
	Opened, Retired, Peak int
	Units                 int64
}

func (s *PreviewDecoderStats) opened(active int) {
	s.Opened++
	s.Peak = max(s.Peak, active)
}
