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
