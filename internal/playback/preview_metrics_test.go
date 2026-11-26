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
