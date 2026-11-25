# PixelCut

A small, source-aware video editor for people who want to cut and crop footage without fighting color management.

PixelCut includes a source-aware command-line media engine and an initial Windows desktop prototype. The project model stores source references and edits without modifying recordings.

## Quick start

Requirements:

- Go 1.23 or newer
- FFmpeg and FFprobe on PATH (FFmpeg must include libx265)
- Optional: mpv with gpu-next support for preview (Windows installer locations are searched if PATH lookup fails)

Build:

```sh
go build -o pixelcut.exe ./cmd/pixelcut
```

On Linux or macOS, use `-o pixelcut` instead.

### Inspect an original recording

```powershell
.\pixelcut.exe inspect "C:\Videos\original.mp4"
```

Inspect reports codec, pixel format, color primaries, transfer, matrix, range, geometry, stream metadata and timing. It does not modify the recording.

### Preview

```powershell
.\pixelcut.exe preview "C:\Videos\original.mp4"
.\pixelcut.exe preview -hdr "C:\Videos\original.mp4"
```

The default requests an SDR Rec.709 preview. The HDR option requests native output on a compatible display and platform. Neither option guarantees a match to the phone's rendering. HDR preview requires physical display qualification.

### Remux without re-encoding

```powershell
.\pixelcut.exe remux original.mp4 copy.mp4
```

Copies audiovisual streams into a new MP4 without re-encoding. Cardinal display rotation is preserved. Recognized Pixel MetaHandle/mett timed metadata is excluded with a warning; unknown auxiliary streams are rejected. Global and stream metadata and chapters are stripped, including the observed GPS tags. This is not a byte-for-byte copy of the original file.

### Trim or crop

```powershell
.\pixelcut.exe trim -start 2.5 -duration 8 original.mp4 trimmed.mp4
.\pixelcut.exe crop -x 120 -y 0 -width 3600 -height 2160 original.mp4 cropped.mp4
```

The crop example assumes landscape 3840 x 2160 source footage. Crop coordinates refer to the displayed orientation. For a portrait recording displayed at 1080 x 1920, combine trim and crop in one encode: 

```powershell
.\pixelcut.exe crop -start 2 -duration 5 -x 40 -y 40 -width 1000 -height 1840 original.mp4 cropped-trimmed.mp4
```

 Crop coordinates and dimensions must be even for 4:2:0 chroma alignment. Start and duration are expressed in seconds. Trim re-encodes video and audio; crop re-encodes video but copies audio.

The renderer accepts explicitly signaled, square-pixel, 10-bit 4:2:0 BT.2020 HLG with supported chroma location and optional pure cardinal display rotation. Re-encoding normalizes rotation into the pixels. Unknown side data and auxiliary streams are rejected; recognized Pixel MetaHandle/mett streams are excluded with a warning. Multiple audio tracks are not yet supported by the renderer. Export writes to a temporary file, checks the output against the color contract, then publishes without overwriting an existing file. The destination filesystem must support hard links (e.g. NTFS).

Re-encoded output uses HEVC Main 10 through libx265 and CRF 16. It is not mathematically lossless. Global and stream metadata and chapters are intentionally omitted from exports. This is not a guarantee of comprehensive privacy sanitization. Per-frame dynamic HDR metadata is not yet qualified. Do not delete source recordings.

## Windows desktop prototype

Double-click `run.bat` in the repository root, or execute it from Command Prompt. It builds the current desktop executable and launches it only after a successful build. Go must be on PATH; FFmpeg, FFprobe and mpv are required for media operations. The launcher does not download dependencies or pull repository changes.

Alternatively, build and launch manually:

```cmd
go build -o pixelcut-gui.exe ./cmd/pixelcut-gui
pixelcut-gui.exe
```

Import a supported HLG video, select it in the media bin and click **Add to V1**. Select the clip, enter source in/out and timeline start in seconds, optionally enter a display-relative crop as `x,y,width,height`, and click **Apply edits**. **Split** takes a timeline time strictly inside the selected clip. Play clip uses mpv embedded in a native Windows window and previews the selected source interval in SDR; committed crops are previewed through the shared geometry filter plan. **Export clip** exports the selected clip, not the entire project. New projects have a V1 video track and an empty A1 audio track; audio follows the selected video.

**Visual crop** pauses the selected clip and shows the full source with a shaded crop region, thirds guides and eight handles. Drag the center to move, the edges/corners to resize, or hold Shift when starting a drag to approximately preserve the aspect ratio with even pixel dimensions. Apply crop commits one undoable edit and shows the cropped result; Cancel crop restores the previous crop. Reset crop removes it. Enter/Esc inside the video apply/cancel. Coordinates match the displayed orientation, including portrait sources. Visual crop needs mpv 0.42 or a compatible version supporting video-crop and user-data. Export uses the existing validated FFmpeg crop path.

**Transform** opens a live position/scale inspector. Set even canvas dimensions, scale (100% fits the cropped source within the canvas), and even X/Y offsets relative to center. Positive X moves right; positive Y moves down. Rotate left/right turns the image by 90 degrees and swaps canvas dimensions. Flip H/V mirrors the rotated image along its displayed horizontal/vertical axes. Crop is applied before these operations, then scale, position, clipping and padding. Areas outside the canvas are clipped and uncovered areas are black. Apply saves one undoable edit, Cancel restores the original, and Reset draft restores the source or crop dimensions at 100% with zero offsets. Limited-range HLG is required for canvas transforms. Transformed export re-encodes once using the same geometry filter plan; unmodified clips remain eligible for remux.

**Save As** creates a new versioned `.pixelcut.json` file without overwriting an existing project. **Open** restores edits and absolute source paths. Keep source files in place. Undo and redo cover committed project edits. Import, export, and preview use the existing FFmpeg and mpv installations.

Timeline clips now display sampled source thumbnails, generated in the background and cached in memory. Thumbnails follow trim and zoom, respect source display orientation and use an approximate SDR tone map for readability; they are not a color reference or a preview of committed crop/transform effects. FFmpeg must include zscale and tonemap for thumbnails. Missing thumbnails leave the clip usable. The linked audio lane now displays real decoded peak waveforms. The worker samples the first audio stream as 8 kHz stereo PCM, keeps the larger absolute channel peak, and caches bounded peak summaries per source. Waveforms follow trim, split, zoom and scrolling; no audio is modified. Missing or timed-out audio analysis leaves the linked lane usable.

The desktop shell includes a dark theme, resizable layout, a graphical V1 lane and linked A1 audio representation. Click the ruler or clip to position the timeline marker; Play clip starts from that position when it falls inside the selected clip. Drag the body of a clip to move it, or drag its left/right edge to trim. Invalid overlapping edits are rejected. Use +/- or Ctrl+mouse wheel to zoom, mouse wheel to scroll, and Fit to show the project. The mustard playhead follows mpv's actual source playback timestamp, mapped onto the timeline, and scrolls the timeline when it reaches the visible edge. Pausing retains the decoded frame and playhead position. Click or drag the timeline ruler to seek, including while paused; releasing a scrub resumes playback if it was playing before. Seeking across clips loads the target clip, while gaps clear the preview. Playback does not yet advance automatically across clips. Clip bodies show a move cursor, trim edges a horizontal resize cursor, the ruler a crosshair, and invalid drags a prohibited cursor. Press Space to play/stop, Esc to stop or cancel a drag, Ctrl+Z/Y to undo/redo, Ctrl+K or S to split, Delete to remove a clip, Left/Right to move the marker by 0.1 seconds, Home/End to jump, and F to fit. F1 or the Shortcuts button shows the full list. Editing shortcuts are scoped to the editor; text fields retain their usual editing keys. When the embedded mpv window has keyboard focus, Space and Esc are bound to stop; click the timeline for other editor shortcuts. No independent audio editing, composite playback or project-wide export yet. The embedded rendering path needs physical Windows acceptance testing. See [desktop prototype qualification](docs/desktop.md).

## Drag-and-drop import

Drop one or more supported video files from Windows Explorer onto the media bin or timeline, or select multiple files through Import video. Files are inspected in the background, duplicates select existing entries, and unsupported files are reported without discarding valid imports. Drag a media-bin item into V1 to place it at a chosen timestamp with snapping and a placement ghost. Explorer drops onto the V1 lane import and place successful assets contiguously; other drops import only. Overlapping placements are rejected without losing successfully imported assets. Each successful batch is undoable and automatically recovered.

## Precision playback and editing

Use comma/period to step through decoded frame timestamps, Ctrl+Left/Right to jump edit boundaries, I/O to set source In/Out, B to toggle source-frame trim snapping, and brackets to adjust preview speed. Frame indices are generated on demand from actual presentation timestamps rather than average FPS.

## Context menus

Right-click a clip, empty timeline space or media-bin asset for relevant editing commands. Clip menus respect multi-selection; empty timeline menus offer paste and gap removal at the clicked position. The Menu key or Shift+F10 also opens the focused panel's menu.

## Timeline clipboard

Ctrl+C copies selected clips, Ctrl+X cuts without rippling, Ctrl+V pastes at the playhead, and Ctrl+D duplicates after the selection or at the end if necessary. Relative spacing, source trims and transformations are preserved. Paste and duplicate are undoable and reject overlapping clips.

## Ripple editing

Shift+Delete ripple-deletes selected clips. G removes the gap at the playhead. R toggles ripple edge trimming, which anchors the edited clip's start and shifts later clips by the duration change. Regular Delete still leaves gaps.

## Multi-selection

Ctrl-click clips to toggle selection, Shift-click to select a chronological range, Ctrl+A to select all timeline clips, or drag from empty lane space to select with a marquee. Move selected clips together with snapping, or delete them in one undoable operation. Clip properties and transformations remain single-clip operations.

## Timeline snapping and media-bin removal

Snapping aligns clip moves and trim edges to the playhead, timeline origin and neighboring clip boundaries. Use the Snap button or N to toggle it; hold Shift while dragging to bypass it. Select an asset in the media bin and press Delete (or Remove asset) to remove its project reference. If clips use the asset, confirmation is required to remove those clips too. Removal is undoable and never deletes source files.

## Automatic session recovery

PixelCut checkpoints imported asset references, committed edits, timeline selection, playhead, zoom and scroll position in the current user's local application data directory. It restores the latest valid session on launch, including after a forced close; playback resumes paused when the selected clip contains the restored marker. Recovery is independent of manually saved project files. New and Open intentionally replace the current session after any unsaved-change confirmation. Unapplied edit drafts and undo history are not persisted.

## Transformation qualification

Run `qualify.bat` from the repository root on an interactive Windows desktop with FFmpeg, FFprobe and mpv 0.42 available. It runs the FFmpeg pixel-level regression and an opt-in native mpv screenshot comparison. This is separate from `run.bat` and normal CI. The native test opens temporary player windows, uses synthetic media, and compares the actual crop/transform script output against the exported clip using the same SDR preview target. See [qualification details](docs/desktop.md#phase-2b2d-transformation-equivalence) for acceptance boundaries.

## Tests

```sh
go test -v ./...
go vet ./...
```

Integration tests generate small synthetic HLG-tagged videos, including a rotated fixture with a simulated Pixel metadata track. They exercise remux, trim, crop and combined edits, check output signaling and geometry, and compare decoded audiovisual frame hashes for the rotated remux. FFmpeg and FFprobe must be available for integration tests to run. This fixture is not a substitute for camera footage or calibrated HDR display testing.

## Roadmap

1. Qualify the integrated engine on actual Pixel footage on Windows. Direct FFmpeg experiments passed provisionally; Windows Media Player showed an abrupt contrast change in edited clips that was not observed in VLC.
2. Establish an SDR export transform and evaluate it against the original phone and reference playback.
3. Qualify the initial non-destructive project model and native Windows preview prototype; build a graphical timeline and independent audio lanes.
4. Add native desktop preview, transitions and overlays only after the baseline is stable.
5. Add hardware-accelerated decode and encode with the same fidelity tests as the software path.

See [media architecture](docs/architecture.md) and [qualification procedure](docs/qualification.md).
