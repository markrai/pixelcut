package media

import (
	"context"
	"encoding/binary"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreviewChromaCorrectionPlan(t *testing.T) {
	for _, tc := range []struct {
		source, output int
		shift          float64
	}{
		{1920, 960, -0.125},
		{3840, 960, -0.1875},
		{3840, 640, -(1 - 1.0/6) / 4},
		{960, 960, 0},
	} {
		p, err := planPreviewChromaCorrection(tc.source, tc.output)
		if err != nil || p.ShiftChromaSamples != tc.shift {
			t.Fatalf("phase plan: %+v %v", p, err)
		}
		sum := 0
		for _, w := range p.Weights {
			sum += w
		}
		if sum != p.WeightScale || p.Weights[3] <= 0 {
			t.Fatalf("invalid normalized kernel: %+v", p)
		}
		if tc.shift < 0 && p.Weights[2] <= 0 {
			t.Fatalf("wrong phase direction: %+v", p)
		}
	}
	if _, err := planPreviewChromaCorrection(1920, 8); err == nil {
		t.Fatal("tiny chroma plane accepted")
	}
}

func TestPreviewChromaCorrectionGraph(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{3840, 2160}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	req := StillRequest{Path: "source.mp4", Width: 3840, Height: 2160, Canvas: CanvasSize{3840, 2160}, OutputWidth: 960, OutputHeight: 540}
	plain, err := previewScaleCandidate(v, req)
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := previewScaleCandidateWithCorrection(v, req, true)
	if err != nil || !strings.HasPrefix(corrected, plain+",convolution=") || strings.Contains(plain, "convolution") || strings.Count(corrected, "hwdownload") != 1 || strings.Contains(corrected, "setparams") {
		t.Fatalf("corrected graph: %q %v", corrected, err)
	}
	cpu, err := previewCheckArgsWithCorrection(Default(), v, req, 0, 4, false, true)
	if err != nil || strings.Contains(strings.Join(cpu, " "), "convolution") {
		t.Fatalf("CPU reference changed: %v %v", cpu, err)
	}
}

func TestPreviewChromaCorrectionCPU(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source := "nullsrc=s=192x144:r=1:d=1,format=yuv420p10le,geq=lum=512:cb='if(lt(X,48),256,768)':cr=512"
	for _, size := range []CanvasSize{{96, 72}, {64, 48}, {48, 36}} {
		plan, err := planPreviewChromaCorrection(192, size.Width)
		if err != nil {
			t.Fatal(err)
		}
		req := StillRequest{Path: filepath.Join(t.TempDir(), "unused.mp4"), OutputWidth: size.Width, OutputHeight: size.Height}
		run := func(graph string) []byte {
			t.Helper()
			cmd := exec.CommandContext(ctx, ff, "-v", "error", "-f", "lavfi", "-i", source, "-vf", graph, "-frames:v", "1", "-threads", "1", "-f", "rawvideo", "-")
			raw, err := cmd.Output()
			if err != nil || len(raw) != size.Width*size.Height*3 {
				t.Fatalf("CPU convolution fixture: %d bytes %v", len(raw), err)
			}
			return raw
		}
		v := (ExportContract{Canvas: CanvasSize{192, 144}, Color: "hlg", FPS: 30}).ExpectedVideo()
		v.Profile = "Main 10"
		req.Width, req.Height, req.Canvas = 192, 144, CanvasSize{192, 144}
		left, err := previewChromaControlGraph(v, req, 0, 1, "left", "left")
		if err != nil {
			t.Fatal(err)
		}
		center, err := previewChromaControlGraph(v, req, 0, 1, "center", "center")
		if err != nil {
			t.Fatal(err)
		}
		help, err := exec.CommandContext(ctx, ff, "-hide_banner", "-h", "filter=scale").CombinedOutput()
		if err != nil || !strings.Contains(string(help), "in_chroma_loc") || !strings.Contains(string(help), "out_chroma_loc") {
			t.Skip("named chroma location controls unavailable")
		}
		reference, uncorrected, corrected := run(left), run(center), run(center+","+plan.filter())
		var before, after = previewPlaneAccumulator{width: size.Width, height: size.Height}, previewPlaneAccumulator{width: size.Width, height: size.Height}
		if err := before.add(reference, uncorrected); err != nil {
			t.Fatal(err)
		}
		if err := after.add(reference, corrected); err != nil {
			t.Fatal(err)
		}
		yBytes := size.Width * size.Height * 2
		for i := 0; i < yBytes; i += 2 {
			if binary.LittleEndian.Uint16(corrected[i:]) != binary.LittleEndian.Uint16(uncorrected[i:]) {
				t.Fatal("correction changed luminance")
			}
		}
		if before.planes[1].q.MaxDelta == 0 {
			t.Skip("software scaler does not distinguish these chroma controls")
		}
		if after.planes[1].q.MaxDelta >= before.planes[1].q.MaxDelta || after.planes[1].q.MeanAbsoluteDelta >= before.planes[1].q.MeanAbsoluteDelta {
			t.Fatalf("phase correction did not improve edge: before %+v after %+v", before.planes[1].q, after.planes[1].q)
		}
	}
}
