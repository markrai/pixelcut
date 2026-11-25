package main

import (
	"path/filepath"
	"testing"

	editorstate "github.com/markrai/pixelcut/internal/editor"
	"github.com/markrai/pixelcut/internal/project"
)

func TestProjectTitle(t *testing.T) {
	for _, tc := range []struct {
		path          string
		dirty, failed bool
		want          string
	}{
		{"", false, false, "PixelCut - Untitled"},
		{"", true, false, "PixelCut - Untitled *"},
		{"C:\\Videos\\Trip.pixelcut.json", false, false, "PixelCut - Trip"},
		{"C:\\Videos\\Trip.PIXELCUT.JSON", true, true, "PixelCut - Trip * [recovery unavailable]"},
	} {
		if got := projectTitle(tc.path, tc.dirty, tc.failed); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

func TestRestoreSaveBaseline(t *testing.T) {
	p := project.New()
	path := filepath.Join(t.TempDir(), "baseline.pixelcut.json")
	rev, err := p.SaveNew(path)
	if err != nil {
		t.Fatal(err)
	}
	q := p.Clone()
	q.Assets = append(q.Assets, project.Asset{ID: "a", Path: filepath.Join(t.TempDir(), "media.mp4"), Duration: 1000000, Width: 1080, Height: 1920})
	ed := editor{model: editorstate.New(p), projectPath: path, dirty: true}
	ed.restoreSaveBaseline()
	if ed.dirty || ed.projectRevision != rev || ed.savedFingerprint != p.Fingerprint() {
		t.Fatal("identical saved project did not restore baseline")
	}
	ed = editor{model: editorstate.New(q), projectPath: path, dirty: true}
	ed.restoreSaveBaseline()
	if !ed.dirty || ed.projectRevision != "" || ed.savedFingerprint != "" {
		t.Fatal("legacy dirty recovery adopted an unverified baseline")
	}
	ed = editor{model: editorstate.New(q), projectPath: path, dirty: true, projectRevision: rev, savedFingerprint: p.Fingerprint()}
	ed.restoreSaveBaseline()
	if !ed.dirty || ed.projectRevision != rev || ed.savedFingerprint != p.Fingerprint() {
		t.Fatal("valid dirty recovery lost baseline")
	}
	newRev, err := q.SaveExisting(path, rev)
	if err != nil {
		t.Fatal(err)
	}
	ed.restoreSaveBaseline()
	if ed.dirty || ed.projectRevision != newRev {
		t.Fatal("save before checkpoint not reconciled")
	}
	ed = editor{model: editorstate.New(p), projectPath: path, projectRevision: rev, savedFingerprint: p.Fingerprint()}
	ed.restoreSaveBaseline()
	if !ed.dirty || ed.projectRevision != rev || ed.savedFingerprint != "" {
		t.Fatal("external update treated as safe overwrite")
	}
}
