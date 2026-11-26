package media

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func fakeCapabilityRunner(calls *[]string) capabilityRunner {
	return func(_ context.Context, args ...string) (string, error) {
		key := strings.Join(args, " ")
		*calls = append(*calls, key)
		switch key {
		case "-version":
			return "ffmpeg version test\nconfiguration: test", nil
		case "-hide_banner -hwaccels":
			return "Hardware acceleration methods:\r\ncuda\r\nqsv\r\nunknown\r\ncuda\r\n", nil
		case "-hide_banner -init_hw_device list":
			return "Supported hardware device types:\ncuda\nqsv\nopencl\n", nil
		case "-hide_banner -decoders":
			return "Decoders:\n V..... hevc HEVC decoder\n V..... hevc_cuvid NVIDIA decoder\n arbitrary hevc_qsv prose\n", nil
		case "-hide_banner -encoders":
			return "Encoders:\n V....D hevc_nvenc NVIDIA\n V..... libx265 CPU\n V..... hevc_qsv Intel\n", nil
		case "-hide_banner -filters":
			return "Filters:\n ... scale_cuda V->V scaler\n ... hwdownload V->V copy\n ... overlay_qsv VV->V overlay\n", nil
		}
		if strings.Contains(key, "cuda=pcgpu") {
			return "", nil
		}
		if strings.Contains(key, "qsv=pcgpu") {
			return "driver not available", errors.New("device failed")
		}
		return "", errors.New("unexpected command")
	}
}

func TestAccelerationInventoryDoesNotAuthorizeGPU(t *testing.T) {
	var calls []string
	r, err := probeAcceleration(context.Background(), fakeCapabilityRunner(&calls), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 6 || !slices.Equal(r.HWAccelMethods, []string{"cuda", "qsv"}) || !slices.Equal(r.HEVCDecoders, []string{"hevc", "hevc_cuvid"}) || !slices.Equal(r.HEVCEncoders, []string{"hevc_nvenc", "hevc_qsv", "libx265"}) || !slices.Equal(r.GPUFilters, []string{"hwdownload", "overlay_qsv", "scale_cuda"}) {
		t.Fatalf("incorrect inventory: %+v", r)
	}
	for _, d := range r.Devices {
		if d.Status != "advertised" {
			t.Fatal("inventory attempted device initialization")
		}
	}
	calls = nil
	r, err = probeAcceleration(context.Background(), fakeCapabilityRunner(&calls), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 8 || len(r.Devices) != 3 || r.Devices[0].Name != "cuda" || r.Devices[0].Status != "initialized" || r.Devices[1].Status != "advertised" || r.Devices[2].Status != "initialization-failed" || r.Devices[2].Detail != "driver not available" {
		t.Fatalf("unexpected device diagnostics: %+v", r.Devices)
	}
	if len(r.Decisions) != 4 {
		t.Fatal("missing stage decisions")
	}
	for _, d := range r.Decisions {
		if d.Selected != BackendCPU || d.Requested != BackendAuto {
			t.Fatal("unqualified hardware selected")
		}
	}
	if !strings.Contains(r.String(), "Rendering remains CPU-only") {
		t.Fatal("diagnostic overstates support")
	}
}

func TestVideoBackendPolicyFailsClosed(t *testing.T) {
	for _, stage := range []VideoStage{StageDecode, StageGeometry, StageComposite, StageEncode} {
		for _, request := range []Backend{BackendAuto, BackendCPU} {
			d, err := SelectVideoBackend(stage, request)
			if err != nil || d.Stage != stage || d.Selected != BackendCPU || d.Reason == "" {
				t.Fatalf("CPU policy: %+v %v", d, err)
			}
		}
		if d, err := SelectVideoBackend(stage, "cuda"); err == nil || d.Selected != "" {
			t.Fatal("unqualified forced GPU silently accepted")
		}
	}
	if _, err := SelectVideoBackend("audio", BackendAuto); err == nil {
		t.Fatal("invalid stage accepted")
	}
}

func TestAccelerationProbeFailuresAndBound(t *testing.T) {
	var calls []string
	run := fakeCapabilityRunner(&calls)
	fail := func(ctx context.Context, args ...string) (string, error) {
		if slices.Contains(args, "-encoders") {
			return "truncated", nil
		}
		return run(ctx, args...)
	}
	if _, err := probeAcceleration(context.Background(), fail, true); err == nil {
		t.Fatal("missing inventory header accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Engine{FFmpeg: "ffmpeg"}).ProbeAcceleration(ctx, true); err == nil {
		t.Fatal("canceled probe accepted")
	}
	var out capabilityOutput
	if n, err := out.Write(make([]byte, capabilityOutputLimit+1)); n != capabilityOutputLimit || err == nil || !out.overflow || len(out.data) != capabilityOutputLimit {
		t.Fatal("output cap not enforced")
	}
}

func TestAccelerationRealFFmpegInventory(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	r, err := Default().ProbeAcceleration(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.HEVCDecoders, "hevc") || len(r.Decisions) != 4 {
		t.Fatalf("incomplete CPU baseline inventory: %+v", r)
	}
	t.Log(r.String())
}

func TestAccelerationNewFilterInventory(t *testing.T) {
	var calls []string
	legacy := fakeCapabilityRunner(&calls)
	run := func(ctx context.Context, args ...string) (string, error) {
		if slices.Contains(args, "-filters") {
			return "Filters:\r\n T.. = Timeline support\r\n .S. = Slice threading\r\n .. hwdownload V->V Download\r\n .. scale_cuda V->V Scale\r\n TS overlay_qsv VV->V Overlay\r\n .. scale_cuda V->V Duplicate\r\n XX scale_vulkan V->V Invalid flags\r\n ... scale_vaapi V->V Legacy\r\n", nil
		}
		return legacy(ctx, args...)
	}
	r, err := probeAcceleration(context.Background(), run, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hwdownload", "overlay_qsv", "scale_cuda", "scale_vaapi"}
	if !slices.Equal(r.GPUFilters, want) {
		t.Fatalf("new and legacy filter flags: got %v want %v", r.GPUFilters, want)
	}
	for _, d := range r.Decisions {
		if d.Selected != BackendCPU {
			t.Fatal("inventory authorized hardware rendering")
		}
	}
}
