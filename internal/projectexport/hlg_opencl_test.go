package projectexport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
)

const hlgOpenCLKernel = `
const sampler_t pixel_sampler = CLK_NORMALIZED_COORDS_FALSE | CLK_ADDRESS_NONE | CLK_FILTER_NEAREST;

float inverse_hlg(float e) {
    e = clamp(e, 0.0f, 1.0f);
    return e <= 0.5f ? e * e / 3.0f : (exp((e - 0.55991073f) / 0.17883277f) + 0.28466892f) / 12.0f;
}

float forward_hlg(float e) {
    e = clamp(e, 0.0f, 1.0f);
    return e <= 1.0f / 12.0f ? sqrt(3.0f * e) : 0.17883277f * log(12.0f * e - 0.28466892f) + 0.55991073f;
}

float3 decode_hlg(float3 p) {
    float y = (p.x - 64.0f) / 876.0f;
    float cb = (p.y - 512.0f) / 896.0f;
    float cr = (p.z - 512.0f) / 896.0f;
    float r = y + 2.0f * (1.0f - 0.2627f) * cr;
    float b = y + 2.0f * (1.0f - 0.0593f) * cb;
    float g = (y - 0.2627f * r - 0.0593f * b) / 0.6780f;
    return (float3)(inverse_hlg(r), inverse_hlg(g), inverse_hlg(b));
}

float3 encode_hlg(float3 p) {
    float r = forward_hlg(p.x);
    float g = forward_hlg(p.y);
    float b = forward_hlg(p.z);
    float y = 0.2627f * r + 0.6780f * g + 0.0593f * b;
    float cb = (b - y) / (2.0f * (1.0f - 0.0593f));
    float cr = (r - y) / (2.0f * (1.0f - 0.2627f));
    return round((float3)(64.0f + 876.0f * clamp(y, 0.0f, 1.0f),
                          clamp(512.0f + 896.0f * cb, 64.0f, 960.0f),
                          clamp(512.0f + 896.0f * cr, 64.0f, 960.0f)));
}

__kernel void composite_hlg(write_only image2d_t dst, uint frame_index, read_only image2d_t atlas) {
    int2 pos = (int2)(get_global_id(0), get_global_id(1));
    int h = get_image_height(dst);
    if (pos.x >= get_image_width(dst) || pos.y >= h) return;
    float3 result = (float3)(64.0f, 512.0f, 512.0f);
    float3 sum = (float3)(0.0f);
    float remaining = 1.0f;
    int mixed = 0;
    for (int i = 3; i >= 0; --i) {
        float4 p = round(read_imagef(atlas, pixel_sampler, pos + (int2)(0, i * h)) * 65535.0f);
        if (p.w == 0.0f) continue;
        if (!mixed && p.w == 100.0f) {
            result = p.xyz;
            break;
        }
        mixed = 1;
        float alpha = p.w / 100.0f;
        sum += remaining * alpha * decode_hlg(p.xyz);
        remaining *= 1.0f - alpha;
        if (p.w == 100.0f) break;
    }
    if (mixed) result = encode_hlg(sum);
    write_imagef(dst, pos, (float4)(result, 100.0f + (float)frame_index) / 65535.0f);
}
`

const openCLCompositeMaxPixels = 1920 * 1080

func packHLGOpenCLAtlas(c media.CanvasSize, layers []layerFrame) ([]byte, error) {
	return packHLGOpenCLAtlasInto(c, layers, nil)
}

func packHLGOpenCLAtlasInto(c media.CanvasSize, layers []layerFrame, atlas []byte) ([]byte, error) {
	if c.Validate() != nil || c.Width > 4096 || c.Height > 1920 || int64(c.Width)*int64(c.Height) > openCLCompositeMaxPixels || len(layers) > MaxVideoLayers {
		return nil, errors.New("invalid or oversized OpenCL qualification canvas")
	}
	for _, layer := range layers {
		b := layer.Bounds
		if layer.Opacity < 0 || layer.Opacity > 100 || b.Width <= 0 || b.Height <= 0 || b.DestX < 0 || b.DestY < 0 || b.Width > c.Width-b.DestX || b.Height > c.Height-b.DestY || len(layer.Pixels) != b.Width*b.Height*6 {
			return nil, errors.New("invalid OpenCL qualification layer")
		}
	}
	size := c.Width * c.Height * 8 * MaxVideoLayers
	if cap(atlas) < size {
		atlas = make([]byte, size)
	} else {
		atlas = atlas[:size]
		clear(atlas)
	}
	for i, layer := range layers {
		b := layer.Bounds
		n := b.Width * b.Height * 2
		for y := 0; y < b.Height; y++ {
			for x := 0; x < b.Width; x++ {
				p := readPixel(layer.Pixels, n, (y*b.Width+x)*2)
				if p.Y > 1023 || p.Cb > 1023 || p.Cr > 1023 {
					return nil, errors.New("OpenCL input exceeds 10-bit code range")
				}
				offset := ((i*c.Height+b.DestY+y)*c.Width + b.DestX + x) * 8
				binary.LittleEndian.PutUint16(atlas[offset:], p.Y)
				binary.LittleEndian.PutUint16(atlas[offset+2:], p.Cb)
				binary.LittleEndian.PutUint16(atlas[offset+4:], p.Cr)
				binary.LittleEndian.PutUint16(atlas[offset+6:], uint16(layer.Opacity))
			}
		}
	}
	return atlas, nil
}

func unpackHLGOpenCL(c media.CanvasSize, packed []byte) ([]byte, error) {
	return unpackHLGOpenCLInto(c, packed, nil, 100)
}

func unpackHLGOpenCLInto(c media.CanvasSize, packed, out []byte, marker uint16) ([]byte, error) {
	if c.Validate() != nil || c.Width > 4096 || c.Height > 1920 || int64(c.Width)*int64(c.Height) > openCLCompositeMaxPixels || len(packed) != c.Width*c.Height*8 {
		return nil, errors.New("invalid OpenCL output extent")
	}
	n := c.Width * c.Height
	if cap(out) < n*6 {
		out = make([]byte, n*6)
	} else {
		out = out[:n*6]
	}
	for i := 0; i < n; i++ {
		offset := i * 8
		p := ycbcr10{binary.LittleEndian.Uint16(packed[offset:]), binary.LittleEndian.Uint16(packed[offset+2:]), binary.LittleEndian.Uint16(packed[offset+4:])}
		if p.Y > 1023 || p.Cb > 1023 || p.Cr > 1023 || binary.LittleEndian.Uint16(packed[offset+6:]) != marker {
			return nil, fmt.Errorf("invalid OpenCL transport at pixel %d", i)
		}
		writePixel(out, n*2, i*2, p)
	}
	return out, nil
}

type openCLCapture struct {
	data     []byte
	limit    int
	overflow bool
}

func (c *openCLCapture) Write(p []byte) (int, error) {
	n := min(len(p), c.limit-len(c.data))
	c.data = append(c.data, p[:n]...)
	c.overflow = c.overflow || n != len(p)
	return len(p), nil
}

var openCLDevicePattern = regexp.MustCompile("^([0-9]{1,2})\\.([0-9]{1,2})$")

func openCLCompositeArgs(c media.CanvasSize, device string) ([]string, error) {
	return openCLCompositeBatchArgs(c, device, 1)
}

func openCLCompositeBatchArgs(c media.CanvasSize, device string, frames int) ([]string, error) {
	if frames < 1 || frames > 120 || c.Validate() != nil || c.Width > 4096 || c.Height > 1920 || int64(c.Width)*int64(c.Height) > openCLCompositeMaxPixels {
		return nil, errors.New("invalid OpenCL batch extent or count")
	}
	if !openCLDevicePattern.MatchString(device) {
		return nil, errors.New("OpenCL device must be a platform.device index")
	}
	return []string{
		"-hide_banner", "-loglevel", "verbose", "-xerror", "-nostdin",
		"-init_hw_device", "opencl=pccl:" + device + ",device_type=gpu", "-filter_hw_device", "pccl",
		"-f", "rawvideo", "-pixel_format", "rgba64le", "-video_size", fmt.Sprintf("%dx%d", c.Width, c.Height*MaxVideoLayers), "-framerate", "1", "-i", "pipe:0",
		"-filter_threads", "1", "-vf", fmt.Sprintf("hwupload,program_opencl=source=hlg.cl:kernel=composite_hlg:size=%dx%d,hwdownload,format=rgba64le", c.Width, c.Height),
		"-frames:v", fmt.Sprint(frames), "-fps_mode", "passthrough", "-c:v", "rawvideo", "-pix_fmt", "rgba64le", "-threads", "1", "-f", "rawvideo", "pipe:1",
	}, nil
}

func runHLGOpenCL(ctx context.Context, ffmpeg, dir, device string, c media.CanvasSize, layers []layerFrame) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	args, err := openCLCompositeArgs(c, device)
	if err != nil {
		return nil, "", err
	}
	atlas, err := packHLGOpenCLAtlas(c, layers)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(atlas)
	out := openCLCapture{limit: c.Width * c.Height * 8}
	log := openCLCapture{limit: 128 * 1024}
	cmd.Stdout, cmd.Stderr = &out, &log
	if err := cmd.Run(); err != nil {
		return nil, string(log.data), fmt.Errorf("OpenCL candidate: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, string(log.data), err
	}
	if out.overflow || log.overflow || len(out.data) != out.limit {
		return nil, string(log.data), errors.New("unexpected OpenCL output or diagnostic length")
	}
	pixels, err := unpackHLGOpenCL(c, out.data)
	return pixels, string(log.data), err
}

func TestHLGOpenCLTransport(t *testing.T) {
	c := media.CanvasSize{Width: 6, Height: 4}
	b := media.Placement{DestX: 1, DestY: 2, Width: 3, Height: 2}
	p := ycbcr10{990, 32, 1000}
	l := solidLayer(b, 73, p)
	atlas, err := packHLGOpenCLAtlas(c, []layerFrame{solidLayer(b, 100, ycbcr10{64, 512, 512}), l})
	if err != nil {
		t.Fatal(err)
	}
	if len(atlas) != c.Width*c.Height*8*4 {
		t.Fatal("wrong atlas extent")
	}
	offset := ((c.Height+2)*c.Width + 1) * 8
	for i, want := range []uint16{990, 32, 1000, 73} {
		if got := binary.LittleEndian.Uint16(atlas[offset+i*2:]); got != want {
			t.Fatalf("transport lane %d = %d want %d", i, got, want)
		}
	}
	if binary.LittleEndian.Uint16(atlas[offset-2:]) != 0 || binary.LittleEndian.Uint16(atlas[len(atlas)-2:]) != 0 {
		t.Fatal("uncovered or unused layer not transparent")
	}
	frame := make([]byte, c.Width*c.Height*8)
	for i := 0; i < len(frame); i += 8 {
		for j, v := range []uint16{990, 32, 1000, 100} {
			binary.LittleEndian.PutUint16(frame[i+j*2:], v)
		}
	}
	got, err := unpackHLGOpenCL(c, frame)
	if err != nil || !bytes.Equal(got, solidLayer(media.Placement{Width: c.Width, Height: c.Height}, 100, p).Pixels) {
		t.Fatalf("transport round trip: %v", err)
	}
	frame[6] = 0
	if _, err := unpackHLGOpenCL(c, frame); err == nil {
		t.Fatal("invalid transport marker accepted")
	}
	if _, err := unpackHLGOpenCL(c, frame[:len(frame)-1]); err == nil {
		t.Fatal("short output accepted")
	}
	l.Pixels = l.Pixels[:1]
	if _, err := packHLGOpenCLAtlas(c, []layerFrame{l}); err == nil {
		t.Fatal("invalid layer accepted")
	}
	if _, err := packHLGOpenCLAtlas(media.CanvasSize{Width: 3840, Height: 2160}, nil); err == nil {
		t.Fatal("oversized atlas accepted")
	}
	if _, err := openCLCompositeArgs(c, "0.0,device_type=cpu"); err == nil {
		t.Fatal("unsafe device selector accepted")
	}
}

func TestHLGOpenCLQualification(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_OPENCL_COMPOSITE") != "1" {
		t.Skip("physical OpenCL qualification is opt-in")
	}
	device := os.Getenv("PIXELCUT_OPENCL_DEVICE")
	if device == "" {
		device = "0.0"
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffmpeg, err = filepath.Abs(ffmpeg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	version := exec.CommandContext(ctx, ffmpeg, "-version")
	version.WaitDelay = time.Second
	versionLog := openCLCapture{limit: 128 * 1024}
	version.Stdout, version.Stderr = &versionLog, &versionLog
	if err := version.Run(); err != nil || versionLog.overflow {
		t.Fatalf("FFmpeg version: %v", err)
	}
	t.Logf("FFmpeg: %s\n%s\nOpenCL device index: %s", ffmpeg, versionLog.data, device)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hlg.cl"), []byte(hlgOpenCLKernel), 0600); err != nil {
		t.Fatal(err)
	}
	for _, f := range hlgQualificationFixtures() {
		t.Run(f.name, func(t *testing.T) {
			ref := black444(f.canvas)
			if err := compositeHLG(ctx, ref, f.canvas, f.layers); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			candidate, log, err := runHLGOpenCL(ctx, ffmpeg, dir, device, f.canvas, f.layers)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("%v\n%s", err, log)
			}
			q, err := compareHLGQualification(f, ref, candidate)
			t.Logf("exact=%d mixed=%d max_mixed_delta=%d startup_inclusive=%s", q.exactPixels, q.mixedPixels, q.maxMixedDelta, elapsed)
			if err != nil {
				t.Fatalf("%v\n%s", err, log)
			}
			if f.name == "black-gap" {
				t.Log(log)
			}
		})
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
	}
}
