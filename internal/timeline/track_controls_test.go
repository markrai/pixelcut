package timeline

import (
	"testing"

	"github.com/markrai/pixelcut/internal/project"
)

func TestLockedTrackGesture(t *testing.T) {
	p := dragFixture(t)
	p, target, err := p.AddTrack(project.VideoTrack)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.ToggleTrackControl(target, project.Lock)
	if err != nil {
		t.Fatal(err)
	}
	req := DragRequest{
		Ref:         project.ClipRef{TrackID: "V1", ClipID: "first"},
		TargetTrack: target,
		Mode:        "move",
		Scale:       100,
		Pixels:      100,
	}
	if EvaluateDrag(p, req).Valid {
		t.Fatal("locked destination accepted")
	}
	p, err = p.ToggleTrackControl("V1", project.Lock)
	if err != nil {
		t.Fatal(err)
	}
	req.TargetTrack = "V1"
	req.Mode = "right"
	if EvaluateDrag(p, req).Valid {
		t.Fatal("locked source trim accepted")
	}
}
