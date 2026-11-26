package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func audioQualificationAsset(t *testing.T, ctx context.Context, dir, id, rate, delay, tone string) project.Asset {
	t.Helper()
	path := filepath.Join(dir, id+".mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=30:d=0.8,format=yuv420p10le,geq=lum=512:cb=512:cr=512,setsar=1", "-itsoffset", delay, "-f", "lavfi", "-i", "aevalsrc="+tone+":s="+rate+":d=0.6", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", path)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	report, err := media.Default().Inspect(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := project.FromReport(id, path, report)
	if err != nil || !a.HasAudio {
		t.Fatalf("fixture asset: %v", err)
	}
	return a
}

func previewQualificationPCM(t *testing.T, ctx context.Context, p project.Project, start int64) ([]byte, media.AudioLevels) {
	t.Helper()
	e, err := composition.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	d := media.NewPCMDecoder(ctx, media.Default())
	defer d.Close()
	var result bytes.Buffer
	var levels media.AudioLevels
	total := media.PCMFramesForTicks(e.Duration() - start)
	began := time.Now()
	for pos := int64(0); pos < total; {
		n := int(min(int64(1379), total-pos))
		b, err := composition.MixPCM(ctx, e, d, start, pos, n)
		if err != nil {
			t.Fatal(err)
		}
		if err := binary.Write(&result, binary.LittleEndian, b.Samples); err != nil {
			t.Fatal(err)
		}
		levels.Merge(b.Levels)
		pos += int64(n)
	}
	d.Close()
	stats := d.Stats()
	if stats.Opened != stats.Retired || stats.Peak != 2 || stats.Opened != 2 {
		t.Fatalf("unexpected decoder resource use: %+v", stats)
	}
	t.Logf("preview start=%d frames=%d elapsed=%s decoders=%+v", start, total, time.Since(began), stats)
	return result.Bytes(), levels
}

func TestAudioQualificationRealDecoderParity(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	a := audioQualificationAsset(t, ctx, dir, "delayed", "44100", "0.1", "0.15*sin(2*PI*997*t)|0.08*sin(2*PI*1301*t)")
	b := audioQualificationAsset(t, ctx, dir, "stereo", "48000", "0", "0.1*sin(2*PI*443*t)|0.12*sin(2*PI*701*t)")
	p := project.New()
	p.Assets = []project.Asset{a, b}
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: a.ID, Start: 10001, In: 20000, Out: 420001}}
	p.Tracks[1].Clips = []project.Clip{{ID: "overlap", AssetID: b.ID, Start: 200000, In: 50000, Out: 300000}, {ID: "muted", AssetID: b.ID, Start: 500000, Out: 100001, AudioMuted: true}}
	var err error
	base := project.ClipRef{TrackID: "V1", ClipID: "base"}
	p, err = p.SetClipGain(base, 125)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.SetClipFades(base, 100000, 50000)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.SetClipEnvelope(base, []project.VolumeKeyframe{{SourceTime: 20000, Percent: 100}, {SourceTime: 220000, Percent: 25}, {SourceTime: 420001, Percent: 100}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.SetClipGain(project.ClipRef{TrackID: "A1", ClipID: "overlap"}, 75)
	if err != nil {
		t.Fatal(err)
	}
	engine := media.Default()
	prepared, err := Prepare(ctx, engine, p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.Plan
	sources := make(map[string]Source)
	for _, source := range prepared.Sources {
		sources[source.Asset.ID] = source
	}
	d := &offlineAudio{ctx: ctx, engine: engine, sources: sources, streams: make(map[string]*audioStream)}
	defer d.Close()
	var offline bytes.Buffer
	var measured media.AudioLevels
	if err := mixAudioObserved(ctx, plan, d, &offline, func(v media.AudioLevels) { measured = v }); err != nil {
		t.Fatal(err)
	}
	if len(d.streams) != 0 {
		t.Fatal("offline decoders not retired")
	}
	preview, levels := previewQualificationPCM(t, ctx, p, 0)
	if !bytes.Equal(preview, offline.Bytes()[:len(preview)]) {
		t.Fatal("real pre-AAC PCM differs between preview and export")
	}
	levels.Frames += plan.OutputAudioFrames - plan.ContentAudioFrames
	if measured != levels || measured.ClipCount() != 0 || measured.Peak[0] == 0 || measured.Peak[1] == 0 {
		t.Fatalf("incorrect real-media metering: %+v %+v", measured, levels)
	}
	seek, _ := previewQualificationPCM(t, ctx, p, 150000)
	if !bytes.Equal(seek, preview[7200*4:]) {
		t.Fatal("grid-aligned mid-clip seek changed canonical PCM")
	}
	offGrid, _ := previewQualificationPCM(t, ctx, p, 150001)
	if len(offGrid) != int(media.PCMFramesForTicks(p.EndTime()-150001))*4 {
		t.Fatal("off-grid seek has wrong run-relative duration")
	}
	for _, bounds := range [][2]int{{0, 2400}, {21600, int(plan.ContentAudioFrames)}} {
		for _, v := range preview[bounds[0]*4 : bounds[1]*4] {
			if v != 0 {
				t.Fatal("delayed source, gap or muted clip leaked audio")
			}
		}
	}
	out := filepath.Join(dir, "export.mp4")
	var submitted media.AudioLevels
	if err := RenderProjectObserved(ctx, engine, p, "hlg", 30, out, func(v Progress) {
		if v.Stage == StageAudio {
			submitted = v.AudioLevels
		}
	}); err != nil {
		t.Fatal(err)
	}
	if submitted != measured {
		t.Fatalf("published export submitted different mix: %+v %+v", submitted, measured)
	}
	raw, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", out, "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "s16le", "-").Output()
	if err != nil || len(raw) < offline.Len() {
		t.Fatalf("decode AAC: %d %v", len(raw), err)
	}
	var dot, x2, y2 float64
	for i := 0; i < offline.Len(); i += 2 {
		x := float64(int16(binary.LittleEndian.Uint16(offline.Bytes()[i:])))
		y := float64(int16(binary.LittleEndian.Uint16(raw[i:])))
		dot += x * y
		x2 += x * x
		y2 += y * y
	}
	correlation := dot / math.Sqrt(x2*y2)
	if !(correlation > 0.95) {
		t.Fatalf("AAC correlation %.6f, expected > 0.95 (not lossless)", correlation)
	}
	t.Logf("exact real preview/export PCM and grid-aligned seek; %s; AAC correlation %.6f", measured.String(), correlation)
}
