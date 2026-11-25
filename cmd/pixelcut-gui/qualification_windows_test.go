package main

import (
	"context"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestNativePreviewEquivalence(t *testing.T) {
	if os.Getenv("PIXELCUT_QUALIFY_PREVIEW") != "1" {
		t.Skip("set PIXELCUT_QUALIFY_PREVIEW=1 on an interactive Windows desktop")
	}
	e := media.Default()
	player, err := e.MPVPath()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script, err := writeCropScript()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(script)
	driver := filepath.Join(dir, "qualification.lua")
	if err = os.WriteFile(driver, []byte(qualificationDriver), 0600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "source.mp4")
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if b, err := exec.CommandContext(ctx, e.FFmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg: %v\n%s", err, b)
		}
	}
	run("-v", "error", "-f", "lavfi", "-i", "nullsrc=s=160x120:r=12:d=2,format=yuv420p10le,geq=lum='128+2*X+3*Y+180*gte(X,90)*lt(Y,40)':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", base)
	for _, rotation := range []int{0, 90, 180, 270} {
		t.Run(fmt.Sprintf("source-%d", rotation), func(t *testing.T) {
			src := base
			a := project.Asset{Width: 160, Height: 120, Range: "tv"}
			if rotation != 0 {
				src = filepath.Join(dir, fmt.Sprintf("source-%d.mp4", rotation))
				run("-v", "error", "-display_rotation:v:0", fmt.Sprint(-rotation), "-i", base, "-c", "copy", src)
				if rotation == 90 || rotation == 270 {
					a.Width, a.Height = a.Height, a.Width
				}
			}
			c := &project.Crop{X: 8, Y: 12, Width: a.Width - 24, Height: a.Height - 32}
			for _, tc := range []struct {
				name string
				tr   *media.Transform
			}{
				{"crop-only", nil},
				{"rotate-flip", &media.Transform{CanvasWidth: c.Height, CanvasHeight: c.Width, Scale: 100, Rotation: 90, FlipHorizontal: true}},
				{"scale-pad", &media.Transform{CanvasWidth: c.Width, CanvasHeight: c.Height, Scale: 50, X: 4, Y: -6}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					prefix := fmt.Sprintf("%d-%s", rotation, tc.name)
					out := filepath.Join(dir, prefix+".mp4")
					ed := media.Edit{Crop: &media.Crop{X: c.X, Y: c.Y, Width: c.Width, Height: c.Height}, Transform: tc.tr}
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					if err := e.Render(ctx, src, out, ed); err != nil {
						t.Fatal(err)
					}
					clock := newPreviewClock(project.Clip{In: 0, Out: 2000000}, 0, true)
					if clock.setComposition(a, c, tc.tr, false) == 0 {
						t.Fatal("could not build composition")
					}
					payload := clock.cropRequest.Load().payload()
					t.Logf("preview composition: %s", payload)
					preview := nativeScreenshot(t, player, script, driver, src, filepath.Join(dir, prefix+"-preview.png"), payload)
					export := nativeScreenshot(t, player, script, driver, out, filepath.Join(dir, prefix+"-export.png"), "")
					comparePreviewImages(t, preview, export)
				})
			}
		})
	}
}

func nativeScreenshot(t *testing.T, player, script, driver, source, path, payload string) image.Image {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, player, "--no-config", "--vo=gpu-next", "--hwdec=no", "--target-prim=bt.709", "--target-trc=bt.1886", "--tone-mapping=auto", "--no-audio", "--pause", "--keep-open=yes", "--osc=no", "--geometry=640x480", "--screenshot-sw=no", "--script="+script, "--script="+driver, "--", source)
	cmd.Env = append(os.Environ(), "PIXELCUT_TEST_SCREENSHOT="+path, "PIXELCUT_TEST_COMPOSITION="+payload)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mpv qualification: %v\n%s", err, b)
	}
	t.Logf("mpv %s: %s", filepath.Base(path), b)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func comparePreviewImages(t *testing.T, a, b image.Image) {
	t.Helper()
	ar, br := a.Bounds(), b.Bounds()
	if ar.Dx() != br.Dx() || ar.Dy() != br.Dy() || ar.Dx() < 8 || ar.Dy() < 8 {
		t.Fatalf("preview bounds %v differ from export %v", ar, br)
	}
	var total, worst float64
	n := 0
	for by := 0; by < 4; by++ {
		for bx := 0; bx < 4; bx++ {
			var block float64
			count := 0
			for y := by * ar.Dy() / 4; y < (by+1)*ar.Dy()/4; y++ {
				for x := bx * ar.Dx() / 4; x < (bx+1)*ar.Dx()/4; x++ {
					r, g, bl, _ := a.At(ar.Min.X+x, ar.Min.Y+y).RGBA()
					rr, gg, bb, _ := b.At(br.Min.X+x, br.Min.Y+y).RGBA()
					d := (math.Abs(float64(r)-float64(rr)) + math.Abs(float64(g)-float64(gg)) + math.Abs(float64(bl)-float64(bb))) / (3 * 257)
					block += d
					count++
				}
			}
			total += block
			n += count
			worst = math.Max(worst, block/float64(count))
		}
	}
	mean := total / float64(n)
	t.Logf("RGB code-value mean error %.3f; worst 4x4 block %.3f", mean, worst)
	if mean > 8 || worst > 18 {
		t.Fatalf("preview/export composition or color differs: mean %.3f, worst %.3f", mean, worst)
	}
}

func TestPreviewComparison(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 32, 24))
	b := image.NewRGBA(a.Bounds())
	for i := range a.Pix {
		a.Pix[i] = 255
		b.Pix[i] = 255
	}
	comparePreviewImages(t, a, b)
}

const qualificationDriver = `
local mp=require("mp")
local output=os.getenv("PIXELCUT_TEST_SCREENSHOT")
local payload=os.getenv("PIXELCUT_TEST_COMPOSITION") or ""
local sent=false
local stable=0
local function fail(message)
    mp.msg.error(message)
    mp.commandv("quit","2")
end
mp.add_timeout(25,function() fail("qualification timed out") end)
mp.add_periodic_timer(0.1,function()
    if not mp.get_property_native("video-out-params") or mp.get_property_number("time-pos",-1)<0 then return end
    if payload~="" then
        if not sent then
            if not mp.get_property_native("user-data/pixelcut/crop-ready") then return end
            mp.commandv("script-message","pixelcut-crop",payload)
            sent=true
            return
        end
        local state=mp.get_property_native("user-data/pixelcut/crop")
        if not state then return end
        if state.error and state.error~="" then fail(state.error);return end
        if not state.ready then return end
    end
    stable=stable+1
    if stable<10 then return end
    mp.msg.info("video-out-params: "..require("mp.utils").format_json(mp.get_property_native("video-out-params")))
    local ok,err=mp.commandv("screenshot-to-file",output,"video")
    if not ok then fail(tostring(err));return end
    mp.commandv("quit","0")
end)
`
