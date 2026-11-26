package playback

import (
	"fmt"
	"sync"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

type PreviewReport struct {
	Decoded, Presented, ProducerDrops, UIDrops, QueueStarvations int64
	FirstFrame, MaxDecode, MaxClockLag, Retirement               time.Duration
	Video, Audio                                                 media.PreviewDecoderStats
	AudioConfirmed                                               int64
	Finished                                                     bool
	Levels                                                       media.AudioLevels
}

type PreviewMetrics struct {
	mu        sync.Mutex
	started   time.Time
	cancelled time.Time
	report    PreviewReport
	latest    media.AudioLevels
	levelTime time.Time
}

func NewPreviewMetrics() *PreviewMetrics {
	return &PreviewMetrics{started: time.Now()}
}

func (m *PreviewMetrics) Decoded(elapsed time.Duration, eligible bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report.Decoded++
	m.report.MaxDecode = max(m.report.MaxDecode, elapsed)
	if !eligible {
		m.report.ProducerDrops++
	}
}

func (m *PreviewMetrics) Presented(lag time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.report.Presented == 0 {
		m.report.FirstFrame = time.Since(m.started)
	}
	m.report.Presented++
	m.report.MaxClockLag = max(m.report.MaxClockLag, lag)
}

func (m *PreviewMetrics) Dropped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report.UIDrops++
}

func (m *PreviewMetrics) Starved() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report.QueueStarvations++
}

func (m *PreviewMetrics) Cancelled() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelled.IsZero() && !m.report.Finished {
		m.cancelled = time.Now()
	}
}

func (m *PreviewMetrics) SetVideo(stats media.PreviewDecoderStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report.Video = stats
}

func (m *PreviewMetrics) SetAudio(stats media.PreviewDecoderStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report.Audio = stats
}

func (m *PreviewMetrics) ConfirmAudio(levels media.AudioLevels) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest = levels
	m.levelTime = time.Now()
	m.report.Levels.Merge(levels)
}

func (m *PreviewMetrics) AudioMeter() (media.AudioLevels, media.AudioLevels, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.latest, m.report.Levels, m.levelTime
}

func (m *PreviewMetrics) Finished(confirmed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report.Finished = true
	m.report.AudioConfirmed = confirmed
	if !m.cancelled.IsZero() {
		m.report.Retirement = time.Since(m.cancelled)
	}
}

func (m *PreviewMetrics) Snapshot() PreviewReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.report
}

func (r PreviewReport) String() string {
	return fmt.Sprintf("Composite preview (SDR diagnostic)\nDecoded / presented: %d / %d\nProducer / UI drops: %d / %d\nFirst presented frame: %s\nMaximum frame render time: %s\nMaximum presented clock lag: %s\nAudio queue starvation episodes: %d\nVideo processes opened / retired / peak: %d / %d / %d\nAudio processes opened / retired / peak: %d / %d / %d\nConfirmed audio frames at retirement: %d\nRetirement duration: %s\nWorkers finished: %t\n\nClock lag is relative to confirmed buffers, not measured speaker/display latency.", r.Decoded, r.Presented, r.ProducerDrops, r.UIDrops, r.FirstFrame.Round(time.Millisecond), r.MaxDecode.Round(time.Millisecond), r.MaxClockLag.Round(time.Millisecond), r.QueueStarvations, r.Video.Opened, r.Video.Retired, r.Video.Peak, r.Audio.Opened, r.Audio.Retired, r.Audio.Peak, r.AudioConfirmed, r.Retirement.Round(time.Millisecond), r.Finished) + "\n\n" + r.Levels.String()
}
