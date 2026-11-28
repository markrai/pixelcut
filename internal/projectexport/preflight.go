package projectexport

import (
	"context"
	"errors"
	"fmt"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/project"
)

type Inspector interface {
	Inspect(context.Context, string) (media.Report, error)
}

type Source struct {
	Asset    project.Asset
	Identity mediacache.Source
	Rotation int
	Chroma   string
	video    media.Stream
}

type Prepared struct {
	Plan    Plan
	Sources []Source
}

func Prepare(ctx context.Context, inspector Inspector, p project.Project, color string, fps int) (Prepared, error) {
	if inspector == nil {
		return Prepared{}, errors.New("media inspector unavailable")
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	plan, err := Build(p, color, fps)
	if err != nil {
		return Prepared{}, err
	}
	prepared := Prepared{Plan: plan}
	used := make(map[string]bool)
	visible := make(map[string]bool)
	for _, span := range plan.Spans {
		for _, layer := range span.Layers {
			id := layer.Layer.Asset.ID
			used[id], visible[id] = true, true
		}
		if span.Transition != nil {
			id := span.Transition.From.Layer.Asset.ID
			used[id], visible[id] = true, true
		}
		for _, audio := range span.Audio {
			used[audio.Asset.ID] = true
		}
	}
	type inspected struct {
		identity mediacache.Source
		report   media.Report
	}
	reports := make(map[string]inspected)
	for _, asset := range p.Assets {
		if !used[asset.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Prepared{}, err
		}
		identity, err := mediacache.Identify(asset.Path)
		if err != nil {
			return Prepared{}, fmt.Errorf("asset %s unavailable: %w", asset.ID, err)
		}
		entry, ok := reports[identity.Path]
		if ok && entry.identity != identity {
			return Prepared{}, fmt.Errorf("asset %s changed during preflight", asset.ID)
		}
		if !ok {
			report, err := inspector.Inspect(ctx, identity.Path)
			if err != nil {
				return Prepared{}, fmt.Errorf("inspect asset %s: %w", asset.ID, err)
			}
			entry = inspected{identity: identity, report: report}
			reports[identity.Path] = entry
			if n := report.ExcludedProjectMetadata(); n > 0 {
				prepared.Plan.Warnings = append(prepared.Plan.Warnings, fmt.Sprintf("asset %s: %d recognized Pixel timed metadata stream(s) will be excluded; source unchanged", asset.ID, n))
			}
		}
		var actual project.Asset
		if asset.Classified() {
			actual, err = project.FromImportReport(asset.ID, identity.Path, entry.report)
		} else {
			if err = entry.report.ValidateProjectExportSource(); err != nil {
				return Prepared{}, fmt.Errorf("asset %s: %w", asset.ID, err)
			}
			actual, err = project.FromReport(asset.ID, identity.Path, entry.report)
		}
		if err != nil {
			return Prepared{}, fmt.Errorf("asset %s: %w", asset.ID, err)
		}
		if actual.Duration != asset.Duration || actual.Width != asset.Width || actual.Height != asset.Height || actual.HasAudio != asset.HasAudio || (asset.Range != "" && asset.Range != actual.Range) {
			return Prepared{}, fmt.Errorf("asset %s metadata changed; relink before export", asset.ID)
		}
		if asset.Classified() && (asset.SourceColor() != actual.SourceColor() || asset.ImportOnly()) {
			return Prepared{}, fmt.Errorf("asset %s color interpretation changed or is not confirmed; relink before export", asset.ID)
		}
		actual.ColorConfirmed = asset.ColorConfirmed
		v, err := entry.report.Video()
		if err != nil {
			return Prepared{}, err
		}
		if visible[asset.ID] && v.HasStaticHDRMetadata() {
			prepared.Plan.Warnings = append(prepared.Plan.Warnings, fmt.Sprintf("asset %s: rendered output does not guarantee preservation or recomputation of source static HDR mastering/light-level metadata; source unchanged", asset.ID))
		}
		chroma := v.ChromaLocation
		if actual.Classified() {
			chroma = actual.Chroma
		}
		if visible[asset.ID] && (actual.Range != "tv" || chroma != "left") {
			prepared.Plan.Warnings = append(prepared.Plan.Warnings, fmt.Sprintf("asset %s: project video export normalizes %s range / %s chroma to limited range / left chroma", asset.ID, actual.Range, chroma))
		}
		if visible[asset.ID] && actual.ColorFamily == media.ColorSDR709 {
			warning := "SDR uses CPU decoding and native BT.709 processing"
			if color == media.ExportColorHLG {
				warning = "SDR uses CPU decoding and the explicit SDR-to-HLG mapping"
			}
			prepared.Plan.Warnings = append(prepared.Plan.Warnings, fmt.Sprintf("asset %s: %s", asset.ID, warning))
		}
		if visible[asset.ID] {
			if engine, ok := inspector.(media.Engine); ok {
				engine = exportDecodeEngine(engine, actual)
				if _, _, err := engine.VideoDecodePipeline(v, "", false); err != nil {
					return Prepared{}, fmt.Errorf("asset %s: %w", asset.ID, err)
				}
			}
		}
		rotation, err := v.DisplayRotation()
		if err != nil {
			return Prepared{}, err
		}
		prepared.Sources = append(prepared.Sources, Source{Asset: actual, Identity: identity, Rotation: rotation, Chroma: chroma, video: v})
	}
	if err := prepared.VerifySources(ctx); err != nil {
		return Prepared{}, err
	}
	return prepared, nil
}

func (p Prepared) VerifySources(ctx context.Context) error {
	for _, s := range p.Sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		now, err := mediacache.Identify(s.Identity.Path)
		if err != nil || now != s.Identity {
			return fmt.Errorf("asset %s changed after export preflight", s.Asset.ID)
		}
	}
	return ctx.Err()
}
