package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const projectLimit = 16 << 20

var ErrProjectChanged = errors.New("project file changed on disk; use Save As to preserve both versions")

func revision(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func ValidRevision(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

func readProjectBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > projectLimit {
		return nil, errors.New("invalid or oversized project file")
	}
	b, err := io.ReadAll(io.LimitReader(f, projectLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > projectLimit {
		return nil, errors.New("project file exceeds size limit")
	}
	return b, nil
}

func LoadRevision(path string) (Project, string, error) {
	var p Project
	b, err := readProjectBytes(path)
	if err != nil {
		return p, "", err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil {
		return p, "", err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return p, "", errors.New("unexpected trailing project content")
	}
	if err = p.Validate(); err != nil {
		return p, "", err
	}
	return p, revision(b), nil
}

func (p Project) Fingerprint() string {
	b, err := json.Marshal(p.Clone())
	if err != nil {
		return ""
	}
	return revision(b)
}

func (p Project) encodeSave(path string) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".pixelcut.json") {
		return nil, errors.New("project filename must end in .pixelcut.json")
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if len(b) > projectLimit {
		return nil, errors.New("project file exceeds size limit")
	}
	return b, nil
}

func stageProject(path string, b []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".pixelcut-project-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func (p Project) SaveNew(path string) (string, error) {
	b, err := p.encodeSave(path)
	if err != nil {
		return "", err
	}
	name, err := stageProject(path, b)
	if err != nil {
		return "", err
	}
	defer os.Remove(name)
	if err = os.Link(name, path); err != nil {
		return "", err
	}
	return revision(b), nil
}

func checkRevision(path, expected string) error {
	if !ValidRevision(expected) {
		return ErrProjectChanged
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProjectChanged, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: destination is not a regular file", ErrProjectChanged)
	}
	b, err := readProjectBytes(path)
	if err != nil {
		return err
	}
	if revision(b) != expected {
		return ErrProjectChanged
	}
	return nil
}

func (p Project) SaveExisting(path, expected string) (string, error) {
	b, err := p.encodeSave(path)
	if err != nil {
		return "", err
	}
	if err = checkRevision(path, expected); err != nil {
		return "", err
	}
	name, err := stageProject(path, b)
	if err != nil {
		return "", err
	}
	defer os.Remove(name)
	if err = checkRevision(path, expected); err != nil {
		return "", err
	}
	if err = os.Rename(name, path); err != nil {
		return "", err
	}
	return revision(b), nil
}
