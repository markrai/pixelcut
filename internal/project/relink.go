package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxRelinkCandidates = 64
const MaxRelinkDirectoryEntries = 100000

func SourceAvailable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (p Project) MissingAssets() []Asset {
	var missing []Asset
	for _, a := range p.Assets {
		if !SourceAvailable(a.Path) {
			missing = append(missing, a)
		}
	}
	return missing
}

func (p Project) RelinkAsset(id string, candidate Asset, replace bool) (Project, error) {
	if err := p.Validate(); err != nil {
		return p, err
	}
	old, ok := p.Asset(id)
	if !ok {
		return p, ErrAssetNotFound
	}
	if err := p.RequireEditableAsset(id); err != nil {
		return p, err
	}
	if !replace && SourceAvailable(old.Path) {
		return p, errors.New("original source is available; use Replace source instead")
	}
	path, err := filepath.Abs(candidate.Path)
	if err != nil {
		return p, err
	}
	if !SourceAvailable(path) {
		return p, errors.New("candidate source is unavailable")
	}
	if sameSource(old.Path, path) {
		return p, errors.New("candidate is already the current source")
	}
	for _, a := range p.Assets {
		if a.ID != id && sameSource(a.Path, path) {
			return p, errors.New("candidate already belongs to another media-bin asset")
		}
	}
	if candidate.Width != old.Width || candidate.Height != old.Height {
		return p, errors.New("display dimensions or orientation do not match")
	}
	if candidate.Range != "tv" && candidate.Range != "pc" {
		return p, errors.New("candidate requires validated source color range")
	}
	if old.ColorFamily != candidate.ColorFamily || (old.ImportOnly() && old.SourceColor() != candidate.SourceColor()) {
		return p, errors.New("source color classification does not match")
	}
	if old.Range != "" && candidate.Range != old.Range {
		return p, errors.New("source color range does not match")
	}
	if candidate.HasAudio != old.HasAudio {
		return p, errors.New("source audio availability does not match")
	}
	if candidate.Duration <= 0 || (!replace && candidate.Duration != old.Duration) {
		return p, errors.New("source duration does not match")
	}
	candidate.ID = id
	candidate.Path = path
	q := p.Clone()
	for i, a := range q.Assets {
		if a.ID == id {
			q.Assets[i] = candidate
			break
		}
	}
	if err = q.Validate(); err != nil {
		return p, fmt.Errorf("replacement would invalidate existing edits: %w", err)
	}
	return q, nil
}

type RelinkCandidate struct{ AssetID, Path string }
type RelinkIssue struct{ AssetID, Name, Reason string }

func (p Project) FindRelinkCandidates(ctx context.Context, dir string) ([]RelinkCandidate, []RelinkIssue, error) {
	if err := p.Validate(); err != nil {
		return nil, nil, err
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	missing := p.MissingAssets()
	if len(missing) == 0 {
		return nil, nil, nil
	}
	wanted := map[string]int{}
	for _, a := range missing {
		wanted[strings.ToLower(filepath.Base(a.Path))]++
	}
	f, err := os.Open(dir)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return nil, nil, errors.New("select a directory")
	}
	matches := map[string][]string{}
	count := 0
	for {
		if err = ctx.Err(); err != nil {
			return nil, nil, err
		}
		entries, readErr := f.ReadDir(512)
		count += len(entries)
		if count > MaxRelinkDirectoryEntries {
			return nil, nil, errors.New("directory exceeds search limit")
		}
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			if wanted[name] == 0 || len(matches[name]) >= 2 {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if SourceAvailable(path) {
				matches[name] = append(matches[name], path)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, readErr
		}
	}
	var candidates []RelinkCandidate
	var issues []RelinkIssue
	for _, a := range missing {
		name := filepath.Base(a.Path)
		key := strings.ToLower(name)
		reason := ""
		switch {
		case wanted[key] > 1:
			reason = "multiple missing assets share this filename; relink individually"
		case len(matches[key]) == 0:
			reason = "no matching file in selected folder"
		case len(matches[key]) > 1:
			reason = "ambiguous filename; relink individually"
		case len(candidates) >= MaxRelinkCandidates:
			reason = "batch limit reached; run search again"
		default:
			candidates = append(candidates, RelinkCandidate{a.ID, matches[key][0]})
		}
		if reason != "" {
			issues = append(issues, RelinkIssue{a.ID, name, reason})
		}
	}
	return candidates, issues, nil
}
