package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"
)

type VideoStage string

const (
	StageDecode    VideoStage = "decode"
	StageGeometry  VideoStage = "geometry"
	StageComposite VideoStage = "composite"
	StageEncode    VideoStage = "encode"
)

type Backend string

const (
	BackendAuto Backend = "auto"
	BackendCPU  Backend = "cpu"
)

type BackendDecision struct {
	Stage     VideoStage `json:"stage"`
	Requested Backend    `json:"requested"`
	Selected  Backend    `json:"selected"`
	Reason    string     `json:"reason"`
}

func SelectVideoBackend(stage VideoStage, requested Backend) (BackendDecision, error) {
	d := BackendDecision{Stage: stage, Requested: requested}
	switch stage {
	case StageDecode, StageGeometry, StageComposite, StageEncode:
	default:
		return d, errors.New("invalid video processing stage")
	}
	switch requested {
	case BackendCPU:
		d.Selected, d.Reason = BackendCPU, "explicit CPU reference"
	case BackendAuto:
		d.Selected, d.Reason = BackendCPU, "no qualified GPU implementation for this stage"
	default:
		return d, fmt.Errorf("backend %q is not qualified for %s", requested, stage)
	}
	return d, nil
}

type DeviceProbe struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type AccelerationReport struct {
	Platform       string            `json:"platform"`
	FFmpegVersion  string            `json:"ffmpeg_version"`
	HWAccelMethods []string          `json:"hwaccel_methods"`
	DeviceTypes    []string          `json:"device_types"`
	HEVCDecoders   []string          `json:"hevc_decoders"`
	HEVCEncoders   []string          `json:"hevc_encoders"`
	GPUFilters     []string          `json:"gpu_filters"`
	Devices        []DeviceProbe     `json:"devices"`
	Decisions      []BackendDecision `json:"decisions"`
}

type capabilityRunner func(context.Context, ...string) (string, error)

const capabilityOutputLimit = 256 * 1024

type capabilityOutput struct {
	data     []byte
	overflow bool
}

func (b *capabilityOutput) Write(p []byte) (int, error) {
	n := min(len(p), capabilityOutputLimit-len(b.data))
	b.data = append(b.data, p[:n]...)
	if n != len(p) {
		b.overflow = true
		return n, errors.New("FFmpeg capability output limit exceeded")
	}
	return n, nil
}

func (e Engine) ProbeAcceleration(ctx context.Context, devices bool) (AccelerationReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(ctx context.Context, args ...string) (string, error) {
		job, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		cmd := exec.CommandContext(job, e.FFmpeg, args...)
		cmd.WaitDelay = time.Second
		var out capabilityOutput
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		if job.Err() != nil {
			err = job.Err()
		} else if out.overflow {
			err = errors.New("FFmpeg capability output limit exceeded")
		}
		return string(out.data), err
	}
	return probeAcceleration(ctx, run, devices)
}

var knownDevices = []string{"cuda", "d3d11va", "dxva2", "qsv", "vaapi", "vulkan", "videotoolbox", "d3d12va", "amf", "vdpau", "drm", "opencl", "mediacodec"}

var knownHEVCDecoders = []string{"hevc", "hevc_cuvid", "hevc_qsv", "hevc_amf", "hevc_v4l2m2m", "hevc_mediacodec", "hevc_videotoolbox"}

var knownHEVCEncoders = []string{"libx265", "hevc_nvenc", "hevc_qsv", "hevc_amf", "hevc_vaapi", "hevc_videotoolbox", "hevc_mf", "hevc_vulkan", "hevc_d3d12va", "hevc_v4l2m2m", "hevc_mediacodec"}

var knownGPUFilters = []string{"hwupload", "hwupload_cuda", "hwdownload", "hwmap", "scale_cuda", "scale_npp", "scale_qsv", "vpp_qsv", "scale_vaapi", "scale_vulkan", "scale_d3d11", "overlay_cuda", "overlay_qsv", "overlay_vaapi", "overlay_vulkan", "libplacebo", "tonemap_cuda", "tonemap_vaapi", "tonemap_opencl"}

func validCapabilityFlags(flags string) bool {
	if len(flags) != 3 && len(flags) != 6 {
		return false
	}
	for _, c := range flags {
		if c != '.' && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

func listedNames(output string, known []string, table bool) []string {
	names := make([]string, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		var name string
		if table && len(fields) >= 3 && validCapabilityFlags(fields[0]) {
			name = fields[1]
		} else if !table && len(fields) == 1 {
			name = fields[0]
		}
		if slices.Contains(known, name) && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func probeAcceleration(ctx context.Context, run capabilityRunner, devices bool) (AccelerationReport, error) {
	r := AccelerationReport{Platform: runtime.GOOS + "/" + runtime.GOARCH, Devices: []DeviceProbe{}}
	version, err := run(ctx, "-version")
	if err != nil {
		return r, fmt.Errorf("FFmpeg version probe: %w", err)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(version), "\n")
	r.FFmpegVersion = strings.TrimSpace(first)
	if !strings.HasPrefix(r.FFmpegVersion, "ffmpeg version ") || len(r.FFmpegVersion) > 1024 {
		return r, errors.New("unexpected FFmpeg version output")
	}
	for _, q := range []struct {
		args   []string
		known  []string
		table  bool
		target *[]string
		header string
	}{
		{[]string{"-hide_banner", "-hwaccels"}, knownDevices, false, &r.HWAccelMethods, "Hardware acceleration methods:"},
		{[]string{"-hide_banner", "-init_hw_device", "list"}, knownDevices, false, &r.DeviceTypes, "Supported hardware device types:"},
		{[]string{"-hide_banner", "-decoders"}, knownHEVCDecoders, true, &r.HEVCDecoders, "Decoders:"},
		{[]string{"-hide_banner", "-encoders"}, knownHEVCEncoders, true, &r.HEVCEncoders, "Encoders:"},
		{[]string{"-hide_banner", "-filters"}, knownGPUFilters, true, &r.GPUFilters, "Filters:"},
	} {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		out, err := run(ctx, q.args...)
		if err != nil {
			return r, fmt.Errorf("FFmpeg %s inventory: %w", q.header, err)
		}
		if !strings.Contains(out, q.header) {
			return r, fmt.Errorf("missing FFmpeg inventory header %q", q.header)
		}
		*q.target = listedNames(out, q.known, q.table)
	}
	for _, name := range r.DeviceTypes {
		p := DeviceProbe{Name: name, Status: "advertised"}
		if devices && slices.Contains([]string{"cuda", "d3d11va", "qsv", "vaapi", "vulkan", "d3d12va", "videotoolbox"}, name) {
			if err := ctx.Err(); err != nil {
				return r, err
			}
			out, err := run(ctx, "-hide_banner", "-nostdin", "-v", "error", "-init_hw_device", name+"=pcgpu", "-f", "lavfi", "-i", "nullsrc=s=64x64:r=1:d=1", "-frames:v", "1", "-f", "null", "-")
			if err == nil {
				p.Status = "initialized"
			} else {
				p.Status = "initialization-failed"
				p.Detail = strings.TrimSpace(out)
				if len(p.Detail) > 512 {
					p.Detail = p.Detail[:512]
				}
				if p.Detail == "" {
					p.Detail = err.Error()
					if len(p.Detail) > 512 {
						p.Detail = p.Detail[:512]
					}
				}
			}
		}
		r.Devices = append(r.Devices, p)
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	for _, stage := range []VideoStage{StageDecode, StageGeometry, StageComposite, StageEncode} {
		d, err := SelectVideoBackend(stage, BackendAuto)
		if err != nil {
			return r, err
		}
		r.Decisions = append(r.Decisions, d)
	}
	return r, nil
}

func (r AccelerationReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\nFFmpeg advertised capabilities (not qualified):\nHardware methods: %s\nDevice types: %s\nHEVC decoders: %s\nHEVC encoders: %s\nGPU-related filters: %s\n", r.Platform, r.FFmpegVersion, strings.Join(r.HWAccelMethods, ", "), strings.Join(r.DeviceTypes, ", "), strings.Join(r.HEVCDecoders, ", "), strings.Join(r.HEVCEncoders, ", "), strings.Join(r.GPUFilters, ", "))
	for _, d := range r.Devices {
		fmt.Fprintf(&b, "\n%s: %s", d.Name, d.Status)
		if d.Detail != "" {
			fmt.Fprintf(&b, "\n%s", d.Detail)
		}
	}
	b.WriteString("\n\nPixelCut backend policy:")
	for _, d := range r.Decisions {
		fmt.Fprintf(&b, "\n%s: %s (%s)", d.Stage, d.Selected, d.Reason)
	}
	b.WriteString("\n\nDevice initialization does not establish Main 10, HDR, frame timing, rendering or encoding support. Rendering remains CPU-only in phase 3A.1.")
	return b.String()
}
