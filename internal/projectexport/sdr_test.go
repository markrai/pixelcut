package projectexport

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestSDRMappingPatches(t *testing.T) {
	for _, tc := range []struct {
		input, want ycbcr10
	}{
		{ycbcr10{64, 512, 512}, ycbcr10{64, 512, 512}},
		{ycbcr10{940, 512, 512}, ycbcr10{721, 512, 512}},
		{ycbcr10{502, 512, 512}, ycbcr10{462, 512, 512}},
		{ycbcr10{250, 409, 960}, ycbcr10{360, 405, 705}},
		{ycbcr10{690, 167, 105}, ycbcr10{631, 330, 429}},
		{ycbcr10{127, 960, 471}, ycbcr10{201, 784, 530}},
		{ycbcr10{0, 512, 512}, ycbcr10{64, 512, 512}},
		{ycbcr10{1023, 512, 512}, ycbcr10{721, 512, 512}},
	} {
		if got := mapSDR709(tc.input); absCode(got.Y, tc.want.Y) > 1 || absCode(got.Cb, tc.want.Cb) > 1 || absCode(got.Cr, tc.want.Cr) > 1 {
			t.Fatalf("SDR patch %+v: got %+v want %+v", tc.input, got, tc.want)
		}
	}
	c := media.CanvasSize{Width: 6, Height: 4}
	b := media.Placement{Width: c.Width, Height: c.Height}
	sdr := solidLayer(b, 100, ycbcr10{940, 512, 512})
	ctx := context.Background()
	if err := convertSDRFrame(ctx, sdr.Pixels, b.Width, b.Height); err != nil {
		t.Fatal(err)
	}
	want := solidLayer(b, 100, ycbcr10{721, 512, 512})
	if !bytes.Equal(sdr.Pixels, want.Pixels) {
		t.Fatal("SDR reference white conversion failed")
	}
	black := solidLayer(b, 100, ycbcr10{64, 512, 512})
	dst := black444(c)
	sdr.Opacity = 50
	if err := compositeHLG(ctx, dst, c, []layerFrame{black, sdr}); err != nil || absCode(readPixel(dst, c.Width*c.Height*2, 0).Y, 596) > 1 {
		t.Fatalf("mixed opacity: %v", err)
	}
	sdr.Opacity = 100
	d := dissolveFrame{index: 0, from: black.Pixels, weight: 500000}
	if err := compositeHLGDissolve(ctx, dst, c, []layerFrame{sdr}, &d); err != nil || absCode(readPixel(dst, c.Width*c.Height*2, 0).Y, 596) > 1 {
		t.Fatalf("mixed dissolve: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := convertSDRFrame(cancelled, sdr.Pixels, b.Width, b.Height); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled conversion: %v", err)
	}
	if err := convertSDRFrame(ctx, sdr.Pixels[:3], b.Width, b.Height); err == nil {
		t.Fatal("invalid buffer accepted")
	}
}

func TestMixedExportPreflightDescriptor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "screen.mp4")
	if err := os.WriteFile(path, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	v := media.Stream{CodecType: "video", CodecName: "h264", PixelFormat: "yuv420p", Width: 1280, Height: 720, Duration: "1"}
	report := media.Report{Streams: []media.Stream{v}}
	a, err := project.FromImportReport("sdr", path, report)
	if err != nil || a.ColorDefaults == 0 {
		t.Fatalf("inferred fixture: %v", err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.Tracks[0].Clips = []project.Clip{{ID: "clip", AssetID: a.ID, Out: a.Duration}}
	i := &fixtureInspector{report: report}
	ctx := context.Background()
	if _, err := Prepare(ctx, i, p, "hlg", 30); !errors.Is(err, project.ErrImportOnlySource) {
		t.Fatalf("unconfirmed source: %v", err)
	}
	p.Assets[0].ColorConfirmed = true
	prepared, err := Prepare(ctx, i, p, "hlg", 30)
	if err != nil || len(prepared.Sources) != 1 || prepared.Sources[0].Chroma != "left" || prepared.Sources[0].Asset.Range != "tv" || !prepared.Sources[0].Asset.ColorConfirmed || !strings.Contains(strings.Join(prepared.Plan.Warnings, " "), SDRMappingDescription) {
		t.Fatalf("confirmed source preflight: %v", err)
	}
	engine := media.Default()
	engine.DecodeBackend = media.BackendCUDA
	if exportDecodeEngine(engine, a).DecodeBackend != media.BackendCPU || exportDecodeEngine(engine, project.Asset{}).DecodeBackend != media.BackendCUDA {
		t.Fatal("incorrect source-specific decode policy")
	}
	i.report.Streams[0].Primaries = "bt709"
	if _, err := Prepare(ctx, i, p, "hlg", 30); err == nil {
		t.Fatal("changed inferred descriptor accepted")
	}
	i.report.Streams[0].Primaries = "bt2020"
	if _, err := Prepare(ctx, i, p, "hlg", 30); err == nil {
		t.Fatal("contradictory color tags accepted")
	}
}
