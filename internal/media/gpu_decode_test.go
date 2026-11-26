package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHEVCDecodeCheckExplicitHardwareAndTransfer(t *testing.T) {
	args, err := hevcDecodeCheckArgs("source.mp4", 2, 4, true)
	if err != nil || !slices.Contains(args, "cuda") || !slices.Contains(args, "-hwaccel_output_format") || !slices.Contains(args, "hwdownload,format=p010le,format=yuv420p10le,select=between(n\\,2\\,5)") || slices.Contains(args, "-ss") {
		t.Fatalf("incorrect CUDA args: %v %v", args, err)
	}
	args, err = hevcDecodeCheckArgs("source.mp4", 2, 4, false)
	if err != nil || !slices.Contains(args, "none") || slices.Contains(args, "cuda") || !slices.Contains(args, "format=yuv420p10le,select=between(n\\,2\\,5)") {
		t.Fatalf("incorrect CPU args: %v %v", args, err)
	}
	for _, tc := range [][2]int{{-1, 1}, {0, 0}, {0, 301}, {MaxIndexedFrames, 1}} {
		if _, err := hevcDecodeCheckArgs("source.mp4", tc[0], tc[1], true); err == nil {
			t.Fatal("invalid interval accepted")
		}
	}
}

func TestDecodeHashContract(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	raw := "#hash: SHA256\r\n#tb 0: 1/1000000\r\n#dimensions 0: 64x48\r\n0, 100000, 100000, 33333, 9216, " + sum + "\r\n0, 166667, 166667, 33333, 9216, " + sum + "\r\n"
	h, err := parseDecodeFrameHashes(raw, 64, 48, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := compareDecodeHashes(h, h, []int64{100000, 166667}, 2); err != nil || n != 2 {
		t.Fatalf("equal frames: %d %v", n, err)
	}
	changed := slices.Clone(h)
	changed[1].hash = strings.Repeat("cd", 32)
	if n, err := compareDecodeHashes(h, changed, []int64{100000, 166667}, 2); err == nil || n != 1 || !strings.Contains(err.Error(), "frame 3") {
		t.Fatal("pixel mismatch not identified")
	}
	changed = slices.Clone(h)
	changed[1].pts++
	if n, err := compareDecodeHashes(h, changed, []int64{100000, 166667}, 2); err == nil || n != 1 {
		t.Fatal("timestamp mismatch accepted")
	}
	if _, err := compareDecodeHashes(h, h, []int64{100000, 166670}, 2); err == nil {
		t.Fatal("source index mismatch accepted")
	}
	for _, bad := range []string{
		strings.Replace(raw, "9216", "4608", 1),
		strings.Replace(raw, "166667, 166667", "100000, 100000", 1),
		strings.Replace(raw, "1/1000000", "1/30", 1),
		strings.Replace(raw, sum, "invalid", 1),
		raw + "0, 200000, 200000, 33333, 9216, " + sum,
	} {
		if _, err := parseDecodeFrameHashes(bad, 64, 48, 2); err == nil {
			t.Fatal("invalid frame output accepted")
		}
	}
}

func TestHEVCDecodeCheckRealVFR(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "vfr.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=64x48:r=30:d=0.4,format=yuv420p10le,geq=lum='64+mod(17*X+13*Y+31*N,877)':cb='64+mod(7*X+11*Y+13*N,897)':cr='64+mod(9*X+19*Y+17*N,897)',select='not(eq(mod(n,4),1))',setsar=1", "-fps_mode", "vfr", "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", path)
	engine := Default()
	report, err := engine.Inspect(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := report.Video()
	if err != nil || v.ValidateHLG() != nil {
		t.Fatalf("invalid fixture: %v", err)
	}
	index, err := engine.ExportFrames(ctx, path)
	if err != nil || index.Count() != 9 {
		t.Fatalf("unexpected VFR index: %v", err)
	}
	full, err := engine.decodeCheckFrames(ctx, path, v, 0, 9, false)
	if err != nil {
		t.Fatal(err)
	}
	window, err := engine.decodeCheckFrames(ctx, path, v, 2, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := compareDecodeHashes(full[2:6], window, index.ticks[2:6], 2); err != nil || n != 4 {
		t.Fatalf("VFR ordinal selection: %d %v", n, err)
	}
	if window[2].pts-window[1].pts == window[1].pts-window[0].pts {
		t.Fatal("fixture did not exercise VFR")
	}
	for _, size := range []scaleCheckSize{{64, 48}, {32, 24}, {128, 96}} {
		scaled, err := engine.frameCheckFrames(ctx, path, v, 0, 9, false, &size)
		if err != nil {
			t.Fatal(err)
		}
		part, err := engine.frameCheckFrames(ctx, path, v, 2, 4, false, &size)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := compareDecodeHashes(scaled[2:6], part, index.ticks[2:6], 2); err != nil || n != 4 {
			t.Fatalf("scaled VFR ordinal selection: %d %v", n, err)
		}
		if size.width == v.Width && size.height == v.Height {
			if _, err := compareDecodeHashes(full, scaled, index.ticks, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	req := StillRequest{Path: path, Width: v.Width, Height: v.Height, Canvas: CanvasSize{v.Width, v.Height}, OutputWidth: 32, OutputHeight: 24}
	q, n, err := engine.comparePreviewScale(ctx, v, req, 2, index.ticks[2:6], false)
	if err != nil || n != 4 || q.ExactFrames != 4 || !q.withinLimits() {
		t.Fatalf("CPU preview comparison: %+v %d %v", q, n, err)
	}
	for _, angle := range []string{"-90", "90", "180"} {
		rotated := filepath.Join(t.TempDir(), "rotated.mp4")
		runFF(t, "-v", "error", "-i", path, "-map", "0:v:0", "-c:v", "copy", "-metadata:s:v:0", "rotate="+angle, rotated)
		r, err := engine.Inspect(ctx, rotated)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := r.Video()
		if err != nil {
			t.Fatal(err)
		}
		rotation, err := raw.DisplayRotation()
		if err != nil || rotation == 0 {
			t.Fatalf("fixture display rotation: %d %v", rotation, err)
		}
		display, err := raw.DisplayGeometry()
		if err != nil {
			t.Fatal(err)
		}
		req := StillRequest{Path: rotated, Width: display.Width, Height: display.Height, Canvas: CanvasSize{display.Width, display.Height}, OutputWidth: display.Width / 2, OutputHeight: display.Height / 2}
		prefix, err := previewScaleCandidate(raw, req)
		want := "scale_cuda=w=32:h=24:format=p010le:interp_algo=lanczos:passthrough=0," + cudaDownloadFilter + "," + strings.Join(displayRotationFilters(rotation), ",")
		if err != nil || prefix != want {
			t.Fatalf("rotated candidate graph: %q want %q: %v", prefix, want, err)
		}
		if rotation != 180 {
			bad := req
			bad.OutputWidth, bad.OutputHeight = bad.OutputHeight, bad.OutputWidth
			if _, err := previewScaleCandidate(raw, bad); err == nil {
				t.Fatal("coded orientation incorrectly accepted as display output")
			}
		}
		q, n, err := engine.comparePreviewScale(ctx, raw, req, 2, index.ticks[2:6], false)
		if err != nil || n != 4 || q.ExactFrames != 4 || !q.withinLimits() {
			t.Fatalf("rotated CPU preview comparison: %+v %d %v", q, n, err)
		}
	}
	if os.Getenv("PIXELCUT_QUALIFY_CUDA_SCALE") == "1" {
		for _, size := range []scaleCheckSize{{64, 48}, {32, 24}, {128, 96}} {
			r, err := engine.CheckCUDAScale(ctx, path, 2, 4, size.width, size.height)
			if err != nil || r.Matched != 4 || r.Status != "matched" {
				t.Fatalf("physical CUDA scale parity: %+v %v", r, err)
			}
			t.Logf("CUDA scale parity: %+v", r)
		}
	}
	if os.Getenv("PIXELCUT_QUALIFY_CUDA") == "1" {
		for _, tc := range [][2]int{{0, 9}, {2, 4}, {7, 2}} {
			r, err := engine.CheckHEVCDecode(ctx, path, tc[0], tc[1])
			if err != nil || r.Matched != tc[1] || r.Status != "matched" {
				t.Fatalf("physical CUDA parity: %+v %v", r, err)
			}
			t.Logf("CUDA parity: %+v", r)
		}
	}
}

func TestDecodeCheckRejectsInvalidSourceBeforeGPU(t *testing.T) {
	r, err := Default().CheckHEVCDecode(context.Background(), filepath.Join(t.TempDir(), "missing.mp4"), 0, 4)
	if err == nil || r.Status != "failed" || r.Failure == "" {
		t.Fatalf("invalid source did not fail closed: %+v %v", r, err)
	}
}
