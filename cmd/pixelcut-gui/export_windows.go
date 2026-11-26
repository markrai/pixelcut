package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/project"
	"github.com/markrai/pixelcut/internal/projectexport"
	"github.com/markrai/pixelcut/internal/service"
)

const exportFPSBase = 9100

var exportRates = [...]int{24, 25, 30, 50, 60}

type projectExportJob struct {
	mu       sync.Mutex
	progress projectexport.Progress
	warnings []string
	levels   media.AudioLevels
	cancel   context.CancelFunc
	stopping bool
	lastPoll time.Time
}

func (j *projectExportJob) observe(p projectexport.Progress) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(p.Warnings) > 0 {
		j.warnings = append([]string(nil), p.Warnings...)
	}
	if p.AudioLevels.Frames > 0 {
		j.levels = p.AudioLevels
	}
	j.progress = p
}

func (j *projectExportJob) snapshot() (projectexport.Progress, []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.progress, append([]string(nil), j.warnings...)
}

func (j *projectExportJob) audioLevels() media.AudioLevels {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.levels
}

func exportStatus(p projectexport.Progress, warnings int) string {
	var status string
	switch p.Stage {
	case projectexport.StageVideo, projectexport.StageAudio:
		kind, unit := "Rendering HDR video", "frames"
		if p.Stage == projectexport.StageAudio {
			kind, unit = "Mixing and muxing audio", "samples"
		}
		if p.Total > 0 {
			status = fmt.Sprintf("%s: %d%% (%d/%d %s submitted)", kind, min(100, p.Completed*100/p.Total), p.Completed, p.Total, unit)
		} else {
			status = kind + "..."
		}
	case projectexport.StageVideoWait:
		status = "Finalizing HEVC video..."
	case projectexport.StageAudioWait:
		status = "Finalizing AAC and MP4..."
	case projectexport.StageVerify:
		status = "Verifying output color and timing..."
	case projectexport.StagePublish:
		status = "Publishing verified MP4..."
	case projectexport.StageDone:
		status = "Export published; finishing cleanup..."
	default:
		status = "Checking project and source media..."
	}
	if p.Stage == projectexport.StageAudio && p.AudioLevels.ClipCount() > 0 {
		status += fmt.Sprintf(" | CLIP: %d channel samples", p.AudioLevels.ClipCount())
	}
	if warnings > 0 {
		status += fmt.Sprintf(" | %d preflight warning(s)", warnings)
	}
	return status
}

func exportMenuItems(p project.Project, clipAllowed bool) []menuItem {
	items := []menuItem{item(idExport, "Export selected clip...\tCtrl+E", clipAllowed), separator(), item(exportFPSBase, "Whole project / HLG HDR / MP4", false)}
	for i, fps := range exportRates {
		label := fmt.Sprintf("Export project at %d fps...", fps)
		if fps == 30 {
			label = "Export project at 30 fps..."
		}
		items = append(items, item(exportFPSBase+1+i, label, p.EndTime() > 0))
	}
	return items
}

func exportRate(id int) (int, bool) {
	i := id - exportFPSBase - 1
	if i < 0 || i >= len(exportRates) {
		return 0, false
	}
	return exportRates[i], true
}

func (e *editor) showExportMenu() error {
	if !e.canOpenMenu() {
		return nil
	}
	p := e.model.Project().Clone()
	allowed := e.availability().Check(commandRequirement(idExport)) == nil
	items := exportMenuItems(p, allowed)
	var r rect
	if ok, _, _ := user.NewProc("GetWindowRect").Call(e.controls[idExportMenu], uintptr(unsafe.Pointer(&r))); ok == 0 {
		return fmt.Errorf("could not locate Export button")
	}
	id := e.popup(items, point{r.Left, r.Bottom})
	if id == idExport {
		return e.command(idExport, 0)
	}
	fps, ok := exportRate(id)
	if !ok {
		return nil
	}
	return e.exportProject(p, fps)
}

func (e *editor) exportProject(p project.Project, fps int) error {
	if e.busy || e.model.Project().Fingerprint() != p.Fingerprint() {
		return fmt.Errorf("project changed; reopen the export menu")
	}
	plan, err := projectexport.Build(p, media.ExportColorHLG, fps)
	if err != nil {
		return err
	}
	path, err := pick(true, "MP4 video (*.mp4)\x00*.mp4\x00", "mp4", "Export whole project (new HDR MP4)")
	if err != nil || path == "" {
		return err
	}
	path, err = projectexport.ValidateDestination(path)
	if err != nil {
		return err
	}
	audio := "No enabled audio"
	if plan.Contract.Audio {
		audio = "AAC stereo, 48 kHz, 192 kbps (all enabled audio)"
	}
	msg := fmt.Sprintf("Export the entire current project, including unsaved edits?\n\n%d x %d | %d fps | %.3f seconds (CFR)\nHEVC Main 10 / BT.2020 HLG / limited range\n%s\n\nDestination: %s\nExisting files will not be overwritten.", plan.Contract.Canvas.Width, plan.Contract.Canvas.Height, fps, float64(plan.VideoFrames)/float64(fps), audio, path)
	if len(plan.Warnings) > 0 {
		msg += "\n\n" + strings.Join(plan.Warnings, "\n")
	}
	r, _, _ := user.NewProc("MessageBoxW").Call(mainWindow, uintptr(unsafe.Pointer(u(msg))), uintptr(unsafe.Pointer(u("Export whole project"))), 0x134)
	if r != 6 {
		return nil
	}
	if e.busy || e.model.Project().Fingerprint() != p.Fingerprint() {
		return fmt.Errorf("project changed; reopen the export menu")
	}
	e.stopPreview()
	e.retireMediaWorkers()
	if e.stillWorker != nil {
		e.stillWorker.Close()
		e.retiredWorkers = append(e.retiredWorkers, e.stillWorker.Done())
		e.stillWorker = nil
	}
	retired := append([]<-chan struct{}(nil), e.retiredWorkers...)
	for _, done := range []<-chan struct{}{e.retiringPreview, e.retiringComposite} {
		if done != nil {
			retired = append(retired, done)
		}
	}
	engine := e.engine
	ctx, cancel := context.WithCancel(context.Background())
	job := &projectExportJob{cancel: cancel}
	hwnd := mainWindow
	err = e.jobs.Start(e.projectEpoch, func(parent context.Context) result {
		stop := context.AfterFunc(parent, cancel)
		defer stop()
		defer cancel()
		wait, stopWait := context.WithTimeout(ctx, 10*time.Second)
		err := service.WaitAll(wait, retired)
		stopWait()
		if err == nil {
			err = projectexport.RenderProjectObserved(ctx, engine, p, media.ExportColorHLG, fps, path, job.observe)
		}
		if err != nil && ctx.Err() != nil {
			return result{message: "Project export canceled. No output published."}
		}
		_, warnings := job.snapshot()
		message := "Project export complete (" + engine.DecodeBackendName() + " decode): " + path
		if len(warnings) != 0 {
			message += fmt.Sprintf(" (%d preflight warning(s); use plan-export for details)", len(warnings))
		}
		if levels := job.audioLevels(); err == nil && levels.Frames > 0 {
			message = fmt.Sprintf("PCM peak L %s / R %s dBFS; clipped channel samples: %d | ", media.PeakLabel(levels.Peak[0]), media.PeakLabel(levels.Peak[1]), levels.ClipCount()) + message
		}
		return result{err: err, message: message}
	}, func() { post.Call(hwnd, wmDone, 0, 0) })
	if err != nil {
		cancel()
		e.restartMediaWorkers()
		return err
	}
	e.exportJob = job
	e.jobFingerprint = p.Fingerprint()
	e.busy = true
	e.setControlsEnabled(false)
	h := e.controls[idExportMenu]
	change(h, "Cancel export")
	user.NewProc("EnableWindow").Call(h, 1)
	user.NewProc("SetFocus").Call(h)
	user.NewProc("InvalidateRect").Call(h, 0, 1)
	e.tell("Stopping preview workers before export...")
	return nil
}

func (e *editor) cancelProjectExport() {
	j := e.exportJob
	if j == nil || j.stopping {
		return
	}
	j.stopping = true
	j.cancel()
	h := e.controls[idExportMenu]
	change(h, "Canceling...")
	user.NewProc("EnableWindow").Call(h, 0)
	user.NewProc("InvalidateRect").Call(h, 0, 1)
	e.tell("Cancel requested. Waiting for export cleanup...")
}

func (e *editor) pollExportProgress() {
	j := e.exportJob
	if j == nil || time.Since(j.lastPoll) < 150*time.Millisecond {
		return
	}
	j.lastPoll = time.Now()
	if j.stopping {
		e.tell("Cancel requested. Waiting for export cleanup...")
		return
	}
	p, warnings := j.snapshot()
	if p.Stage != "" {
		e.tell(exportStatus(p, len(warnings)))
	}
}

func (e *editor) finishExportUI() {
	e.exportJob.cancel()
	e.exportJob = nil
	change(e.controls[idExportMenu], "Export...")
	user.NewProc("InvalidateRect").Call(e.controls[idExportMenu], 0, 1)
	e.restartMediaWorkers()
}
