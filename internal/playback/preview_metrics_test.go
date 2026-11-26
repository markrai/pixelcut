package playback

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

func TestPreviewMetricsConcurrentAndRetired(t *testing.T) {
	m := NewPreviewMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Decoded(time.Duration(j)*time.Millisecond, j%2 == 0)
				m.Presented(50 * time.Millisecond)
				m.Dropped()
				m.Starved()
				_ = m.Snapshot()
			}
		}()
	}
	wg.Wait()
	m.Cancelled()
	m.SetVideo(media.PreviewDecoderStats{Opened: 4, Retired: 4, Peak: 2})
	m.SetAudio(media.PreviewDecoderStats{Opened: 2, Retired: 2, Peak: 2})
	m.Finished(48000)
	r := m.Snapshot()
	if r.Decoded != 400 || r.Presented != 400 || r.ProducerDrops != 200 || r.UIDrops != 400 || r.QueueStarvations != 400 {
		t.Fatalf("lost metrics: %+v", r)
	}
	if !r.Finished || r.Video.Opened != r.Video.Retired || r.Audio.Opened != r.Audio.Retired || r.AudioConfirmed != 48000 || r.MaxClockLag != 50*time.Millisecond || r.MaxDecode != 99*time.Millisecond {
		t.Fatalf("incorrect final metrics: %+v", r)
	}
	if !strings.Contains(r.String(), "not measured speaker/display latency") {
		t.Fatal("diagnostic report misrepresents clock-relative timing")
	}
}

func TestConfirmedAudioLevelsAreRunScoped(t *testing.T) {
	m := NewPreviewMetrics()
	if latest, total, at := m.AudioMeter(); latest.Frames != 0 || total.Frames != 0 || !at.IsZero() {
		t.Fatal("new run has stale levels")
	}
	first := media.AudioLevels{Frames: 2400, Peak: [2]uint32{32767, 1000}, Clipped: [2]int64{7, 0}}
	second := media.AudioLevels{Frames: 2400, Peak: [2]uint32{200, 32768}, Clipped: [2]int64{0, 3}}
	m.ConfirmAudio(first)
	m.ConfirmAudio(second)
	latest, total, at := m.AudioMeter()
	if latest != second || total.Frames != 4800 || total.Peak != [2]uint32{32767, 32768} || total.ClipCount() != 10 || at.IsZero() {
		t.Fatalf("incorrect confirmed meter: %+v %+v", latest, total)
	}
	m.Cancelled()
	m.Finished(4800)
	if m.Snapshot().Levels != total {
		t.Fatal("retirement lost confirmed peaks")
	}
	if next := NewPreviewMetrics().Snapshot().Levels; next.Frames != 0 || next.ClipCount() != 0 {
		t.Fatal("new run inherited clipping")
	}
}
