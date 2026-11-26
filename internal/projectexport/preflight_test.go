package projectexport

import (
	"context"
	"os"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

type fixtureInspector struct {
	report media.Report
	calls  int
}

func (i *fixtureInspector) Inspect(_ context.Context, _ string) (media.Report, error) {
	i.calls++
	return i.report, nil
}

func TestPrepareRevalidatesSourceAndRecordsNormalization(t *testing.T) {
	p := exportFixture(t)
	path := p.Assets[0].Path
	if err := os.WriteFile(path, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	c := media.ExportContract{Color: "hlg", Canvas: media.CanvasSize{Width: 64, Height: 48}, FPS: 30}
	video := c.ExpectedVideo()
	video.Duration = "1.000000"
	i := &fixtureInspector{report: media.Report{Streams: []media.Stream{video, {CodecType: "audio", CodecName: "aac"}}}}
	ctx := context.Background()
	prepared, err := Prepare(ctx, i, p, "hlg", 30)
	if err != nil || i.calls != 1 || len(prepared.Sources) != 1 {
		t.Fatalf("preflight: %v calls=%d", err, i.calls)
	}
	if err := os.WriteFile(path, []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepared.VerifySources(ctx); err == nil {
		t.Fatal("changed source accepted after preflight")
	}
	i.report.Streams[0].ChromaLocation = "center"
	normalized, err := Prepare(ctx, i, p, "hlg", 30)
	if err != nil || normalized.Sources[0].Chroma != "center" || len(normalized.Plan.Warnings) <= len(prepared.Plan.Warnings) {
		t.Fatalf("center-sited normalization not recorded: %v", err)
	}
	i.report.Streams[0].Range = "pc"
	if _, err := Prepare(ctx, i, p, "hlg", 30); err == nil {
		t.Fatal("changed declared range accepted")
	}
	i.report.Streams[0] = video
	i.report.Streams[0].Width = 128
	if _, err := Prepare(ctx, i, p, "hlg", 30); err == nil {
		t.Fatal("changed geometry accepted")
	}
	i.report.Streams[0] = video
	i.report.Streams = append(i.report.Streams, media.Stream{CodecType: "subtitle"})
	if _, err := Prepare(ctx, i, p, "hlg", 30); err == nil {
		t.Fatal("unsupported auxiliary stream accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Prepare(cancelled, i, p, "hlg", 30); err == nil {
		t.Fatal("cancelled preflight accepted")
	}
}
