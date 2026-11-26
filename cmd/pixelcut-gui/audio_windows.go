package main

import (
	"fmt"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func (e *editor) syncPreviewLabel() {
	if e.previewLabel == 0 {
		return
	}
	preview := "Source preview / SDR"
	if e.prefs.HDR {
		preview = "Source preview / HDR (experimental)"
	}
	if e.model.TrackIndex() >= 0 {
		policy := e.model.Project().TrackPolicies()[e.trackID()]
		if !policy.Video && !policy.Audio {
			preview = "Preview suppressed by track controls"
		} else if !policy.Video {
			preview = "Audio preview / active track only"
		} else if !policy.Audio {
			preview += " / muted"
		}
	}
	if e.layerPlan != nil {
		preview = "Layer layout / schematic only"
	} else if c, ok := e.selected(); ok && c.OpacityPercent() != 100 {
		preview += " / layer opacity not applied"
	}
	change(e.previewLabel, preview)
}

func audioClipLabel(c project.Clip) string {
	if c.AudioDetached {
		return " / video only"
	}
	return ""
}

func (e *editor) editClipAudio(detach bool) error {
	c, ok := e.selected()
	if !ok || e.model.Count() != 1 || e.audioTrack() {
		return fmt.Errorf("select one video clip")
	}
	p := e.model.Project()
	ref := project.ClipRef{TrackID: e.trackID(), ClipID: c.ID}
	if err := p.RequireEditableTrack(ref.TrackID); err != nil {
		return err
	}
	var q project.Project
	if detach {
		a, ok := p.Asset(c.AssetID)
		if !ok || !a.HasAudio || c.AudioDetached {
			return fmt.Errorf("select a video clip with attached source audio")
		}
		id := nextID("audio")
		for _, t := range p.Tracks {
			if t.Kind != project.AudioTrack {
				continue
			}
			candidate, err := p.DetachAudio(ref, t.ID, id)
			if err == nil {
				q = candidate
				break
			}
		}
		if q.Version == 0 {
			expanded, target, err := p.AddTrack(project.AudioTrack)
			if err != nil {
				return err
			}
			q, err = expanded.DetachAudio(ref, target, id)
			if err != nil {
				return err
			}
		}
		if err := e.commitWithSelection(q, editorstate.SelectionUpdate{}); err != nil {
			return err
		}
		e.tell("Audio detached. Select its audio track to audition independently.")
		return nil
	}
	matches := p.MatchingAudio(ref)
	if len(matches) != 1 {
		return fmt.Errorf("reattach requires exactly one audio clip with identical source and timeline intervals")
	}
	q, err := p.ReattachAudio(ref, matches[0])
	if err != nil {
		return err
	}
	if err = e.commitWithSelection(q, editorstate.SelectionUpdate{}); err != nil {
		return err
	}
	e.tell("Source audio reattached. Independent audio clip removed.")
	return nil
}
