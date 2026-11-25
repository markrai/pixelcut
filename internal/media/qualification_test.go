package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTransformPixelEquivalence(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	ctx := context.Background()
	e := Default()
	dir := t.TempDir()
	base := filepath.Join(dir, "asymmetric.mp4")
	runFF(t, "-v", "error", "-f", "lavfi", "-i", "nullsrc=s=160x120:r=12:d=1,format=yuv420p10le,geq=lum='128+2*X+3*Y+180*gte(X,90)*lt(Y,40)':cb=512:cr=512,setsar=1", "-c:v", "libx265", "-x265-params", "pools=2:frame-threads=1:log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:chromaloc=0", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv", "-chroma_sample_location", "left", "-tag:v", "hvc1", base)
	for _, rotation := range []int{0, 90, 180, 270} {
		t.Run(fmt.Sprintf("source-%d", rotation), func(t *testing.T) {
			src := base
			w, h := 160, 120
			if rotation != 0 {
				src = filepath.Join(dir, fmt.Sprintf("source-%d.mp4", rotation))
				runFF(t, "-v", "error", "-display_rotation:v:0", fmt.Sprint(-rotation), "-i", base, "-c", "copy", src)
				if rotation == 90 || rotation == 270 {
					w, h = h, w
				}
			}
			crop := Crop{X: 8, Y: 12, Width: w - 24, Height: h - 32}
			for _, tc := range []struct {
				name      string
				tr        Transform
				reference string
			}{
				{"rotate-flip", Transform{CanvasWidth: crop.Height, CanvasHeight: crop.Width, Scale: 100, Rotation: 90, FlipHorizontal: true}, fmt.Sprintf("crop=%d:%d:8:12,transpose=clock,hflip,setsar=1", crop.Width, crop.Height)},
				{"scale-pad", Transform{CanvasWidth: crop.Width, CanvasHeight: crop.Height, Scale: 50, X: 4, Y: -6}, fmt.Sprintf("crop=%d:%d:8:12,scale=%d:%d:flags=lanczos,pad=%d:%d:%d:%d:color=black,setsar=1", crop.Width, crop.Height, evenRound(float64(crop.Width)/2), evenRound(float64(crop.Height)/2), crop.Width, crop.Height, evenRound(float64(crop.Width-evenRound(float64(crop.Width)/2))/2)+4, evenRound(float64(crop.Height-evenRound(float64(crop.Height)/2))/2)-6)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					graph, ow, oh, err := VideoFilters(w, h, &crop, &tc.tr)
					if err != nil || graph != tc.reference {
						t.Fatalf("graph %q want %q: %v", graph, tc.reference, err)
					}
					reference := runFF(t, "-v", "error", "-i", src, "-vf", tc.reference, "-frames:v", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-")
					prefix := map[int]string{0: "", 90: "transpose=clock,", 180: "hflip,vflip,", 270: "transpose=cclock,"}[rotation]
					preview := runFF(t, "-v", "error", "-noautorotate", "-i", src, "-vf", prefix+graph, "-frames:v", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-")
					if len(reference) != ow*oh*3 || len(preview) != len(reference) {
						t.Fatal("unexpected raw frame size")
					}
					for i := range reference {
						if reference[i] != preview[i] {
							t.Fatalf("rotation normalization differs at byte %d", i)
						}
					}
					out := filepath.Join(dir, fmt.Sprintf("export-%d-%s.mp4", rotation, tc.name))
					if err = e.Render(ctx, src, out, Edit{Crop: &crop, Transform: &tc.tr}); err != nil {
						t.Fatal(err)
					}
					actual := runFF(t, "-v", "error", "-i", out, "-frames:v", "1", "-pix_fmt", "yuv420p10le", "-f", "rawvideo", "-")
					if len(actual) != len(reference) {
						t.Fatal("export frame size mismatch")
					}
					var sum int64
					var samples int
					for i := 0; i < ow*oh*2; i += 2 {
						d := int(binary.LittleEndian.Uint16(actual[i:])) - int(binary.LittleEndian.Uint16(reference[i:]))
						if d < 0 {
							d = -d
						}
						sum += int64(d)
						samples++
					}
					mae := float64(sum) / float64(samples)
					t.Logf("10-bit luma mean absolute error %.3f", mae)
					if mae > 8 {
						t.Fatalf("lossy export differs excessively: %.3f", mae)
					}
					if tc.name == "scale-pad" && binary.LittleEndian.Uint16(reference[:2]) != 64 {
						t.Fatal("limited-range black padding is not code 64")
					}
				})
			}
		})
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatal(err)
	}
}
