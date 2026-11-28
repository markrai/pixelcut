package project

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

func importSDRReport() media.Report {
	return media.Report{Streams: []media.Stream{
		{CodecType: "video", CodecName: "h264", PixelFormat: "yuv420p", Width: 1920, Height: 1080, Duration: "2", ChromaLocation: "left"},
		{CodecType: "audio", CodecName: "aac"},
	}}
}

func TestClassifiedImportOnlyProject(t *testing.T) {
	p := New()
	p.Version = TransitionVersion
	path := filepath.Join(t.TempDir(), "screen.mp4")
	report := importSDRReport()
	a, err := FromImportReport("screen", path, report)
	if err != nil || !a.ImportOnly() || a.ColorFamily != media.ColorSDR709 || a.ColorDefaults == 0 || a.Range != "tv" || !a.HasAudio {
		t.Fatalf("import: %+v %v", a, err)
	}
	if _, err := FromReport(a.ID, path, report); err == nil {
		t.Fatal("strict HLG report entry point accepted SDR")
	}
	batch, err := p.ImportPaths(context.Background(), []string{path, path}, func(context.Context, string) (Asset, error) { return a, nil })
	if err != nil || batch.Added != 1 || batch.Duplicates != 1 || batch.Project.Version != Version || p.Version != TransitionVersion || len(p.Assets) != 0 {
		t.Fatalf("batch: %+v %v", batch, err)
	}
	q := batch.Project
	for _, track := range []string{"V1", "A1"} {
		r, err := q.PlaceAssets(track, []string{a.ID}, []string{"clip"}, 0, -1, 0)
		if !errors.Is(err, ErrImportOnlySource) || r.Project.Fingerprint() != q.Fingerprint() {
			t.Fatalf("import-only placement on %s: %v", track, err)
		}
	}
	forged := q.Clone()
	forged.Tracks[0].Clips = []Clip{{ID: "clip", AssetID: a.ID, Out: a.Duration}}
	if !errors.Is(forged.Validate(), ErrImportOnlySource) {
		t.Fatal("direct project clip bypassed placement guard")
	}
	old := q.Clone()
	old.Version = TransitionVersion
	if !errors.Is(old.Validate(), ErrUnsupportedVersion) {
		t.Fatal("version 10 accepted new color descriptor")
	}
	file := filepath.Join(t.TempDir(), "mixed-bin.pixelcut.json")
	if err := q.SaveAs(file); err != nil {
		t.Fatal(err)
	}
	got, err := Load(file)
	if err != nil || got.Fingerprint() != q.Fingerprint() || got.Assets[0] != a {
		t.Fatalf("round trip: %v", err)
	}
	changed := a
	changed.ColorDefaults = 0
	if compatibleAssetMetadata(a, changed) {
		t.Fatal("different inference provenance treated as identical")
	}
}

func TestClassifiedImportStreamAndRelinkSafety(t *testing.T) {
	dir := t.TempDir()
	r := importSDRReport()
	path := filepath.Join(dir, "first.mp4")
	a, err := FromImportReport("a", path, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Streams = append(r.Streams, media.Stream{CodecType: "audio", CodecName: "aac"})
	if _, err := FromImportReport("bad", path, r); err == nil {
		t.Fatal("multiple audio streams accepted")
	}
	p, _, _, err := New().AddAsset(a)
	if err != nil {
		t.Fatal(err)
	}
	candidate := a
	candidate.Path = filepath.Join(dir, "replacement.mp4")
	if err := os.WriteFile(candidate.Path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := candidate
	changed.ColorDefaults = 0
	if _, err := p.RelinkAsset(a.ID, changed, false); err == nil {
		t.Fatal("relink changed inference contract")
	}
	q, err := p.RelinkAsset(a.ID, candidate, false)
	if err != nil || q.Assets[0] != candidate {
		t.Fatalf("same descriptor relink: %v", err)
	}
}

func TestVersion10TransitionCompatibility(t *testing.T) {
	p := transitionFixture(t)
	q, err := p.SetVideoTransition(ClipRef{TrackID: "V1", ClipID: "incoming"}, &VideoTransition{Kind: CrossDissolve, FromClipID: "outgoing", Duration: 500000})
	if err != nil {
		t.Fatal(err)
	}
	q.Version = TransitionVersion
	file := filepath.Join(t.TempDir(), "version10.pixelcut.json")
	if err := q.SaveAs(file); err != nil {
		t.Fatal(err)
	}
	got, err := Load(file)
	if err != nil || got.Version != TransitionVersion || got.Fingerprint() != q.Fingerprint() {
		t.Fatalf("existing transition project changed: %v", err)
	}
}

func TestClassifiedImportRealSDR(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := media.Default()
	path := filepath.Join(t.TempDir(), "screen.mp4")
	cmd := exec.CommandContext(ctx, e.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=10:duration=0.3", "-c:v", "libx264", "-threads", "2", "-pix_fmt", "yuv420p", path)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("SDR fixture: %v %s", err, b)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.CachedImportReport(ctx, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := FromImportReport("screen", path, r)
	if err != nil || a.ColorFamily != media.ColorSDR709 || a.ColorDefaults == 0 || a.Width != 1280 || a.Height != 720 {
		t.Fatalf("real SDR classification: %+v %v", a, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("source changed during import")
	}
	t.Logf("real H264 SDR source: %+v", a.SourceColor())
}
