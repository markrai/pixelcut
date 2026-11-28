package composition

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

func previewTransitionFixture(t *testing.T) project.Project {
	t.Helper()
	dir := t.TempDir()
	p := project.New()
	for _, a := range []project.Asset{
		{ID: "a", Path: filepath.Join(dir, "base.mp4"), Duration: 1000000, Width: 8, Height: 8, HasAudio: true},
		{ID: "b", Path: filepath.Join(dir, "next.mp4"), Duration: 1000000, Width: 8, Height: 8, HasAudio: true},
	} {
		if err := os.WriteFile(a.Path, []byte(a.ID), 0600); err != nil {
			t.Fatal(err)
		}
		p.Assets = append(p.Assets, a)
	}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: "a", Out: 250000}, {ID: "next", AssetID: "b", Start: 250000, In: 400000, Out: 800000}}
	q, err := p.SetVideoTransition(project.ClipRef{TrackID: "V1", ClipID: "next"}, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first", Duration: 200000})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestSDRTransitionBlend(t *testing.T) {
	from := []byte{0, 0, 0, 255}
	to := []byte{255, 255, 255, 255}
	dst := make([]byte, 4)
	for _, tc := range []struct {
		weight int64
		want   byte
	}{{0, 0}, {500000, 180}, {1000000, 255}} {
		if err := blendSDRTransition(context.Background(), dst, from, to, 1, 1, tc.weight); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(dst, []byte{tc.want, tc.want, tc.want, 255}) {
			t.Fatalf("weight %d: %v", tc.weight, dst)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := blendSDRTransition(ctx, dst, from, to, 1, 1, 500000); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if err := blendSDRTransition(context.Background(), dst, from[:3], to, 1, 1, 500000); err == nil {
		t.Fatal("invalid buffer accepted")
	}
}

func TestTransitionPreviewSeekAndLifecycle(t *testing.T) {
	p := previewTransitionFixture(t)
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	d := &sequenceDecoder{}
	identities := make(map[string]mediacache.Source)
	for _, tc := range []struct {
		at, until int64
		ids       []string
		pixel     [4]byte
	}{{200000, 250000, []string{"first"}, [4]byte{200, 0, 0, 255}},
		{250000, 450000, []string{"next", "first"}, [4]byte{200, 0, 0, 255}},
		{350000, 450000, []string{"next", "first"}, [4]byte{140, 0, 140, 255}},
		{450000, 650000, []string{"next"}, [4]byte{0, 0, 200, 255}},
		{650000, 650000, nil, [4]byte{0, 0, 0, 255}}} {
		f, err := renderCompositeFrame(context.Background(), e, d, tc.at, identities)
		if err != nil || !f.Frame.Valid() || f.ValidUntil != tc.until || !reflect.DeepEqual(d.active[len(d.active)-1], tc.ids) {
			t.Fatalf("frame at %d: %+v %v active=%v", tc.at, f, err, d.active)
		}
		if len(f.Sources) != len(tc.ids) || !bytes.Equal(f.Frame.Pixels[:4], tc.pixel[:]) {
			t.Fatalf("frame at %d: sources=%v pixel=%v want=%v", tc.at, f.Sources, f.Frame.Pixels[:4], tc.pixel)
		}
	}
	if e.NextVideoSample(0, 200000) != 250000 || e.NextVideoSample(0, 400000) != 450000 || e.VideoSampleAt(0, 460000) != 450000 {
		t.Fatal("off-grid transition boundaries skipped")
	}
	d = &sequenceDecoder{}
	plan, err := e.PlanLayers(350000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderStill(context.Background(), d, plan, 8); err != nil || len(d.calls) != 2 || d.calls[0].Tick != 500000 || d.calls[1].Tick != 350000 {
		t.Fatalf("seek mapping: %+v %v", d.calls, err)
	}
	f, err := e.At(350000)
	if err != nil || len(f.Audio) != 1 || f.Audio[0].Ref.ClipID != "next" || f.Audio[0].SourceTime != 500000 {
		t.Fatalf("video handle extended audio: %+v %v", f.Audio, err)
	}
	if err := os.WriteFile(p.Assets[0].Path, []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := renderCompositeFrame(context.Background(), e, d, 350000, identities); err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("outgoing source change ignored: %v", err)
	}
}

func TestTransitionPreviewRejectsUnsupportedCoverage(t *testing.T) {
	p := previewTransitionFixture(t)
	q, err := p.SetClipOpacity(project.ClipRef{TrackID: "V1", ClipID: "first"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlanLayers(350000); err == nil || !strings.Contains(err.Error(), "opaque full-canvas") {
		t.Fatalf("unsupported coverage accepted: %v", err)
	}
}

func TestTransitionPreviewRealMedia(t *testing.T) {
	physical := os.Getenv("PIXELCUT_QUALIFY_TRANSITION_PREVIEW_GPU") == "1"
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			if physical {
				t.Fatal(err)
			}
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "fixture.mp4")
	engine := media.Default()
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=1,format=yuv420p10le,geq=lum='if(lt(N,2),280,if(lt(N,6),64,940))':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Inspect(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := project.FromReport("a", path, report)
	if err != nil {
		t.Fatal(err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.Tracks[0].Clips = []project.Clip{{ID: "first", AssetID: a.ID, Out: 200000}, {ID: "next", AssetID: a.ID, Start: 200000, In: 600000, Out: 1000000}}
	q, err := p.SetVideoTransition(project.ClipRef{TrackID: "V1", ClipID: "next"}, &project.VideoTransition{Kind: project.CrossDissolve, FromClipID: "first", Duration: 200000})
	if err != nil {
		t.Fatal(err)
	}
	e, err := Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	modes := []media.Backend{media.BackendCPU}
	if physical {
		modes = append(modes, media.BackendCUDA)
	}
	var reference [][]byte
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			engine.DecodeBackend = mode
			still := media.NewStillDecoder(engine)
			stream := media.NewStreamDecoder(ctx, engine)
			defer stream.Close()
			identities := make(map[string]mediacache.Source)
			var values []byte
			for i, at := range []int64{100000, 200000, 300000, 400000, 500000, 600000} {
				plan, err := e.PlanLayers(at)
				if err != nil {
					t.Fatal(err)
				}
				s, err := RenderStill(ctx, still, plan, CompositePreviewSide)
				if err != nil {
					t.Fatal(err)
				}
				f, err := renderCompositeFrame(ctx, e, stream, at, identities)
				if err != nil || !bytes.Equal(s.Pixels, f.Frame.Pixels) {
					t.Fatalf("still/stream at %d: %v", at, err)
				}
				if mode == media.BackendCPU {
					reference = append(reference, bytes.Clone(s.Pixels))
				} else if !bytes.Equal(reference[i], s.Pixels) {
					t.Fatalf("CPU/CUDA preview mismatch at %d", at)
				}
				values = append(values, s.Pixels[0])
			}
			if !(values[1] < values[0] && values[1] < values[2] && values[2] < values[3] && values[3] == values[4] && values[5] == 0) {
				t.Fatalf("transition progression: %v", values)
			}
			stream.Close()
			stats := stream.Stats()
			if stats.Opened != 2 || stats.Retired != 2 || stats.Peak != 2 || stats.Units != 7 {
				t.Fatalf("unexpected transition decoder lifecycle: %+v", stats)
			}
			t.Logf("aligned still/stream parity, transition values=%v stats=%+v", values, stats)
		})
		if t.Failed() {
			return
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("source changed")
	}
}
