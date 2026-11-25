package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type transformSession struct {
	clipID          string
	asset           project.Asset
	crop            *project.Crop
	original, draft *media.Transform
	serial          uint64
	lastInput       string
	lastChange      time.Time
	valid           bool
	rotation        int
	flipH, flipV    bool
}

func defaultTransform(a project.Asset, c *project.Crop) media.Transform {
	w, h := a.Width, a.Height
	if c != nil {
		w, h = c.Width, c.Height
	}
	return media.Transform{CanvasWidth: w, CanvasHeight: h, Scale: 100}
}

func (e *editor) initTransformInspector() {
	for _, p := range e.layout {
		if p.x >= 825 && p.y >= 50 && p.y < 410 {
			e.inspectorControls = append(e.inspectorControls, p.h)
		}
	}
	start := len(e.layout)
	label("Transform / orientation", 825, 50, 270)
	label("Canvas width (px)", 825, 83, 145)
	edit("", 975, 80, 121, idCanvasW)
	label("Canvas height (px)", 825, 115, 145)
	edit("", 975, 112, 121, idCanvasH)
	label("Scale (%)", 825, 147, 145)
	edit("", 975, 144, 121, idScale)
	label("Position X (px)", 825, 179, 145)
	edit("", 975, 176, 121, idPositionX)
	label("Position Y (px)", 825, 211, 145)
	edit("", 975, 208, 121, idPositionY)
	button("Rotate left", 825, 244, 130, idRotateLeft)
	button("Rotate right", 961, 244, 135, idRotateRight)
	button("Flip H: off", 825, 276, 130, idFlipH)
	button("Flip V: off", 961, 276, 135, idFlipV)
	label("100% fits canvas; offsets from center.", 825, 306, 270)
	button("Apply transform", 825, 330, 130, idTransformApply)
	button("Cancel", 961, 330, 135, idTransformCancel)
	button("Reset draft", 825, 370, 130, idTransformReset)
	for _, p := range e.layout[start:] {
		e.transformControls = append(e.transformControls, p.h)
		user.NewProc("ShowWindow").Call(p.h, 0)
	}
}

func (e *editor) showTransformInspector(show bool) {
	for _, h := range e.inspectorControls {
		mode := uintptr(5)
		if show {
			mode = 0
		}
		user.NewProc("ShowWindow").Call(h, mode)
	}
	for _, h := range e.transformControls {
		mode := uintptr(0)
		if show {
			mode = 5
		}
		user.NewProc("ShowWindow").Call(h, mode)
	}
}

func (e *editor) writeTransform(t media.Transform) {
	if s := e.transformSession; s != nil {
		s.rotation, s.flipH, s.flipV = t.Rotation, t.FlipHorizontal, t.FlipVertical
		e.updateOrientationButtons()
	}
	for i, id := range []int{idCanvasW, idCanvasH, idScale, idPositionX, idPositionY} {
		change(e.controls[id], strconv.Itoa([]int{t.CanvasWidth, t.CanvasHeight, t.Scale, t.X, t.Y}[i]))
	}
}

func (e *editor) beginTransform() error {
	c, ok := e.selected()
	if !ok {
		return errors.New("Select a video clip first")
	}
	a, ok := e.p.Asset(c.AssetID)
	if !ok {
		return errors.New("Source missing")
	}
	if !project.SourceAvailable(a.Path) {
		return errors.New("Source offline; use Relink")
	}
	if a.Range != "" && a.Range != "tv" {
		return errors.New("Canvas transforms currently require limited-range HLG")
	}
	if e.clock == nil || !e.previewActive() || e.clock.clip.ID != c.ID {
		if err := e.startPreview(true); err != nil {
			return err
		}
	}
	if err := e.pausePreview(); err != nil {
		return err
	}
	t := defaultTransform(a, c.Crop)
	if c.Transform != nil {
		t = *c.Transform
	}
	e.transformSession = &transformSession{clipID: c.ID, asset: a, crop: c.Crop, original: c.Transform, lastChange: time.Now()}
	e.showTransformInspector(true)
	e.writeTransform(t)
	user.NewProc("SetFocus").Call(e.controls[idScale])
	e.tell("Adjust position, scale and canvas. Preview updates as values change.")
	return nil
}

func (e *editor) endTransformUI() {
	e.transformSession = nil
	e.showTransformInspector(false)
}

func (e *editor) cancelTransform() {
	s := e.transformSession
	if s == nil {
		return
	}
	if e.clock != nil {
		e.clock.setComposition(s.asset, s.crop, s.original, false)
	}
	e.endTransformUI()
	user.NewProc("SetFocus").Call(e.timeline)
	e.tell("Transform canceled.")
}

func (e *editor) resetTransformDraft() {
	if s := e.transformSession; s != nil {
		e.writeTransform(defaultTransform(s.asset, s.crop))
	}
}

func (e *editor) readTransform() (media.Transform, string, error) {
	var values [5]int
	var input []string
	for _, id := range []int{idCanvasW, idCanvasH, idScale, idPositionX, idPositionY} {
		input = append(input, strings.TrimSpace(read(e.controls[id])))
	}
	s := e.transformSession
	if s == nil {
		return media.Transform{}, "", errors.New("No transform session")
	}
	key := fmt.Sprintf("%s,%d,%t,%t", strings.Join(input, ","), s.rotation, s.flipH, s.flipV)
	for i, v := range input {
		n, err := strconv.Atoi(v)
		if err != nil {
			return media.Transform{}, key, errors.New("Enter whole numbers for all transform fields")
		}
		values[i] = n
	}
	return media.Transform{CanvasWidth: values[0], CanvasHeight: values[1], Scale: values[2], X: values[3], Y: values[4], Rotation: s.rotation, FlipHorizontal: s.flipH, FlipVertical: s.flipV}, key, nil
}

func (e *editor) updateTransformDraft() {
	s := e.transformSession
	if s == nil || e.clock == nil {
		return
	}
	t, key, err := e.readTransform()
	if key != s.lastInput {
		s.lastInput = key
		s.lastChange = time.Now()
		s.valid = false
		return
	}
	if s.valid || time.Since(s.lastChange) < 120*time.Millisecond {
		return
	}
	if err != nil {
		e.tell(err.Error())
		return
	}
	w, h := s.asset.Width, s.asset.Height
	if s.crop != nil {
		w, h = s.crop.Width, s.crop.Height
	}
	if _, err = t.Placement(w, h); err != nil {
		e.tell(err.Error())
		return
	}
	var draft *media.Transform
	if t != defaultTransform(s.asset, s.crop) {
		draft = &t
	}
	s.serial = e.clock.setComposition(s.asset, s.crop, draft, false)
	s.draft = draft
	s.valid = s.serial != 0
	if s.valid {
		e.tell(fmt.Sprintf("Canvas %d × %d  |  Scale %d%%  |  Position %d, %d  |  Rotation %d°", t.CanvasWidth, t.CanvasHeight, t.Scale, t.X, t.Y, t.Rotation))
	}
}

func (e *editor) applyTransform() error {
	s := e.transformSession
	if s == nil || e.clock == nil {
		return errors.New("Transform preview unavailable")
	}
	_, key, err := e.readTransform()
	if err != nil {
		return err
	}
	if !s.valid || key != s.lastInput {
		return errors.New("Wait for valid transform values and preview")
	}
	state := e.clock.cropSample.Load()
	if state == nil || state.Serial != s.serial || !state.Ready || state.Error != "" {
		return errors.New("Wait for the transformed preview to be ready")
	}
	c, ok := e.selected()
	if !ok || c.ID != s.clipID {
		return errors.New("Clip selection changed")
	}
	c.Transform = s.draft
	q := e.p.Clone()
	q.Tracks[e.trackIndex()].Clips[e.clip] = c
	if err := q.Validate(); err != nil {
		return err
	}
	e.undo = append(e.undo, e.p.Clone())
	if len(e.undo) > 50 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
	e.p = q
	e.dirty = true
	e.endTransformUI()
	e.refresh()
	user.NewProc("SetFocus").Call(e.timeline)
	e.tell("Transform applied.")
	return nil
}

func (e *editor) updateOrientationButtons() {
	s := e.transformSession
	if s == nil {
		return
	}
	for _, item := range []struct {
		id   int
		name string
		on   bool
	}{{idFlipH, "Flip H", s.flipH}, {idFlipV, "Flip V", s.flipV}} {
		state := "off"
		if item.on {
			state = "on"
		}
		change(e.controls[item.id], item.name+": "+state)
		user.NewProc("InvalidateRect").Call(e.controls[item.id], 0, 1)
	}
}

func (e *editor) changeOrientation(id int) error {
	t, _, err := e.readTransform()
	if err != nil {
		return err
	}
	switch id {
	case idRotateLeft:
		t.Rotation = (t.Rotation + 270) % 360
		t.CanvasWidth, t.CanvasHeight = t.CanvasHeight, t.CanvasWidth
	case idRotateRight:
		t.Rotation = (t.Rotation + 90) % 360
		t.CanvasWidth, t.CanvasHeight = t.CanvasHeight, t.CanvasWidth
	case idFlipH:
		t.FlipHorizontal = !t.FlipHorizontal
	case idFlipV:
		t.FlipVertical = !t.FlipVertical
	default:
		return errors.New("Unknown orientation control")
	}
	e.writeTransform(t)
	return nil
}
