package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/session"
)

const (
	prefSnap = 5001 + iota
	prefRipple
	prefFrames
	prefRestore
	prefSDR
	prefHDR
	prefSpeed0
	prefSpeed1
	prefSpeed2
	prefSpeed3
	prefSpeed4
	prefView1
	prefView5
	prefView15
	prefFFmpeg
	prefFFprobe
	prefMPV
	prefAutoTools
	prefReset
	prefGPUInfo
)

func engineForPreferences(p session.Preferences) media.Engine {
	e := media.Default()
	if p.FFmpeg != "" {
		e.FFmpeg = p.FFmpeg
	}
	if p.FFprobe != "" {
		e.FFprobe = p.FFprobe
	}
	if p.MPV != "" {
		e.MPV = p.MPV
	}
	return e
}

func (e *editor) loadPreferences() {
	p := session.DefaultPreferences()
	if e.recentDir != "" {
		var err error
		p, err = session.LoadPreferences(e.recentDir)
		if err != nil {
			e.preferencesWarning = "Preferences could not be read; using defaults: " + err.Error()
			fmt.Fprintln(os.Stderr, e.preferencesWarning)
		}
	}
	e.prefs = p
	e.snapDisabled = !p.Snap
	e.rippleTrim = p.Ripple
	e.frameSnap = p.FrameSnap
	e.playbackSpeed = p.Speed
	e.engine = engineForPreferences(p)
}

func (e *editor) syncPreferencesUI() {
	e.updateSnapButton()
	e.updateRippleButton()
	label := "Frames: off"
	if e.frameSnap {
		label = "Frames: on"
	}
	for id, text := range map[int]string{idFrameSnap: label, idSpeed: fmt.Sprintf("Speed: %gx", e.speed())} {
		h := e.controls[id]
		change(h, text)
		user.NewProc("InvalidateRect").Call(h, 0, 1)
	}
	e.syncPreviewLabel()
}

func (e *editor) rememberPreferences() string {
	e.prefs.Snap = !e.snapDisabled
	e.prefs.Ripple = e.rippleTrim
	e.prefs.FrameSnap = e.frameSnap
	e.prefs.Speed = e.speed()
	if e.recentDir == "" {
		return " Preferences storage unavailable."
	}
	if err := session.SavePreferences(e.recentDir, e.prefs); err != nil {
		return " Preferences not saved: " + err.Error()
	}
	e.preferencesWarning = ""
	return ""
}

func (e *editor) applyPreferences(p session.Preferences) error {
	if err := p.Validate(); err != nil {
		return err
	}
	engine := engineForPreferences(p)
	toolsChanged := engine != e.engine
	reopen := !toolsChanged && p.HDR != e.prefs.HDR && e.previewActive()
	playing := e.playing()
	if toolsChanged || reopen {
		e.stopPreview()
	}
	if toolsChanged {
		e.retireMediaWorkers()
		e.frameCache = nil
	}
	e.prefs = p
	e.engine = engine
	e.snapDisabled = !p.Snap
	e.rippleTrim = p.Ripple
	e.frameSnap = p.FrameSnap
	e.playbackSpeed = p.Speed
	if e.clock != nil {
		e.clock.setSpeed(p.Speed)
	}
	if toolsChanged {
		e.restartMediaWorkers()
	}
	e.syncPreferencesUI()
	note := e.rememberPreferences()
	if reopen {
		if c, ok := e.selected(); !ok || e.playhead < c.Start || e.playhead >= c.Start+(c.Out-c.In) {
			e.tell("Preferences applied." + note)
			return nil
		}
		if err := e.startPreview(!playing); err != nil {
			return fmt.Errorf("preferences applied%s; preview: %w", note, err)
		}
	}
	e.tell("Preferences applied." + note)
	return nil
}

func prefChecked(id int, label string, checked bool) menuItem {
	return menuItem{id: id, label: label, enabled: true, checked: checked}
}

func toolLabel(name, path string) string {
	if path == "" {
		return name + ": automatic"
	}
	return name + ": " + strings.ReplaceAll(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), "&", "&&")
}

func preferenceItems(p session.Preferences) []menuItem {
	items := []menuItem{
		prefChecked(prefSnap, "Magnetic snapping", p.Snap),
		prefChecked(prefRipple, "Ripple trimming", p.Ripple),
		prefChecked(prefFrames, "Source-frame trim snapping", p.FrameSnap),
		prefChecked(prefRestore, "Restore paused preview on startup", p.RestorePreview),
		separator(), prefChecked(prefSDR, "SDR preview (Rec.709)", !p.HDR),
		prefChecked(prefHDR, "HDR preview (experimental)", p.HDR), separator(),
	}
	for i, speed := range playbackSpeeds {
		items = append(items, prefChecked(prefSpeed0+i, fmt.Sprintf("Preview speed: %gx", speed), p.Speed == speed))
	}
	items = append(items, separator(),
		prefChecked(prefView1, "Recovery view checkpoint: 1 second", p.ViewSeconds == 1),
		prefChecked(prefView5, "Recovery view checkpoint: 5 seconds", p.ViewSeconds == 5),
		prefChecked(prefView15, "Recovery view checkpoint: 15 seconds", p.ViewSeconds == 15),
		separator(), item(prefFFmpeg, toolLabel("FFmpeg", p.FFmpeg)+"...", true),
		item(prefFFprobe, toolLabel("FFprobe", p.FFprobe)+"...", true),
		item(prefMPV, toolLabel("mpv", p.MPV)+"...", true),
		item(prefAutoTools, "Use automatic tool discovery", true),
		separator(), item(prefGPUInfo, "GPU capabilities / device diagnostics...", true),
		separator(), item(prefReset, "Reset preferences to defaults...", true))
	return items
}

func (e *editor) showPreferences() error {
	if !e.canOpenMenu() {
		return nil
	}
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idPreferences], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate Settings button")
	}
	id := e.popup(preferenceItems(e.prefs), point{r.Left, r.Bottom})
	if id == 0 {
		return nil
	}
	p := e.prefs
	switch id {
	case prefSnap:
		p.Snap = !p.Snap
	case prefRipple:
		p.Ripple = !p.Ripple
	case prefFrames:
		p.FrameSnap = !p.FrameSnap
	case prefRestore:
		p.RestorePreview = !p.RestorePreview
	case prefSDR:
		p.HDR = false
	case prefHDR:
		if !p.HDR {
			msg := "HDR preview requires a compatible HDR display and Windows HDR configuration. Color accuracy has not been qualified. Export settings are unchanged. Enable experimental HDR preview?"
			if r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(msg))), uintptr(unsafe.Pointer(u("HDR preview"))), 0x134); r != 6 {
				return nil
			}
		}
		p.HDR = true
	case prefSpeed0, prefSpeed1, prefSpeed2, prefSpeed3, prefSpeed4:
		p.Speed = playbackSpeeds[id-prefSpeed0]
	case prefView1:
		p.ViewSeconds = 1
	case prefView5:
		p.ViewSeconds = 5
	case prefView15:
		p.ViewSeconds = 15
	case prefFFmpeg, prefFFprobe, prefMPV:
		return e.pickTool(id)
	case prefAutoTools:
		p.FFmpeg = ""
		p.FFprobe = ""
		p.MPV = ""
	case prefGPUInfo:
		return e.showGPUInfo()
	case prefReset:
		if r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u("Reset application preferences? Projects, recovery snapshots and cache contents are unchanged."))), uintptr(unsafe.Pointer(u("PixelCut settings"))), 0x134); r != 6 {
			return nil
		}
		p = session.DefaultPreferences()
	default:
		return nil
	}
	return e.applyPreferences(p)
}

func verifyTool(ctx context.Context, path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("select a regular executable file")
	}
	arg := "-version"
	prefix := name + " version "
	if name == "mpv" {
		arg = "--version"
		prefix = "mpv "
	}
	job, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(job, path, arg).Output()
	if err != nil {
		return fmt.Errorf("%s version check: %w", name, err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), prefix) {
		return fmt.Errorf("selected executable did not identify as %s", name)
	}
	return nil
}

func (e *editor) pickTool(id int) error {
	name := "ffmpeg"
	if id == prefFFprobe {
		name = "ffprobe"
	} else if id == prefMPV {
		name = "mpv"
	}
	path, err := pick(false, "Executable files (*.exe)\x00*.exe\x00", "exe", "Select "+name+" executable")
	if err != nil || path == "" {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	p := e.prefs
	switch id {
	case prefFFmpeg:
		p.FFmpeg = path
	case prefFFprobe:
		p.FFprobe = path
	case prefMPV:
		p.MPV = path
	}
	if err = p.Validate(); err != nil {
		return err
	}
	e.tell("Validating " + name + " executable...")
	e.startJob(func(ctx context.Context) result {
		if err := verifyTool(ctx, path, name); err != nil {
			return result{err: err}
		}
		return result{preferences: &p}
	})
	return nil
}
