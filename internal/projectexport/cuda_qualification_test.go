package projectexport

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestExperimentalCUDAProjectExportParity(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_CUDA") != "1" {
		t.Skip("set PIXELCUT_QUALIFY_CUDA=1 on a CUDA workstation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	dir := t.TempDir()
	cpu, gpu := media.Default(), media.Default()
	gpu.DecodeBackend = media.BackendCUDA
	p, _ := cudaQualificationFixture(t, ctx, cpu, dir)
	prepared, err := Prepare(ctx, gpu, p, "hlg", 30)
	if err != nil {
		t.Fatal(err)
	}
	sources := make(map[string]Source)
	for _, s := range prepared.Sources {
		sources[s.Asset.ID] = s
	}
	plan := prepared.Plan
	black := black444(plan.Contract.Canvas)
	frame := make([]byte, len(black))
	var ci, gi renderIndexes
	for _, span := range plan.Spans {
		var c, g bytes.Buffer
		if err := renderSpan(ctx, cpu, &ci, sources, plan, span, &c, black, frame); err != nil {
			t.Fatal(err)
		}
		if err := renderSpan(ctx, gpu, &gi, sources, plan, span, &g, black, frame); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(c.Bytes(), g.Bytes()) {
			t.Fatalf("CUDA pre-encode HLG frame mismatch in span %d", span.Start)
		}
	}
	out := filepath.Join(dir, "cuda.mp4")
	if err := RenderProject(ctx, gpu, p, "hlg", 30, out); err != nil {
		t.Fatal(err)
	}
	report, err = cpu.Inspect(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyProjectOutput(plan, report); err != nil {
		t.Fatal(err)
	}
	if err := prepared.VerifySources(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("CUDA pre-encode HLG pixels matched CPU across VFR cuts, gaps and translucent layers; output contract passed")
}

func cudaQualificationFixture(t *testing.T, ctx context.Context, engine media.Engine, dir string) (project.Project, string) {
	t.Helper()
	src := filepath.Join(dir, "source.mp4")
	cmd := exec.CommandContext(ctx, engine.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=10:d=1,format=yuv420p10le,geq=lum='64+mod(17*X+13*Y+31*N,877)':cb='64+mod(7*X+11*Y+13*N,897)':cr='64+mod(9*X+19*Y+17*N,897)',setsar=1", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=1", "-vf", "select=not(eq(n\\,2))", "-fps_mode", "passthrough", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	report, err := engine.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	a, err := project.FromReport("a", src, report)
	if err != nil {
		t.Fatal(err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.CanvasWidth, p.CanvasHeight = 192, 144
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", Start: 10001, In: 50000, Out: 450001}}
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "top", AssetID: "a", Start: 150000, In: 300000, Out: 500000, Transform: &media.Transform{CanvasWidth: 192, CanvasHeight: 144, Scale: 50, FlipHorizontal: true}}}
	p, err = p.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "top"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	return p, src
}
