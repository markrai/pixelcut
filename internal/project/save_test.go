package project

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveExistingRevision(t *testing.T) {
	p := fixture(t)
	path := filepath.Join(t.TempDir(), "test.pixelcut.json")
	r, err := p.SaveNew(path)
	if err != nil || !ValidRevision(r) {
		t.Fatalf("new: %q %v", r, err)
	}
	q, loaded, err := LoadRevision(path)
	if err != nil || loaded != r || !reflect.DeepEqual(p, q) {
		t.Fatal("load revision mismatch")
	}
	q.Tracks[0].Clips[0].Start += 1000000
	next, err := q.SaveExisting(path, r)
	if err != nil || next == r {
		t.Fatalf("save: %q %v", next, err)
	}
	got, actual, err := LoadRevision(path)
	if err != nil || actual != next || !reflect.DeepEqual(got, q) {
		t.Fatal("updated project not restored")
	}
	if _, err = p.SaveExisting(path, r); !errors.Is(err, ErrProjectChanged) {
		t.Fatal("stale revision accepted")
	}
	if _, err = p.SaveExisting(path, ""); !errors.Is(err, ErrProjectChanged) {
		t.Fatal("unknown revision accepted")
	}
	if _, err = p.SaveNew(path); err == nil {
		t.Fatal("Save As overwrote existing file")
	}
	got, actual, err = LoadRevision(path)
	if err != nil || actual != next || !reflect.DeepEqual(got, q) {
		t.Fatal("failed save changed project")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = p.SaveExisting(path, next); !errors.Is(err, ErrProjectChanged) {
		t.Fatal("missing file recreated")
	}
}

func TestSaveExistingInvalidProject(t *testing.T) {
	p := fixture(t)
	path := filepath.Join(t.TempDir(), "test.pixelcut.json")
	r, err := p.SaveNew(path)
	if err != nil {
		t.Fatal(err)
	}
	q := p.Clone()
	q.Tracks[0].Clips[0].Out = 0
	if _, err = q.SaveExisting(path, r); err == nil {
		t.Fatal("invalid save accepted")
	}
	got, actual, err := LoadRevision(path)
	if err != nil || actual != r || !reflect.DeepEqual(got, p) {
		t.Fatal("invalid save changed file")
	}
	if p.Fingerprint() != got.Fingerprint() || q.Fingerprint() == p.Fingerprint() {
		t.Fatal("fingerprint mismatch")
	}
}
