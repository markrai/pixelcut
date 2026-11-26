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
	for _, span := range plan.Spans {
		for _, layer := range span.Layers {
			id := layer.Layer.Asset.ID
			used[id] = true
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
			if err = report.ValidateProjectExportSource(); err != nil {
				return Prepared{}, fmt.Errorf("asset %s: %w", asset.ID, err)
			}
			entry = inspected{identity: identity, report: report}
			reports[identity.Path] = entry
			if n := report.ExcludedProjectMetadata(); n > 0 {
				prepared.Plan.Warnings = append(prepared.Plan.Warnings, fmt.Sprintf("asset %s: %d recognized Pixel timed metadata stream(s) will be excluded; source unchanged", asset.ID, n))
			}
		}
		actual, err := project.FromReport(asset.ID, identity.Path, entry.report)
		if err != nil {
			return Prepared{}, fmt.Errorf("asset %s: %w", asset.ID, err)
		}
		if actual.Duration != asset.Duration || actual.Width != asset.Width || actual.Height != asset.Height || actual.HasAudio != asset.HasAudio || (asset.Range != "" && asset.Range != actual.Range) {
			return Prepared{}, fmt.Errorf("asset %s metadata changed; relink before export", asset.ID)
		}
		v, err := entry.report.Video()
		if err != nil {
			return Prepared{}, err
		}
		if v.Range != "tv" || v.ChromaLocation != "left" {
			prepared.Plan.Warnings = append(prepared.Plan.Warnings, fmt.Sprintf("asset %s: project video export normalizes %s range / %s chroma to limited range / left chroma", asset.ID, v.Range, v.ChromaLocation))
		}
		rotation, err := v.DisplayRotation()
		if err != nil {
			return Prepared{}, err
		}
		prepared.Sources = append(prepared.Sources, Source{Asset: actual, Identity: identity, Rotation: rotation, Chroma: v.ChromaLocation})
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
