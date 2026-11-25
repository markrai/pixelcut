package session

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxRecent = 12
const recentLimit = 2 << 20

func validRecent(path string) bool {
	return filepath.IsAbs(path) && len(path) <= 131072 && strings.HasSuffix(strings.ToLower(path), ".pixelcut.json")
}

func TouchRecent(paths []string, path string) []string {
	if !validRecent(path) {
		return append([]string(nil), paths...)
	}
	path = filepath.Clean(path)
	out := []string{path}
	for _, p := range paths {
		if !validRecent(p) {
			continue
		}
		p = filepath.Clean(p)
		if p == path {
			continue
		}
		a, ae := os.Stat(p)
		b, be := os.Stat(path)
		if ae == nil && be == nil && os.SameFile(a, b) {
			continue
		}
		out = append(out, p)
		if len(out) >= MaxRecent {
			break
		}
	}
	return out
}

func LoadRecent(dir string) ([]string, error) {
	f, err := os.Open(filepath.Join(dir, "recent.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, recentLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > recentLimit {
		return nil, errors.New("recent projects file too large")
	}
	var paths []string
	if err = decodeStrict(b, &paths); err != nil {
		return nil, err
	}
	if len(paths) > MaxRecent {
		return nil, errors.New("too many recent projects")
	}
	for _, p := range paths {
		if !validRecent(p) {
			return nil, errors.New("invalid recent project path")
		}
	}
	return paths, nil
}

func SaveRecent(dir string, paths []string) error {
	if len(paths) > MaxRecent {
		return errors.New("too many recent projects")
	}
	for _, p := range paths {
		if !validRecent(p) {
			return errors.New("invalid recent project path")
		}
	}
	b, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	if len(b) > recentLimit {
		return errors.New("recent projects file too large")
	}
	f, err := os.CreateTemp(dir, ".recent-*")
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
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "recent.json"))
}
