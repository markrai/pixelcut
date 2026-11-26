package main

import (
	"fmt"
	"strings"
	"unsafe"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

const (
	trackMenuSelect = 6000
	trackMenuMove   = 6100
	trackMenuVideo  = 6200
	trackMenuAudio  = 6201
	trackMenuRemove = 6202
	trackMenuHide   = 6203
	trackMenuMute   = 6204
	trackMenuSolo   = 6205
	trackMenuLock   = 6206
)

func (e *editor) trackIndex() int { return e.model.TrackIndex() }

func (e *editor) trackID() string  { return e.model.TrackID() }
func (e *editor) audioTrack() bool { return e.track().Kind == "audio" }

func trackStatus(t project.Track) string {
	var status []string
	if t.Hidden {
		status = append(status, "H")
	}
	if t.Muted {
		status = append(status, "M")
	}
	if t.Solo {
		status = append(status, "S")
	}
	if t.Locked {
		status = append(status, "L")
	}
	return strings.Join(status, " ")
}

func trackControlItems(t project.Track) []menuItem {
	return []menuItem{
		{id: trackMenuHide, label: "Hide video", enabled: t.Kind == project.VideoTrack, checked: t.Hidden},
		{id: trackMenuMute, label: "Mute audio", enabled: true, checked: t.Muted},
		{id: trackMenuSolo, label: "Solo track", enabled: true, checked: t.Solo},
		{id: trackMenuLock, label: "Lock edits", enabled: true, checked: t.Locked},
	}
}

func (e *editor) toggleTrackControl(id int) error {
	var control project.TrackControl
	switch id {
	case trackMenuHide:
		control = project.Visibility
	case trackMenuMute:
		control = project.Mute
	case trackMenuSolo:
		control = project.Solo
	case trackMenuLock:
		control = project.Lock
	default:
		return nil
	}
	q, err := e.model.Project().ToggleTrackControl(e.trackID(), control)
	if err != nil {
		return err
	}
	if err = e.commit(q); err != nil {
		return err
	}
	e.tell("Track controls updated. Ctrl+Z to undo.")
	return nil
}

func (e *editor) syncTrackUI() {
	e.syncPreviewLabel()
	if e.timeline != 0 && e.visibleTrack != e.trackID() {
		e.visibleTrack = e.trackID()
		e.laneScroll = e.timelineStack().Reveal(e.trackID())
	}
	if h := e.controls[idTracks]; h != 0 {
		change(h, fmt.Sprintf("Track: %s (%d)", e.trackID(), len(e.model.Project().Tracks)))
		user.NewProc("InvalidateRect").Call(h, 0, 1)
	}
	if h := e.controls[idAdd]; h != 0 {
		change(h, placementButtonLabel(*e.track()))
		user.NewProc("InvalidateRect").Call(h, 0, 1)
	}
	if e.timelineLabel != 0 {
		change(e.timelineLabel, "TIMELINE / "+e.trackID()+" / active track")
	}
}

func (e *editor) switchTrack(id string) {
	if e.model.Project().TrackIndex(id) < 0 || id == e.trackID() {
		return
	}
	e.stopPreview()
	e.model.SwitchTrack(id)
	e.refresh()
	e.tell("Editing " + id + ". Preview is isolated to the active track.")
}

func trackMenuItems(p project.Project, active string, selected []string) []menuItem {
	var items []menuItem
	for i, t := range p.Tracks {
		label := fmt.Sprintf("%s  %s (%d clips)", strings.ReplaceAll(t.ID, "&", "&&"), t.Kind, len(t.Clips))
		items = append(items, menuItem{id: trackMenuSelect + i, label: label, enabled: true, checked: t.ID == active})
	}
	items = append(items, separator(), item(trackMenuVideo, "Add video track", len(p.Tracks) < project.MaxTracks), item(trackMenuAudio, "Add audio track", len(p.Tracks) < project.MaxTracks))
	i := p.TrackIndex(active)
	items = append(items, item(trackMenuRemove, "Remove active empty track", i >= 2 && len(p.Tracks[i].Clips) == 0 && !p.Tracks[i].Locked))
	if i >= 0 {
		items = append(items, separator())
		items = append(items, trackControlItems(p.Tracks[i])...)
	}
	if i >= 0 && len(selected) > 0 {
		items = append(items, separator())
		for j, t := range p.Tracks {
			if j == i || t.Kind != p.Tracks[i].Kind {
				continue
			}
			_, err := p.TransferClips(active, t.ID, selected)
			items = append(items, item(trackMenuMove+j, "Move selection to "+strings.ReplaceAll(t.ID, "&", "&&"), err == nil))
		}
	}
	return append(items, separator(), item(menuLayerCanvas, "Composition canvas...", true), item(menuCompositePlay, "Play composite (SDR, silent)", true), item(menuSourcePlay, "Play active track (source preview)", true), item(menuCompositeStill, "Show composite frame at playhead", true), item(menuLayerLayout, "Show layer layout at playhead", true))
}

func (e *editor) showTracks() error {
	if !e.canOpenMenu() {
		return nil
	}
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idTracks], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate track menu")
	}
	id := e.popup(trackMenuItems(e.model.Project(), e.trackID(), e.selectionIDs()), point{r.Left, r.Bottom})
	if id == 0 {
		return nil
	}
	if id >= trackMenuSelect && id < trackMenuSelect+len(e.model.Project().Tracks) {
		e.switchTrack(e.model.Project().Tracks[id-trackMenuSelect].ID)
		return nil
	}
	if id == menuLayerCanvas || id == menuLayerLayout || id == menuCompositeStill || id == menuCompositePlay || id == menuSourcePlay {
		return e.layerAction(id, point{r.Left, r.Bottom})
	}
	if id >= trackMenuHide && id <= trackMenuLock {
		return e.toggleTrackControl(id)
	}
	var q project.Project
	var err error
	target := "V1"
	var ids []string
	switch {
	case id == trackMenuVideo || id == trackMenuAudio:
		kind := "video"
		if id == trackMenuAudio {
			kind = "audio"
		}
		q, target, err = e.model.Project().AddTrack(kind)
	case id == trackMenuRemove:
		q, err = e.model.Project().RemoveTrack(e.trackID())
	case id >= trackMenuMove && id < trackMenuMove+len(e.model.Project().Tracks):
		target = e.model.Project().Tracks[id-trackMenuMove].ID
		ids = e.selectionIDs()
		q, err = e.model.Project().TransferClips(e.trackID(), target, ids)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if err = q.ValidateEditorLayout(); err != nil {
		return err
	}
	e.stopPreview()
	update := editorstate.SelectionUpdate{TrackID: target, ReplaceClips: true, ClipIDs: ids}
	if len(ids) > 0 {
		update.Primary, update.Anchor = ids[0], ids[0]
	}
	if err = e.commitWithSelection(q, update); err != nil {
		return err
	}
	e.tell("Tracks updated. Active track: " + target + ". Ctrl+Z to undo.")
	return nil
}
