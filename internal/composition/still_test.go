package composition

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type solidStillDecoder struct {
	calls []media.StillRequest
	err   error
}

func (d *solidStillDecoder) Decode(_ context.Context, req media.StillRequest) (media.StillFrame, error) {
	d.calls = append(d.calls, req)
	if d.err != nil {
		return media.StillFrame{}, d.err
	}
	pixels := make([]byte, req.OutputWidth*req.OutputHeight*4)
	for i := 0; i < len(pixels); i += 4 {
		if filepath.Base(req.Path) == "base.mp4" {
			pixels[i] = 200
		} else {
			pixels[i+2] = 200
		}
		pixels[i+3] = 255
	}
	return media.StillFrame{Width: req.OutputWidth, Height: req.OutputHeight, Pixels: pixels}, nil
}

func TestRenderStillLayerOrderAndTransparentMargins(t *testing.T) {
	p := project.New()
	dir := t.TempDir()
	p.Assets = []project.Asset{
		{ID: "a", Path: filepath.Join(dir, "base.mp4"), Duration: 100, Width: 8, Height: 8},
		{ID: "b", Path: filepath.Join(dir, "top.mp4"), Duration: 100, Width: 8, Height: 8},
	}
	p.Tracks[0].Clips = []project.Clip{{ID: "base", AssetID: "a", In: 10, Out: 90}}
	var err error
	p, _, err = p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracks[2].Clips = []project.Clip{{ID: "top", AssetID: "b", Start: 5, In: 30, Out: 90, Transform: &media.Transform{CanvasWidth: 8, CanvasHeight: 8, Scale: 50}}}
	p, err = p.SetClipOpacity(project.ClipRef{TrackID: "V2", ClipID: "top"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.PlanLayers(20)
	if err != nil {
		t.Fatal(err)
	}
	d := &solidStillDecoder{}
	f, err := RenderStill(context.Background(), d, plan, 8)
	if err != nil || !f.Valid() || len(d.calls) != 2 {
		t.Fatalf("render: %+v %v", f, err)
	}
	if d.calls[0].Tick != 30 || d.calls[1].Tick != 45 || d.calls[1].OutputWidth != 4 || d.calls[1].OutputHeight != 4 {
		t.Fatalf("incorrect decoder requests: %+v", d.calls)
	}
	pixel := func(x, y int) [4]byte {
		i := (y*f.Width + x) * 4
		return [4]byte{f.Pixels[i], f.Pixels[i+1], f.Pixels[i+2], f.Pixels[i+3]}
	}
	if pixel(0, 0) != ([4]byte{200, 0, 0, 255}) || pixel(3, 3) != ([4]byte{100, 0, 100, 255}) || pixel(6, 6) != ([4]byte{200, 0, 0, 255}) {
		t.Fatal("layer ordering, opacity or transparent margins incorrect")
	}
	d.err = errors.New("decoder failed")
	if f, err = RenderStill(context.Background(), d, plan, 8); err == nil || f.Valid() {
		t.Fatal("partial composite published after decode failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = RenderStill(ctx, d, plan, 8); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	plan, err = e.PlanLayers(e.Duration())
	if err != nil {
		t.Fatal(err)
	}
	f, err = RenderStill(context.Background(), d, plan, 8)
	if err != nil || !f.Valid() || pixel(3, 3) != ([4]byte{0, 0, 0, 255}) {
		t.Fatal("end frame was not black")
	}
}

func TestStillSizeAndInvalidLayer(t *testing.T) {
	w, h, err := StillSize(media.CanvasSize{Width: 2160, Height: 3840}, 960)
	if err != nil || w != 540 || h != 960 {
		t.Fatalf("portrait size: %d %d %v", w, h, err)
	}
	plan := LayerPlan{Canvas: media.CanvasSize{Width: 8, Height: 8}, Layers: []PlannedLayer{{Layer: VideoLayer{Opacity: 100}, Geometry: media.LayerGeometry{Bounds: media.Placement{DestX: 7, Width: 2, Height: 2}}}}}
	if _, err = RenderStill(context.Background(), &solidStillDecoder{}, plan, 8); err == nil {
		t.Fatal("invalid bounds accepted")
	}
}
