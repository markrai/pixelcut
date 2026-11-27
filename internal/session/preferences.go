package session

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Preferences struct {
	Version        int     `json:"version"`
	Snap           bool    `json:"snap"`
	Ripple         bool    `json:"ripple"`
	FrameSnap      bool    `json:"frame_snap"`
	Speed          float64 `json:"preview_speed"`
	HDR            bool    `json:"hdr_preview"`
	RestorePreview bool    `json:"restore_preview"`
	ViewSeconds    int     `json:"view_checkpoint_seconds"`
	FFmpeg         string  `json:"ffmpeg,omitempty"`
	FFprobe        string  `json:"ffprobe,omitempty"`
	MPV            string  `json:"mpv,omitempty"`
	DecodeBackend  string  `json:"decode_backend,omitempty"`
	EncodeBackend  string  `json:"encode_backend,omitempty"`
}

func DefaultPreferences() Preferences {
	return Preferences{Version: 1, Snap: true, Speed: 1, RestorePreview: true, ViewSeconds: 1}
}

func (p Preferences) Validate() error {
	if p.Version != 1 {
		return errors.New("unsupported preferences version")
	}
	switch p.Speed {
	case .25, .5, 1, 1.5, 2:
	default:
		return errors.New("invalid preview speed")
	}
	switch p.ViewSeconds {
	case 1, 5, 15:
	default:
		return errors.New("invalid view checkpoint interval")
	}
	switch p.DecodeBackend {
	case "", "cpu", "auto", "cuda":
	default:
		return errors.New("invalid video decode backend")
	}
	switch p.EncodeBackend {
	case "", "cpu", "nvenc":
	default:
		return errors.New("invalid video encode backend")
	}
	for _, path := range []string{p.FFmpeg, p.FFprobe, p.MPV} {
		if path != "" && (!filepath.IsAbs(path) || strings.ContainsRune(path, 0) || len(path) > 131072) {
			return errors.New("invalid media tool path")
		}
	}
	return nil
}

const preferencesLimit = 1 << 20

func LoadPreferences(dir string) (Preferences, error) {
	defaults := DefaultPreferences()
	f, err := os.Open(filepath.Join(dir, "preferences.json"))
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, preferencesLimit+1))
	if err != nil {
		return defaults, err
	}
	if len(b) > preferencesLimit {
		return defaults, errors.New("preferences file too large")
	}
	var p Preferences
	if err = decodeStrict(b, &p); err != nil {
		return defaults, err
	}
	if err = p.Validate(); err != nil {
		return defaults, err
	}
	return p, nil
}

func SavePreferences(dir string, p Preferences) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > preferencesLimit {
		return errors.New("preferences file too large")
	}
	f, err := os.CreateTemp(dir, ".preferences-*")
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
	return os.Rename(name, filepath.Join(dir, "preferences.json"))
}
