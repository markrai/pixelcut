package projectexport

import (
	"errors"
	"fmt"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
)

const MaxDuration int64 = 24 * 60 * 60 * project.TicksPerSecond
const MaxSpans = 10000
const MaxContributions = 100000

type Span struct {
	Start, End             int64
	FirstFrame, EndFrame   int64
	FirstSample, EndSample int64
	Layers                 []composition.PlannedLayer
	Audio                  []composition.AudioContribution
	Transition             *composition.PlannedTransition `json:",omitempty"`
}

type Plan struct {
	Contract           media.ExportContract
	ProjectFingerprint string
	Duration           int64
	VideoFrames        int64
	ContentAudioFrames int64
	OutputAudioFrames  int64
	Spans              []Span
	Warnings           []string
}

func frameCeiling(at int64, fps int) int64 {
	return at/project.TicksPerSecond*int64(fps) + (at%project.TicksPerSecond*int64(fps)+project.TicksPerSecond-1)/project.TicksPerSecond
}

func Build(p project.Project, color string, fps int) (Plan, error) {
	return buildTimelinePlan(p, color, fps)
}

func ValidateTransitionLayout(p project.Project) error {
	_, err := buildTimelinePlan(p, media.ExportColorHLG, 30)
	return err
}

func buildTimelinePlan(p project.Project, color string, fps int) (Plan, error) {
	e, err := composition.Compile(p)
	if err != nil {
		return Plan{}, err
	}
	duration := e.Duration()
	if duration <= 0 || duration > MaxDuration {
		return Plan{}, errors.New("project export requires a positive timeline of at most 24 hours")
	}
	contract := media.ExportContract{Color: color, Canvas: p.CompositionCanvas(), FPS: fps}
	if err = contract.Validate(); err != nil {
		return Plan{}, err
	}
	plan := Plan{Contract: contract, ProjectFingerprint: p.Fingerprint(), Duration: duration, VideoFrames: frameCeiling(duration, fps)}
	contributions := 0
	visible := false
	mixed := false
	zeroFrameSpans := 0
	for at := int64(0); at < duration; {
		if len(plan.Spans) >= MaxSpans {
			return Plan{}, errors.New("project export span budget exceeded")
		}
		f, err := e.At(at)
		if err != nil {
			return Plan{}, err
		}
		layers, err := e.PlanLayers(at)
		if err != nil {
			return Plan{}, err
		}
		if f.NextBoundary <= at || f.NextBoundary > duration {
			return Plan{}, errors.New("invalid export boundary")
		}
		contributions += len(layers.Layers) + len(f.Audio) + len(layers.Transitions)
		if contributions > MaxContributions {
			return Plan{}, errors.New("project export contribution budget exceeded")
		}
		s := Span{Start: at, End: f.NextBoundary, FirstFrame: frameCeiling(at, fps), EndFrame: frameCeiling(f.NextBoundary, fps), FirstSample: media.PCMFramesForTicks(at), EndSample: media.PCMFramesForTicks(f.NextBoundary), Layers: layers.Layers, Audio: f.Audio}
		if len(layers.Transitions) > 1 {
			return Plan{}, errors.New("export supports at most one active video transition at a time")
		}
		if len(layers.Transitions) == 1 {
			s.Transition = &layers.Transitions[0]
			if err := validateTransitionSpan(s, contract.Canvas); err != nil {
				return Plan{}, err
			}
		}
		for _, layer := range spanDecodeLayers(s) {
			mixed = mixed || layer.Layer.Asset.ColorFamily == media.ColorSDR709
		}
		if len(s.Layers) > 0 {
			visible = true
			if s.FirstFrame == s.EndFrame {
				zeroFrameSpans++
			}
		}
		plan.Contract.Audio = plan.Contract.Audio || len(s.Audio) > 0
		plan.Spans = append(plan.Spans, s)
		at = s.End
	}
	if !visible {
		return Plan{}, errors.New("HLG project export requires at least one enabled nonzero-opacity video layer")
	}
	if plan.Contract.Audio {
		plan.ContentAudioFrames = media.PCMFramesForTicks(duration)
		plan.OutputAudioFrames = plan.VideoFrames * int64(media.PCMRate/fps)
	}
	if mixed {
		plan.Warnings = append(plan.Warnings, SDRMappingDescription)
	}
	if zeroFrameSpans != 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%d visible edit spans contain no sample on the selected CFR grid", zeroFrameSpans))
	}
	if duration*int64(fps)%project.TicksPerSecond != 0 {
		plan.Warnings = append(plan.Warnings, "CFR output extends to the end of the final frame; audio tail is padded with silence when present")
	}
	return plan, nil
}

func (p Plan) FrameTick(frame int64) (int64, bool) {
	if frame < 0 || frame >= p.VideoFrames || p.Duration <= 0 || p.Duration > MaxDuration || p.Contract.Validate() != nil || p.VideoFrames != frameCeiling(p.Duration, p.Contract.FPS) {
		return 0, false
	}
	return frame * project.TicksPerSecond / int64(p.Contract.FPS), true
}
