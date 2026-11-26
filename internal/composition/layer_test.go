package composition

import (
	"testing"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

func TestPlanLayersOrderOpacityAndTrackPolicies(t *testing.T) {
	p := fixture(t)
	var err error
	p, err = p.SetCompositionCanvas(media.CanvasSize{Width: 1080, Height: 1920})
	if err != nil {
		t.Fatal(err)
	}
	ref := project.ClipRef{TrackID: "V2", ClipID: "overlay"}
	p, err = p.ApplyLayerPreset(ref, media.LayerTopRight)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.SetClipOpacity(ref, 50)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.PlanLayers(20)
	if err != nil || len(plan.Layers) != 3 {
		t.Fatalf("layer plan: %+v %v", plan, err)
	}
	for i, id := range []string{"V1", "V3", "V2"} {
		if plan.Layers[i].Layer.Ref.TrackID != id {
			t.Fatal("layer order differed from stacked lanes")
		}
	}
	top := plan.Layers[2]
	b := top.Geometry.Bounds
	if top.Layer.Opacity != 50 || b.Width >= plan.Canvas.Width || b.Height >= plan.Canvas.Height || b.DestX <= b.DestY {
		t.Fatalf("PiP placement or opacity lost: %+v", top)
	}
	p, err = p.SetClipOpacity(ref, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.ToggleTrackControl("V3", project.Visibility)
	if err != nil {
		t.Fatal(err)
	}
	e, err = Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = e.PlanLayers(20)
	if err != nil || len(plan.Layers) != 1 || plan.Layers[0].Layer.Ref.TrackID != "V1" {
		t.Fatalf("hidden or transparent layers contributed: %+v %v", plan, err)
	}
	f, err := e.At(20)
	if err != nil || len(f.Audio) != 3 {
		t.Fatal("layer opacity or visibility affected audio")
	}
	plan, err = e.PlanLayers(e.Duration())
	if err != nil || len(plan.Layers) != 0 {
		t.Fatal("end boundary retained expired layers")
	}
}
