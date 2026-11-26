package composition

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

func TestCompositeQualificationRealMedia(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=10:d=1,format=yuv420p10le,geq=lum='128+4*X+3*Y+20*N':cb=480:cr=560,setsar=1", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", "-shortest", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	engine := media.Default()
	report, err := engine.Inspect(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := project.FromReport("asset", path, report)
	if err != nil || !asset.HasAudio {
		t.Fatalf("invalid HLG/AAC fixture: %v", err)
	}
	p := project.New()
	p.Assets = []project.Asset{asset}
	p.CanvasWidth, p.CanvasHeight = 64, 48
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: asset.ID, Out: 500000}}
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "overlay", AssetID: asset.ID, Out: 500000, Transform: &media.Transform{CanvasWidth: 64, CanvasHeight: 48, Scale: 50, FlipHorizontal: true}}}
	p, err = p.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "overlay"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	disk, err := mediacache.Open(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	still := media.NewStillDecoderWithCache(engine, disk)
	video := media.NewStreamDecoderWithCache(ctx, engine, disk)
	defer video.Close()
	audio := media.NewPCMDecoder(ctx, engine)
	defer audio.Close()
	plan, err := e.PlanLayers(0)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	ref, err := RenderStill(ctx, still, plan, CompositePreviewSide)
	if err != nil || !ref.Valid() {
		t.Fatalf("still: %v", err)
	}
	cold := time.Since(started)
	if disk.Stats().Entries == 0 {
		t.Fatal("still decoder did not persist timestamp index")
	}
	identities := make(map[string]mediacache.Source)
	started = time.Now()
	frame, err := renderCompositeFrame(ctx, e, video, 0, identities)
	if err != nil || !frame.Frame.Valid() {
		t.Fatalf("streamed composite: %v", err)
	}
	warm := time.Since(started)
	if !bytes.Equal(ref.Pixels, frame.Frame.Pixels) {
		t.Fatal("aligned still and streaming SDR pixels differ")
	}
	plan, err = e.PlanLayers(100000)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	next, err := renderCompositeFrame(ctx, e, video, 100000, identities)
	if err != nil || bytes.Equal(next.Frame.Pixels, frame.Frame.Pixels) {
		t.Fatalf("video did not advance: %v", err)
	}
	steady := time.Since(started)
	if video.Stats().Opened != 2 || video.Stats().Peak != 2 || video.Stats().Units != 4 {
		t.Fatalf("persistent video reuse: %+v", video.Stats())
	}
	mixed, err := MixPCM(ctx, e, audio, 0, 0, media.PCMBlockFrames)
	if err != nil {
		t.Fatal(err)
	}
	single := media.NewPCMDecoder(ctx, engine)
	defer single.Close()
	reference, err := single.DecodePCM(ctx, media.PCMRequest{ClipID: "reference", Path: path, Frames: media.PCMBlockFrames})
	if err != nil {
		t.Fatal(err)
	}
	nonzero := false
	for i, sample := range reference {
		nonzero = nonzero || sample != 0
		want := int16(max(-32768, min(32767, 2*int(sample))))
		if mixed.Samples[i] != want {
			t.Fatalf("two real audio streams differ from equal-gain sum at %d", i)
		}
	}
	if !nonzero || audio.Stats().Peak != 2 {
		t.Fatal("real audio mix did not decode two audible contributions")
	}
	if _, err = MixPCM(ctx, e, audio, 0, media.PCMBlockFrames, media.PCMBlockFrames); err != nil || audio.Stats().Opened != 2 {
		t.Fatalf("audio stream reuse: %v", err)
	}
	video.Close()
	audio.Close()
	single.Close()
	if v, a := video.Stats(), audio.Stats(); v.Opened != v.Retired || a.Opened != a.Retired {
		t.Fatalf("unretired processes: video=%+v audio=%+v", v, a)
	}
	final, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(original) != sha256.Sum256(final) {
		t.Fatalf("preview modified HLG source: %v", err)
	}
	t.Logf("synthetic HLG Main 10 source preserved; SDR still/stream exact match at 0; cold=%s cached-stream-start=%s steady-frame=%s video=%+v audio=%+v", cold, warm, steady, video.Stats(), audio.Stats())
}

type benchPreviewDecoder struct{}

func (benchPreviewDecoder) Decode(_ context.Context, req media.StillRequest) (media.StillFrame, error) {
	return media.StillFrame{Width: req.OutputWidth, Height: req.OutputHeight, Pixels: make([]byte, req.OutputWidth*req.OutputHeight*4)}, nil
}

func (benchPreviewDecoder) Retain([]string) error { return nil }

func (benchPreviewDecoder) DecodePCM(_ context.Context, req media.PCMRequest) ([]int16, error) {
	return make([]int16, req.Frames*media.PCMChannels), nil
}

func BenchmarkCompositePreview(b *testing.B) {
	p := project.New()
	p.Assets = []project.Asset{{ID: "a", Path: filepath.Join(b.TempDir(), "a.mp4"), Duration: 1000000, Width: 1920, Height: 1080, HasAudio: true}}
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", Out: 1000000}}
	var err error
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		b.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "top", AssetID: "a", Out: 1000000, Transform: &media.Transform{CanvasWidth: 1920, CanvasHeight: 1080, Scale: 50}}}
	e, err := Compile(p)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	d := benchPreviewDecoder{}
	b.Run("Blend640TwoLayers", func(b *testing.B) {
		plan, err := e.PlanLayers(0)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := RenderStill(ctx, d, plan, CompositePreviewSide); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("MixTwoAudioBlocks", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := MixPCM(ctx, e, d, 0, 0, media.PCMBlockFrames); err != nil {
				b.Fatal(err)
			}
		}
	})
}
