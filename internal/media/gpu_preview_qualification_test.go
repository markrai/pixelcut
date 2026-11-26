package media

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreviewChromaControlGraph(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{1920, 1080}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	req := StillRequest{Path: "source.mp4", Width: 1920, Height: 1080, Canvas: CanvasSize{1920, 1080}, OutputWidth: 960, OutputHeight: 540}
	g, err := previewChromaControlGraph(v, req, 2, 4, "left", "center")
	if err != nil || !strings.HasPrefix(g, "scale=960:540:flags=lanczos:in_chroma_loc=left:out_chroma_loc=center,") || !strings.Contains(g, "select=between(n\\,2\\,5)") || strings.Contains(g, "tonemap") {
		t.Fatalf("invalid chroma control: %q %v", g, err)
	}
	if _, err := previewChromaControlGraph(v, req, 2, 4, "unknown", "left"); err == nil {
		t.Fatal("invalid location accepted")
	}
}

func TestPreviewBenchmarkMedian(t *testing.T) {
	values := []float64{3, 1, 2}
	if medianPreviewSeconds(values) != 2 || values[0] != 3 || medianPreviewSeconds([]float64{4, 2}) != 3 || medianPreviewSeconds(nil) != 0 {
		t.Fatal("incorrect benchmark median")
	}
}

func TestPreviewQualificationChromaFixtures(t *testing.T) {
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	physical := os.Getenv("PIXELCUT_QUALIFY_CUDA_CHROMA") == "1"
	for _, fixture := range []struct {
		name, cb, cr string
	}{
		{"edges", "if(lt(X,48+N),256,768)", "if(lt(Y,36+N),768,256)"},
		{"impulses", "if(eq(X,47+N)*eq(Y,35),896,512)", "if(eq(X,49)*eq(Y,37+N),128,512)"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "chroma.mp4")
			filter := "nullsrc=s=192x144:r=10:d=0.4,format=yuv420p10le,geq=lum='512':cb='" + fixture.cb + "':cr='" + fixture.cr + "',setsar=1"
			runFF(t, "-v", "error", "-f", "lavfi", "-i", filter, "-c:v", "libx265", "-x265-params", "lossless=1:pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
			e, tools, err := Default().previewQualificationTools(ctx)
			if err != nil || tools.FFmpegVersion == "" || tools.FFprobeVersion == "" || !filepath.IsAbs(tools.FFmpegPath) || !filepath.IsAbs(tools.FFprobePath) {
				t.Fatalf("qualification tool identity: %+v %v", tools, err)
			}
			report, err := e.Inspect(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			v, err := report.Video()
			if err != nil {
				t.Fatal(err)
			}
			index, err := e.ExportFrames(ctx, path)
			if err != nil || index.Count() != 4 {
				t.Fatalf("fixture index: %v", err)
			}
			req := StillRequest{Path: path, Width: 192, Height: 144, Canvas: CanvasSize{192, 144}, OutputWidth: 96, OutputHeight: 72}
			graphs, err := buildPreviewStageGraphs(v, req, 0, 4)
			if err != nil {
				t.Fatal(err)
			}
			args := previewPairArgs(path, []string{"-noautorotate", "-hwaccel", "none", "-c:v", "hevc"}, graphs.reference, 4, "yuv420p10le")
			q, err := e.comparePreviewPlanes(ctx, req, 0, index.ticks, args, args)
			if err != nil || q.ExactFrames != 4 || q.Y.Samples != 96*72*4 {
				t.Fatalf("CPU chroma fixture: %+v %v", q, err)
			}
			help, err := exec.CommandContext(ctx, e.FFmpeg, "-hide_banner", "-h", "filter=scale").CombinedOutput()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(help), "in_chroma_loc") && strings.Contains(string(help), "out_chroma_loc") {
				left, err := previewChromaControlGraph(v, req, 0, 4, "left", "left")
				if err != nil {
					t.Fatal(err)
				}
				center, err := previewChromaControlGraph(v, req, 0, 4, "left", "center")
				if err != nil {
					t.Fatal(err)
				}
				input := []string{"-noautorotate", "-hwaccel", "none", "-c:v", "hevc"}
				controls, err := e.comparePreviewPlanes(ctx, req, 0, index.ticks, previewPairArgs(path, input, left, 4, "yuv420p10le"), previewPairArgs(path, input, center, 4, "yuv420p10le"))
				if err != nil || controls.Compared != 4 || controls.U.DifferentSamples == 0 {
					t.Fatalf("explicit CPU chroma location control: %+v %v", controls, err)
				}
			} else {
				t.Log("explicit named chroma location controls unavailable in this FFmpeg; CPU baseline and timing still exercised")
			}
			bgra, err := previewCheckArgs(e, v, req, 0, 4, false)
			if err != nil {
				t.Fatal(err)
			}
			if seconds, err := e.timePreviewPath(ctx, bgra, req, index.ticks); err != nil || seconds <= 0 {
				t.Fatalf("CPU independent timing: %f %v", seconds, err)
			}
			if physical {
				r, err := e.CheckCUDAPreviewScaleWithOptions(ctx, path, 0, 4, 96, 72, PreviewScaleOptions{Chroma: true, Benchmark: true})
				raw, marshalErr := json.MarshalIndent(r, "", "  ")
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				t.Log(string(raw))
				if err != nil {
					t.Errorf("physical chroma qualification: %v", err)
				}
			}
		})
	}
}
