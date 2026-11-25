package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/markrai/pixelcut/internal/project"
)

const maxBytes = 16 << 20
const version = 1

type State struct {
	Project          project.Project `json:"project"`
	Asset            int             `json:"selected_asset"`
	Clip             int             `json:"selected_clip"`
	ActiveTrack      string          `json:"active_track,omitempty"`
	SelectedClips    []string        `json:"selected_clips,omitempty"`
	Playhead         int64           `json:"playhead_us"`
	Origin           int64           `json:"origin_us"`
	Zoom             float64         `json:"zoom"`
	Dirty            bool            `json:"dirty"`
	ProjectPath      string          `json:"project_path,omitempty"`
	ProjectRevision  string          `json:"project_revision,omitempty"`
	SavedFingerprint string          `json:"saved_fingerprint,omitempty"`
}

func (s State) Validate() error {
	if err := s.Project.ValidateEditorLayout(); err != nil {
		return err
	}
	id := s.ActiveTrack
	if id == "" {
		id = "V1"
	}
	i := s.Project.TrackIndex(id)
	if i < 0 {
		return errors.New("unknown active track")
	}
	t := s.Project.Tracks[i]
	if s.Asset < -1 || s.Asset >= len(s.Project.Assets) || s.Clip < -1 || s.Clip >= len(t.Clips) {
		return errors.New("invalid session selection")
	}
	if len(s.SelectedClips) > 0 {
		if len(s.SelectedClips) < 2 || s.Clip < 0 {
			return errors.New("invalid group selection")
		}
		ids := map[string]bool{}
		for _, id := range s.SelectedClips {
			if id == "" || ids[id] {
				return errors.New("invalid group selection IDs")
			}
			ids[id] = true
		}
		if !ids[t.Clips[s.Clip].ID] {
			return errors.New("primary clip outside selection")
		}
		count := 0
		for _, c := range t.Clips {
			if ids[c.ID] {
				count++
			}
		}
		if count != len(ids) {
			return errors.New("unknown group selection clip")
		}
	}
	if s.Playhead < 0 || s.Origin < 0 || math.IsNaN(s.Zoom) || math.IsInf(s.Zoom, 0) || (s.Zoom != 0 && (s.Zoom < .05 || s.Zoom > 1000)) {
		return errors.New("invalid session timeline view")
	}
	if s.ProjectPath != "" && !filepath.IsAbs(s.ProjectPath) {
		return errors.New("invalid saved project path")
	}
	if s.ProjectRevision != "" && (s.ProjectPath == "" || !project.ValidRevision(s.ProjectRevision)) {
		return errors.New("invalid project revision")
	}
	if s.SavedFingerprint != "" && !project.ValidRevision(s.SavedFingerprint) {
		return errors.New("invalid saved fingerprint")
	}
	return nil
}

type record struct {
	Version  int             `json:"version"`
	Sequence uint64          `json:"sequence"`
	State    json.RawMessage `json:"state"`
	Hash     [32]byte        `json:"sha256"`
}

type Store struct {
	dir      string
	active   int
	sequence uint64
	last     []byte
}

func slot(dir string, i int) string { return filepath.Join(dir, fmt.Sprintf("session-%d.json", i)) }

func decodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing session data")
	}
	return nil
}

func readSlot(path string) (record, State, error) {
	var r record
	var s State
	f, err := os.Open(path)
	if err != nil {
		return r, s, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return r, s, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return r, s, errors.New("invalid session file size")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return r, s, err
	}
	if len(b) > maxBytes {
		return r, s, errors.New("session too large")
	}
	if err = decodeStrict(b, &r); err != nil {
		return r, s, err
	}
	if r.Version != version || r.Sequence == 0 || sha256.Sum256(r.State) != r.Hash {
		return r, s, errors.New("invalid session envelope")
	}
	if err = decodeStrict(r.State, &s); err != nil {
		return r, s, err
	}
	if err = s.Validate(); err != nil {
		return r, s, err
	}
	return r, s, nil
}

func Open(dir string) (*Store, *State, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	store := &Store{dir: dir, active: -1}
	var state *State
	var bad error
	for i := 0; i < 2; i++ {
		r, s, err := readSlot(slot(dir, i))
		if err != nil {
			if !os.IsNotExist(err) {
				bad = err
			}
			continue
		}
		if state == nil || r.Sequence > store.sequence {
			store.active, store.sequence, store.last = i, r.Sequence, r.State
			state = &s
		}
	}
	if state == nil && bad != nil {
		return nil, nil, fmt.Errorf("no valid recovery snapshot: %w", bad)
	}
	return store, state, nil
}

func (s *Store) Save(state State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if bytes.Equal(payload, s.last) {
		return nil
	}
	if s.sequence == math.MaxUint64 {
		return errors.New("session sequence exhausted")
	}
	r := record{Version: version, Sequence: s.sequence + 1, State: payload, Hash: sha256.Sum256(payload)}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(b) > maxBytes {
		return errors.New("session exceeds recovery size limit")
	}
	next := 0
	if s.active == 0 {
		next = 1
	}
	f, err := os.CreateTemp(s.dir, ".session-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, slot(s.dir, next)); err != nil {
		return err
	}
	s.active, s.sequence, s.last = next, r.Sequence, payload
	return nil
}
