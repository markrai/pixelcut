package main

import (
	"fmt"
	"unsafe"

	"github.com/markrai/pixelcut/internal/project"
)

const (
	clipGainBase    = 9200
	clipGainDown    = 9250
	clipGainUp      = 9251
	clipGainDownTen = 9252
	clipGainUpTen   = 9253
	clipGainHeader  = 9254
	clipFadeInMenu  = 9260
	clipFadeOutMenu = 9261
	clipFadeReset   = 9262
	clipFadeBase    = 9270
)

var clipGainChoices = [...]int{0, 10, 25, 50, 75, 100, 125, 150, 200, 300, 400}

func clipAudioAvailable(p project.Project, ref project.ClipRef) bool {
	c, ok := p.Clip(ref)
	if !ok || c.AudioDetached || p.RequireEditableTrack(ref.TrackID) != nil {
		return false
	}
	a, ok := p.Asset(c.AssetID)
	return ok && a.HasAudio
}

func clipAudioButtonLabel(c project.Clip) string {
	if c.AudioMuted {
		return "Audio: muted"
	}
	return fmt.Sprintf("Audio: %d%%", c.GainPercent())
}

func clipAudioItems(c project.Clip) []menuItem {
	label := "Mute clip audio"
	if c.AudioMuted {
		label = "Unmute clip audio"
	}
	gain := c.GainPercent()
	items := []menuItem{
		{id: menuClipMute, label: label, enabled: true, checked: c.AudioMuted},
		separator(),
		item(clipGainHeader, fmt.Sprintf("Clip volume: %d%% (100%% = original)", gain), false),
	}
	for i, percent := range clipGainChoices {
		label := fmt.Sprintf("%d%%", percent)
		if percent == 100 {
			label += " (original)"
		}
		items = append(items, menuItem{id: clipGainBase + i, label: label, enabled: true, checked: gain == percent})
	}
	return append(items, separator(),
		item(clipGainDownTen, "Decrease volume by 10 points", gain > 0),
		item(clipGainUpTen, "Increase volume by 10 points", gain < project.MaxGainPercent),
		item(clipGainDown, "Decrease volume by 1 point", gain > 0),
		item(clipGainUp, "Increase volume by 1 point", gain < project.MaxGainPercent), separator(),
		item(clipFadeInMenu, fmt.Sprintf("Fade in: %s...", fadeLabel(c.FadeIn)), true),
		item(clipFadeOutMenu, fmt.Sprintf("Fade out: %s...", fadeLabel(c.FadeOut)), true),
		item(clipFadeReset, "Remove both fades", c.HasFades()))
}

func clipAudioChoice(p project.Project, ref project.ClipRef, id int) (project.Project, error) {
	c, ok := p.Clip(ref)
	if !ok {
		return p, project.ErrClipNotFound
	}
	if id == clipFadeReset {
		return p.SetClipFades(ref, 0, 0)
	}
	if id == menuClipMute {
		return p.SetClipMuted(ref, !c.AudioMuted)
	}
	gain := c.GainPercent()
	switch id {
	case clipGainDown:
		gain--
	case clipGainUp:
		gain++
	case clipGainDownTen:
		gain -= 10
	case clipGainUpTen:
		gain += 10
	default:
		i := id - clipGainBase
		if i < 0 || i >= len(clipGainChoices) {
			return p, fmt.Errorf("unknown clip audio action")
		}
		gain = clipGainChoices[i]
	}
	return p.SetClipGain(ref, max(0, min(project.MaxGainPercent, gain)))
}

func (e *editor) syncClipAudioControl() {
	h := e.controls[idClipAudio]
	if h == 0 {
		return
	}
	c, ok := e.selected()
	label := "Audio..."
	var enabled uintptr
	if ok && e.model.Count() == 1 {
		p := e.model.Project()
		a, present := p.Asset(c.AssetID)
		if present && a.HasAudio && !c.AudioDetached {
			label = clipAudioButtonLabel(c)
			if !e.busy && clipAudioAvailable(p, project.ClipRef{TrackID: e.trackID(), ClipID: c.ID}) {
				enabled = 1
			}
		}
	}
	change(h, label)
	user.NewProc("EnableWindow").Call(h, enabled)
	user.NewProc("InvalidateRect").Call(h, 0, 1)
}

func (e *editor) openClipAudio() error {
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idClipAudio], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate clip Audio button")
	}
	return e.clipAudioAction(idClipAudio, point{r.Left, r.Bottom})
}

func (e *editor) clipAudioAction(id int, screen point) error {
	if !e.canOpenMenu() {
		return nil
	}
	e.updatePlaybackClock()
	if e.playing() {
		if err := e.pausePreview(); err != nil {
			return err
		}
	}
	c, ok := e.selected()
	if !ok || e.model.Count() != 1 {
		return fmt.Errorf("select a single clip with source audio")
	}
	p := e.model.Project()
	ref := project.ClipRef{TrackID: e.trackID(), ClipID: c.ID}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return err
	}
	if !clipAudioAvailable(p, ref) {
		return fmt.Errorf("select an attached or independent audio clip")
	}
	if id == idClipAudio {
		id = e.popup(clipAudioItems(c), screen)
		if id == 0 {
			return nil
		}
	}
	if e.model.Count() != 1 || e.trackID() != ref.TrackID || e.model.PrimaryID() != ref.ClipID || e.model.Fingerprint() != p.Fingerprint() {
		return fmt.Errorf("selection changed; reopen the clip audio menu")
	}
	var q project.Project
	var err error
	if id == clipFadeInMenu || id == clipFadeOutMenu {
		in, out := c.FadeIn, c.FadeOut
		current := in
		if id == clipFadeOutMenu {
			current = out
		}
		choice := e.popup(fadeItems(current, c.Duration()), screen)
		if choice == 0 {
			return nil
		}
		if e.model.Fingerprint() != p.Fingerprint() || e.model.Count() != 1 || e.trackID() != ref.TrackID || e.model.PrimaryID() != ref.ClipID {
			return fmt.Errorf("selection changed; reopen the clip audio menu")
		}
		value, ok := fadeChoice(choice)
		if !ok {
			return fmt.Errorf("invalid fade choice")
		}
		if id == clipFadeInMenu {
			in = value
		} else {
			out = value
		}
		q, err = p.SetClipFades(ref, in, out)
	} else {
		q, err = clipAudioChoice(p, ref, id)
	}
	if err != nil {
		return err
	}
	if err := e.commit(q); err != nil {
		return err
	}
	c, _ = q.Clip(ref)
	status := fmt.Sprintf("Clip volume %d%%.", c.GainPercent())
	if c.AudioMuted {
		status += " Muted; volume retained."
	}
	if c.HasFades() {
		status += fmt.Sprintf(" Fade in %s / out %s. Use composite playback to hear fades.", fadeLabel(c.FadeIn), fadeLabel(c.FadeOut))
	}
	e.tell(status)
	return nil
}

var clipFadeChoices = [...]int64{0, 100000, 250000, 500000, 1000000, 2000000, 3000000, 5000000, 10000000, 30000000, 60000000}

func fadeLabel(ticks int64) string {
	if ticks == 0 {
		return "Off"
	}
	return fmt.Sprintf("%g s", project.Seconds(ticks))
}

func fadeItems(current, duration int64) []menuItem {
	items := make([]menuItem, 0, len(clipFadeChoices))
	for i, ticks := range clipFadeChoices {
		label := fadeLabel(ticks)
		if ticks > duration {
			label += " (limited to clip)"
		}
		items = append(items, menuItem{id: clipFadeBase + i, label: label, enabled: true, checked: current == ticks})
	}
	return items
}

func fadeChoice(id int) (int64, bool) {
	i := id - clipFadeBase
	if i < 0 || i >= len(clipFadeChoices) {
		return 0, false
	}
	return clipFadeChoices[i], true
}
