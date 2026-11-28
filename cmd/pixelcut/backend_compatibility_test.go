package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestExportCLIWithoutNVIDIA(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("CPU-only FFmpeg shim uses POSIX shell")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip(err)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip(err)
	}
	ffmpeg, err = filepath.Abs(ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=192x144:r=30:d=0.5,format=yuv420p10le,geq=lum='180+20*N':cb=512:cr=512,setsar=1", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=0.5", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", "-c:a", "aac", src)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	engine := media.Default()
	report, err := engine.Inspect(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	a, err := project.FromReport("a", src, report)
	if err != nil || !a.HasAudio {
		t.Fatalf("fixture import: %v", err)
	}
	p := project.New()
	p.Assets = []project.Asset{a}
	p.CanvasWidth, p.CanvasHeight = a.Width, a.Height
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: a.ID, Out: 400000}}
	projectPath := filepath.Join(dir, "fixture.pixelcut.json")
	if err := p.SaveAs(projectPath); err != nil {
		t.Fatal(err)
	}
	originalProject, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	originalSource, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(dir, "trace.txt")
	shim := "#!/bin/sh\ndecode=no\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    hevc_nvenc) printf 'nvenc\\n' >> \"$PIXELCUT_TEST_FFMPEG_TRACE\"; printf 'simulated NVENC unavailable\\n' >&2; exit 1;;\n    cuda) printf 'cuda\\n' >> \"$PIXELCUT_TEST_FFMPEG_TRACE\"; printf 'simulated CUDA unavailable\\n' >&2; exit 1;;\n    libx265) printf 'x265\\n' >> \"$PIXELCUT_TEST_FFMPEG_TRACE\";;\n    -noautorotate) decode=yes;;\n  esac\ndone\nif [ \"$decode\" = yes ]; then\n  printf 'software-decode\\n' >> \"$PIXELCUT_TEST_FFMPEG_TRACE\"\nfi\nexec \"$PIXELCUT_TEST_REAL_FFMPEG\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte(shim), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXELCUT_TEST_REAL_FFMPEG", ffmpeg)
	t.Setenv("PIXELCUT_TEST_FFMPEG_TRACE", trace)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		name      string
		options   []string
		fail      bool
		reject    string
		forbidden string
	}{
		{"default", nil, false, "", ""},
		{"cuda", []string{"-decode", "cuda"}, true, "cuda", "software-decode"},
		{"nvenc", []string{"-encode", "nvenc"}, true, "nvenc", "x265"},
		{"combined", []string{"-decode", "cuda", "-encode", "nvenc"}, true, "", "x265"},
		{"cpu-recovery", []string{"-decode", "cpu", "-encode", "cpu"}, false, "", ""},
		{"auto-cpu", []string{"-decode", "auto"}, false, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(trace, nil, 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, tc.name+".mp4")
			args := append([]string{"export-project", "-color", "hlg", "-fps", "30", "-progress"}, tc.options...)
			args = append(args, projectPath, out)
			var log bytes.Buffer
			err := run(ctx, args, &log)
			raw, readErr := os.ReadFile(trace)
			if readErr != nil {
				t.Fatal(readErr)
			}
			calls := strings.Fields(string(raw))
			contains := func(value string) bool {
				for _, call := range calls {
					if call == value {
						return true
					}
				}
				return false
			}
			if tc.fail {
				if err == nil || ctx.Err() != nil || strings.Contains(log.String(), "\ndone:") {
					t.Fatalf("hardware request did not fail closed: %v %s", err, log.String())
				}
				if tc.reject != "" && !contains(tc.reject) || tc.reject == "" && !contains("cuda") && !contains("nvenc") || contains(tc.forbidden) || tc.name == "combined" && contains("software-decode") {
					t.Fatalf("wrong backend or fallback: %v, error: %v", calls, err)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("failed hardware export published output")
				}
			} else {
				if err != nil || !contains("x265") || !contains("software-decode") || contains("cuda") || contains("nvenc") {
					t.Fatalf("CPU export: %v calls=%v %s", err, calls, log.String())
				}
				r, err := engine.Inspect(ctx, out)
				if err != nil {
					t.Fatal(err)
				}
				v, err := r.Video()
				if err != nil || v.CodecName != "hevc" || v.PixelFormat != "yuv420p10le" || v.Transfer != "arib-std-b67" {
					t.Fatalf("CPU output contract: %+v %v", v, err)
				}
			}
			files, err := filepath.Glob(filepath.Join(dir, ".pixelcut-*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("staged outputs leaked: %v %v", files, err)
			}
		})
	}
	out := filepath.Join(dir, "video-only.mp4")
	var log bytes.Buffer
	args := []string{"export-video", "-color", "hlg", "-fps", "30", projectPath, out}
	if err := run(ctx, args, &log); err == nil {
		t.Fatal("video-only export silently discarded enabled audio")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("video-only rejection published output")
	}
	args = append([]string{"export-video", "-discard-audio"}, args[1:]...)
	if err := run(ctx, args, &log); err != nil {
		t.Fatalf("explicit audio discard: %v", err)
	}
	videoOnly, err := engine.Inspect(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range videoOnly.Streams {
		if stream.CodecType == "audio" {
			t.Fatal("video-only export contains audio")
		}
	}
	for path, want := range map[string][]byte{src: originalSource, projectPath: originalProject} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(want, got) {
			t.Fatalf("input changed: %s", path)
		}
	}
}
