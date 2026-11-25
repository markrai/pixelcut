package timeline

import (
	"math"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

type DragRequest struct {
	Ref            project.ClipRef
	TargetTrack    string
	Mode           string
	GroupIDs       []string
	Pixels         int32
	Scale          float64
	Marker         int64
	Magnetic       bool
	Bypass         bool
	Ripple         bool
	Frames         *media.FrameIndex
	SourceDuration int64
}

type DragResult struct {
	Project     project.Project
	Clip        project.Clip
	Valid       bool
	Snapped     bool
	Target      int64
	RippleDelta int64
}

func EvaluateDrag(p project.Project, req DragRequest) DragResult {
	result := DragResult{Project: p}
	original, ok := p.Clip(req.Ref)
	if !ok || req.Scale <= 0 || math.IsNaN(req.Scale) || math.IsInf(req.Scale, 0) {
		return result
	}
	result.Clip = original
	raw := max(-1e15, min(1e15, float64(req.Pixels)/req.Scale*float64(project.TicksPerSecond)))
	delta := int64(math.Round(raw/10000)) * 10000
	tolerance := SnapTolerance(req.Scale, req.Magnetic && !req.Bypass)
	track, id := req.Ref.TrackID, req.Ref.ClipID
	if req.TargetTrack != "" && req.TargetTrack != track {
		if req.Mode != "move" || req.Ripple || req.Frames != nil {
			return result
		}
		ids := req.GroupIDs
		if len(ids) == 0 {
			ids = []string{id}
		}
		r, err := p.MoveClipsTo(track, req.TargetTrack, ids, delta, req.Marker, tolerance)
		result.Project, result.Valid, result.Snapped, result.Target = r.Project, err == nil, r.Snapped, r.Target
		if c, ok := r.Project.Clip(project.ClipRef{TrackID: req.TargetTrack, ClipID: id}); ok {
			result.Clip = c
		} else {
			result.Valid = false
		}
		return result
	}
	if req.Frames != nil && !req.Bypass {
		base := original.In
		if req.Mode == "right" {
			base = original.Out
		}
		if req.SourceDuration < base {
			return result
		}
		offset := int64(math.Round(raw))
		wanted := base + max(-base, min(req.SourceDuration-base, offset))
		frameDelta, _, ok := FrameTrimDelta(req.Frames, original, req.SourceDuration, req.Mode, wanted, req.Ripple)
		if !ok {
			return result
		}
		if req.Ripple {
			r, err := p.RippleTrim(track, id, req.Mode, frameDelta, -1, 0)
			result.Project, result.RippleDelta = r.Project, r.Delta
			result.Valid = err == nil && r.Delta == frameDelta
		} else {
			c, err := p.DragClip(track, id, req.Mode, frameDelta)
			result.Clip = c
			result.Valid = err == nil && ((req.Mode == "left" && c.In == original.In+frameDelta) || (req.Mode == "right" && c.Out == original.Out+frameDelta))
		}
		if result.Valid {
			result.Snapped = true
			if req.Mode == "right" {
				result.Target = original.End() + frameDelta
			} else {
				result.Target = original.Start
				if frameDelta >= -original.Start && frameDelta <= math.MaxInt64-original.Start {
					result.Target += frameDelta
				}
			}
		}
	} else if req.Ripple {
		r, err := p.RippleTrim(track, id, req.Mode, delta, req.Marker, tolerance)
		result.Project, result.RippleDelta = r.Project, r.Delta
		result.Valid, result.Snapped, result.Target = err == nil, r.Snapped, r.Target
	} else if len(req.GroupIDs) > 1 {
		r, err := p.MoveClips(track, req.GroupIDs, delta, req.Marker, tolerance)
		result.Project = r.Project
		result.Valid, result.Snapped, result.Target = err == nil, r.Snapped, r.Target
	} else {
		r, err := p.SnapDrag(track, id, req.Mode, delta, req.Marker, tolerance)
		result.Clip = r.Clip
		result.Valid, result.Snapped, result.Target = err == nil, r.Snapped, r.Target
	}
	return result
}
