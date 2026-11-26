package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestRenderProjectAudioAndVideoPacketPreservation(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "delayed.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=30:d=0.5,format=yuv420p10le,geq=lum=512:cb=512:cr=512,setsar=1", "-itsoffset", "0.1", "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=44100:duration=0.2", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	engine := media.Default()
	report, err := engine.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := project.FromReport("a", src, report)
	if err != nil || !asset.HasAudio {
		t.Fatalf("invalid fixture: %v", err)
	}
	p := project.New()
	p.Assets = []project.Asset{asset}
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", Out: 400000}}
	p.Tracks[1].Clips = []project.Clip{{ID: "independent", AssetID: "a", Start: 450000, In: 100000, Out: 200001}}
	prepared, err := Prepare(ctx, engine, p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.Plan
	if plan.OutputAudioFrames != 27200 || plan.ContentAudioFrames != 26401 || plan.VideoFrames != 17 {
		t.Fatalf("incorrect fixture plan: %+v", plan)
	}
	sources := map[string]Source{"a": prepared.Sources[0]}
	video := filepath.Join(dir, "video.mp4")
	if err := RenderVideo(ctx, engine, p, "hlg", 30, true, video); err != nil {
		t.Fatal(err)
	}
	mux := filepath.Join(dir, "mux.mp4")
	if err := muxAudio(ctx, engine, plan, sources, video, mux); err != nil {
		t.Fatal(err)
	}
	hash := func(path string) []byte {
		t.Helper()
		data, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", path, "-map", "0:v:0", "-c:v", "copy", "-f", "hash", "-hash", "sha256", "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if !bytes.Equal(hash(video), hash(mux)) {
		t.Fatal("mux modified encoded HEVC packets")
	}
	out := filepath.Join(dir, "project.mp4")
	var events []Progress
	if err := RenderProjectObserved(ctx, engine, p, "hlg", 30, out, func(v Progress) { events = append(events, v) }); err != nil {
		t.Fatal(err)
	}
	if err := checkExportProgress(events, plan); err != nil {
		t.Fatal(err)
	}
	report, err = engine.Inspect(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyProjectOutput(plan, report); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", out, "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "s16le", "-").Output()
	if err != nil || len(raw) < int(plan.OutputAudioFrames)*4 {
		t.Fatalf("decode output audio: %d bytes %v", len(raw), err)
	}
	peak := func(first, end int) int {
		var p int
		for i := first * 4; i < end*4; i += 2 {
			v := int(int16(binary.LittleEndian.Uint16(raw[i:])))
			p = max(p, v, -v)
		}
		return p
	}
	if peak(500, 2400) > 100 || peak(7200, 12000) < 1000 || peak(16800, 20400) > 100 || peak(22800, 25200) < 1000 {
		t.Fatal("audio content/gap synchronization outside expected window")
	}
	after, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("source changed")
	}
	if err := RenderProject(ctx, engine, p, "hlg", 30, out); err == nil {
		t.Fatal("existing output accepted")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	failed := filepath.Join(dir, "cancelled.mp4")
	if err := RenderProject(cancelled, engine, p, "hlg", 30, failed); err == nil {
		t.Fatal("cancelled export accepted")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("cancelled output published")
	}
	mid, stopMid := context.WithCancel(ctx)
	defer stopMid()
	midPath := filepath.Join(dir, "mid-cancel.mp4")
	if err := RenderProjectObserved(mid, engine, p, "hlg", 30, midPath, func(v Progress) {
		if v.Stage == StageVideo && v.Completed >= 1 {
			stopMid()
		}
	}); err == nil {
		t.Fatal("mid-render cancellation accepted")
	}
	if _, err := os.Stat(midPath); !os.IsNotExist(err) {
		t.Fatal("mid-render cancellation published output")
	}
	audioCtx, stopAudio := context.WithCancel(ctx)
	defer stopAudio()
	audioPath := filepath.Join(dir, "audio-cancel.mp4")
	if err := RenderProjectObserved(audioCtx, engine, p, "hlg", 30, audioPath, func(v Progress) {
		if v.Stage == StageAudio && v.Completed > 0 {
			stopAudio()
		}
	}); err == nil {
		t.Fatal("mid-audio cancellation accepted")
	}
	if _, err := os.Stat(audioPath); !os.IsNotExist(err) {
		t.Fatal("mid-audio cancellation published output")
	}
	racePath := filepath.Join(dir, "race.mp4")
	var raceErr error
	if err := RenderProjectObserved(ctx, engine, p, "hlg", 30, racePath, func(v Progress) {
		if v.Stage == StagePublish {
			raceErr = os.WriteFile(racePath, []byte("existing"), 0600)
		}
	}); err == nil || raceErr != nil {
		t.Fatalf("late destination collision: %v %v", err, raceErr)
	}
	if data, err := os.ReadFile(racePath); err != nil || string(data) != "existing" {
		t.Fatal("late destination collision overwritten")
	}
	changedPath := filepath.Join(dir, "source-changed.mp4")
	var changeErr error
	if err := RenderProjectObserved(ctx, engine, p, "hlg", 30, changedPath, func(v Progress) {
		if v.Stage == StagePublish {
			changeErr = os.WriteFile(src, append(append([]byte(nil), original...), 0), 0600)
		}
	}); err == nil || changeErr != nil {
		t.Fatalf("changed source publication: %v %v", err, changeErr)
	}
	if _, err := os.Stat(changedPath); !os.IsNotExist(err) {
		t.Fatal("changed source export published")
	}
	files, err := filepath.Glob(filepath.Join(dir, ".pixelcut-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary export files leaked: %v %v", files, err)
	}
	t.Log("17 HLG frames; exact 27200-sample AAC presentation; delayed 44.1 kHz source and independent audio; HEVC packet hash unchanged on mux")
}
