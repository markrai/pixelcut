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

Import a supported HLG video, select it in the media bin and click **Add to V1**. Select the clip, enter source in/out and timeline start in seconds, optionally enter a display-relative crop as `x,y,width,height`, and click **Apply edits**. **Split** takes a timeline time strictly inside the selected clip. Play clip uses mpv embedded in a native Windows window and previews the selected source interval in SDR; committed crops are previewed through the shared geometry filter plan. **Export clip** exports the selected clip, not the entire project. New projects have a V1 video track and an empty A1 audio track; source audio follows the selected video unless detached.

**Visual crop** pauses the selected clip and shows the full source with a shaded crop region, thirds guides and eight handles. Drag the center to move, the edges/corners to resize, or hold Shift when starting a drag to approximately preserve the aspect ratio with even pixel dimensions. Apply crop commits one undoable edit and shows the cropped result; Cancel crop restores the previous crop. Reset crop removes it. Enter/Esc inside the video apply/cancel. Coordinates match the displayed orientation, including portrait sources. Visual crop needs mpv 0.42 or a compatible version supporting video-crop and user-data. Export uses the existing validated FFmpeg crop path.

**Transform** opens a live position/scale inspector. Set even canvas dimensions, scale (100% fits the cropped source within the canvas), and even X/Y offsets relative to center. Positive X moves right; positive Y moves down. Rotate left/right turns the image by 90 degrees and swaps canvas dimensions. Flip H/V mirrors the rotated image along its displayed horizontal/vertical axes. Crop is applied before these operations, then scale, position, clipping and padding. Areas outside the canvas are clipped and uncovered areas are black. Apply saves one undoable edit, Cancel restores the original, and Reset draft restores the source or crop dimensions at 100% with zero offsets. Limited-range HLG is required for canvas transforms. Transformed export re-encodes once using the same geometry filter plan; unmodified clips remain eligible for remux.

**Save As** creates a new versioned `.pixelcut.json` file without overwriting an existing project. **Open** restores edits and absolute source paths. Keep source files in place. Undo and redo cover committed project edits. Import, export, and preview use the existing FFmpeg and mpv installations.

Timeline clips now display sampled source thumbnails, generated in the background and cached in memory and on disk. Thumbnails follow trim and zoom, respect source display orientation and use an approximate SDR tone map for readability; they are not a color reference or a preview of committed crop/transform effects. FFmpeg must include zscale and tonemap for thumbnails. Missing thumbnails leave the clip usable. The linked audio lane now displays real decoded peak waveforms. The worker samples the first audio stream as 8 kHz stereo PCM, keeps the larger absolute channel peak, and caches bounded peak summaries per source. Waveforms follow trim, split, zoom and scrolling; no audio is modified. Missing or timed-out audio analysis leaves the linked lane usable.

The desktop shell includes a dark theme, resizable layout, stacked video lanes, linked waveforms and independent audio lanes. Click the ruler or clip to position the timeline marker; Play clip starts from that position when it falls inside the selected clip. Drag the body of a clip to move it, or drag its left/right edge to trim. Invalid overlapping edits are rejected. Use +/- or Ctrl+mouse wheel to zoom, mouse wheel to scroll, and Fit to show the project. The mustard playhead follows mpv's actual source playback timestamp, mapped onto the timeline, and scrolls the timeline when it reaches the visible edge. Pausing retains the decoded frame and playhead position. Click or drag the timeline ruler to seek, including while paused; releasing a scrub resumes playback if it was playing before. Seeking across clips loads the target clip, while gaps clear the preview. Playback does not yet advance automatically across clips. Clip bodies show a move cursor, trim edges a horizontal resize cursor, the ruler a crosshair, and invalid drags a prohibited cursor. Press Space to play/stop, Esc to stop or cancel a drag, Ctrl+Z/Y to undo/redo, Ctrl+K or S to split, Delete to remove a clip, Left/Right to move the marker by 0.1 seconds, Home/End to jump, and F to fit. F1 or the Shortcuts button shows the full list. Editing shortcuts are scoped to the editor; text fields retain their usual editing keys. When the embedded mpv window has keyboard focus, Space and Esc are bound to stop; click the timeline for other editor shortcuts. Independent audio audition and editing are supported; no composite playback or project-wide export yet. The embedded rendering path needs physical Windows acceptance testing. See [desktop prototype qualification](docs/desktop.md).

## Drag-and-drop import

Drop one or more supported video files from Windows Explorer onto the media bin or timeline, or select multiple files through Import video. Files are inspected in the background, duplicates select existing entries, and unsupported files are reported without discarding valid imports. Drag a media-bin item into V1 to place it at a chosen timestamp with snapping and a placement ghost. Explorer drops onto the V1 lane import and place successful assets contiguously; other drops import only. Overlapping placements are rejected without losing successfully imported assets. Each successful batch is undoable and automatically recovered.

## Stacked timeline tracks

All project tracks share a single ruler and playhead. Video tracks are stacked with the most recently added video track at the top; each has its attached source-audio waveform row. Independent audio tracks appear below the video tracks. Click a track header or clip to make that track active. Drag selected clip bodies horizontally and vertically onto a compatible track; snapping uses the destination track's boundaries. Overlapping or cross-kind drops are rejected, and a successful transfer is one undoable edit. Trim handles remain on the original track. Drag media-bin assets or Explorer files onto a specific main lane to place them there. Shift+mouse wheel scrolls the track stack; ordinary wheel scrolling and Ctrl+wheel zoom retain their existing behavior. Selection and marquee operations remain scoped to the active track. Preview and clip export are still isolated; simultaneous composition is not yet supported.

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

## Project lifecycle

Ctrl+S saves the current project (or prompts for a filename when untitled); Ctrl+Shift+S creates a new project file without overwriting an existing one. Recent reopens previously saved or opened projects, and Ctrl+W closes the current project. The title shows the project name and an unsaved asterisk. Save checks for external file changes before replacement. New, Open and Close prompt to save, discard or cancel unsaved edits.

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

### Persistent media cache

The Cache toolbar menu shows disk usage, lets you choose 256 MiB, 1 GiB (default) or 4 GiB, and clears disposable thumbnails, waveforms, frame indexes and import metadata. Cached media is reused across projects and restarts when the source path, size, modification time and relevant processing parameters match. Cache entries are local and checksummed; clearing them does not remove recordings, projects or recovery data. Visible media regenerates as needed.

### Missing media and relinking

Offline assets retain every timeline edit. The Relink toolbar menu and asset/clip context menus locate moved recordings, search a folder for compatible filename matches, or explicitly replace a source. Candidates are freshly inspected and confirmed before updating references; original media is never modified. Relinking is undoable and integrates with session recovery.

### Application preferences

Open Settings (Ctrl+,) to configure editing toggles, preview speed, SDR or experimental HDR output, startup preview restoration, recovery view checkpoint timing and tool executable paths. Settings persist independently of projects. Space/Play starts from the marker, skips leading/interior gaps to the next clip, and does not restart playback at the final timeline end.

### Multitrack foundation (2C.1)

The Track menu switches the active editing track, adds video/audio tracks, transfers clips between compatible tracks and removes empty additional tracks. Independent audio clips can reference an imported video's audio stream. Each track uses the same timeline timebase; editing and preview currently operate on one active track, and video preview/export are isolated to a selected video clip. Stacked lanes and independent audio audition are implemented; simultaneous audio mixing and compositing follow in later phases. New projects use format version 5; versions 1 through 4 remain readable.

## Development and architectural stabilization

Feature work is paused for the 2C.R refactoring milestone. See [Architecture](ARCHITECTURE.md), [Roadmap](ROADMAP.md), and the [reproducible baseline](docs/refactor-baseline.md) for ownership boundaries, phase acceptance gates, test commands and measurements. Automated CI success is distinct from physical Windows and HDR qualification.


### Independent audio preview (2C.3)

Select an audio track to audition it using the existing playback controls. Right-click a video clip to detach source audio into an independent audio track, or reattach a uniquely aligned matching audio clip. The detached video previews and exports without its source audio. Independent audio and video preview remain isolated; simultaneous composition and mixing are future phases.


### Track controls (2C.4)

Right-click a lane header or use the Track menu to hide video, mute audio, solo or lock the active track. H, M, S and L in the header identify its settings. Hide suppresses video but not its attached audio; mute suppresses audio but not video; solo includes only soloed tracks (multiple solos are allowed). Hide and mute still take precedence. Locked tracks allow selection, copy and preview, but reject edits, transfers, placement, and source changes. Toggle controls again to restore normal operation. Settings persist and are undoable. Preview remains active-track only; selected-clip export honors mute and detached audio but ignores timeline hide/solo.


### Layer layout (2C.5)

Use the Track menu to select the shared composition canvas (automatic, portrait, landscape or square, including UHD presets). Right-click a video clip to set opacity (0/25/50/75/100%) or choose Fit canvas / a picture-in-picture corner. PiP uses 35% of aspect-fit scale and a 4% margin, preserving crop and orientation. The first placement preset freezes the automatic canvas size. Fine-tune scale and offset with Transform. Show layer layout at playhead displays colored bounds and opacity labels in bottom-to-top order, not decoded composite video. Hidden and zero-opacity layers do not appear; audio is unchanged. Press Play to return to isolated source preview. Partial opacity is not applied to isolated source audition, and selected-clip export rejects it rather than silently discarding it. Actual composite playback and project export remain future phases.


### Paused composite preview (2C.6.1–2C.6.2)

Track > Show composite frame at playhead renders real video pixels from the enabled video layers into a shared SDR preview. PiP, crop, scale, orientation, opacity and track visibility are applied. Changing layer opacity or placement opens this preview; the separate Show layer layout command still shows outlines. A seek within the paused composite view requests a replacement frame. The first frame may need a full timestamp index of each source. Preview is capped at 960 pixels on its longest side. SDR tone mapping is applied per source and opacity is blended in display RGB, not in HDR scene-linear space. Press Play to enter continuous composite preview, or choose Track > Play active track (source preview) for isolated audition. Continuous composite video playback is available in 2C.6.3; simultaneous audio and HDR composite rendering/export remain pending. No source file or existing export pipeline changes.


### Continuous composite preview (2C.6.3)

Track > Play composite (SDR, silent) starts project-wide video preview from the current marker, independent of selection. Play from a paused composite still resumes this mode; Stop/Space pauses on the most recently displayed frame. Track > Play active track (source preview) returns to the original mpv audition. The initial continuous renderer uses up to four persistent FFmpeg video decoders, a 640-pixel longest-side cap and a fixed 10 fps target at nominal 1x. When decoding is slow, the project marker advances only with displayed frames, so playback slows rather than accumulating a queue or pretending to be synchronized. Gaps render black and the final marker stops at project end. A seek during playback automatically resumes after retiring the old run; a paused seek requests a still. Source preview retains its own speed setting. Audio is deliberately absent from this mode until 2C.6.4; it is not HDR display or export. Very short clips can fall between the 100 ms samples, and exact VFR and boundary scheduling remain to be qualified in 2C.6.5–2C.6.6. The still view retains its 960-pixel limit.


### Composite audio and master clock (2C.6.4)

Track > Play composite (SDR, with audio) now includes enabled linked and independent audio, mixed to 48 kHz stereo PCM. Hide video does not mute linked audio; mute, solo and detach are respected by the composition evaluator. Audio is summed at unity gain with saturation; overlapping loud sources can clip, and gain/fades remain 2E. The default Windows waveOut device reports completed buffers, which advance project time; video follows that clock at the existing 10 fps target and drops stale frames. If decoding falls behind, video may temporarily blank, and audio starvation stalls the clock. The device is required even for a silent interval (silence supplies the clock). Pause captures the last completed buffer position (up to 50 ms granularity), not the exact speaker sample; seeking preserves the prior playing or paused state. Source audition and HDR export are unchanged. Real device latency, VFR and synchronization qualification remain pending.


### Integrated composite navigation (2C.6.5)

Click or drag the ruler during composite playback: audio is canceled immediately, the playhead tracks the gesture, and playback resumes automatically at the latest requested marker after release and retirement of the old workers. While paused, seeking stays paused; moving continuously coalesces still requests until 80 ms of inactivity. Space or Stop cancels a pending resume. Track selection does not switch out of the composite mode. Ctrl+arrow jumps to project-wide edit boundaries in composite mode, and frame stepping remains relative to the selected source clip. At project end the paused image is black. The video scheduler includes exact enabled layer start/end ticks in addition to its 100 ms grid, and outgoing pictures expire at the cut. Very short clips may still not appear because of the 50 ms confirmed audio clock, device latency, or slow decoding. No frame-accurate physical playback or seamless zero-latency seek claim is made.


### Composite qualification and diagnostics (2C.6.6)

Composite still and playback workers now reuse the existing bounded on-disk frame timestamp cache, keyed by source and FFprobe identity. Clear media cache cancels and drains cache-owning workers before clearing. After playing or pausing, choose Track > Composite playback diagnostics to inspect the last run: startup to first presented frame, maximum frame render duration, decoded/presented/dropped counts, clock-relative lag, audio queue starvation episodes, decoder open/retirement/peak counts and cancellation retirement time. Use Ctrl+C to copy the dialog. These counters are diagnostic, not physical A/V measurements; process counts are final after worker retirement. Composite output is explicitly tone-mapped SDR, not native HDR. The original HLG source audition and selected-clip HDR export remain separate. See [composite qualification](docs/composite-qualification.md) for the required evidence before accepting 2C.6.


### Whole-project export preflight (2D.1)

Run `pixelcut plan-export -color hlg -fps 30 project.pixelcut.json` to inspect the intended HDR export contract, timeline spans, frame/sample ranges and validated contributing sources. This is read-only JSON planning, **not** an export command. Both HLG and a supported integer CFR (24, 25, 30, 50 or 60) must be explicit. Full-range and center-sited HLG inputs are explicitly normalized for project video export in 2D.3; other unsupported inputs are rejected rather than relabeled. HDR video rendering and opacity blending are available in 2D.3; audio mux is available in 2D.4; Windows export UI is available in 2D.5. See [project export](docs/project-export.md).


### Experimental whole-project HLG video export (2D.2)

```text
pixelcut export-video -color hlg -fps 30 -discard-audio project.pixelcut.json output.mp4
```

Exports original 10-bit HLG frames, opaque layers, cuts, geometry, and black gaps to one HEVC Main 10 HLG MP4. If enabled audio is present, the command refuses unless `-discard-audio` is explicitly supplied; use export-project for audio-inclusive export. Partial opacity is now composited in scene-linear BT.2020 RGB (2D.3). At most four simultaneous layers and a 4096-pixel side / 4096x2160 total-pixel canvas are supported. Output must not exist. The source frame index selects the predecessor at each CFR tick; decoding currently starts at source beginning for each span, so long trims may be slow. Actual Pixel HDR/rotation/VFR qualification is still required. Existing selected-clip export is unchanged.


### Linear-light HDR composition (2D.3)

The experimental `export-video` command now supports translucent video layers with the same 0–100% project opacity controls. Input full/limited range and left/center chroma siting are converted to limited-range 10-bit HLG 4:4:4 before geometry. Blended pixels are decoded with the HLG inverse OETF to scene-linear BT.2020 RGB, combined by source-over coverage, then encoded once; fully opaque pixels retain the direct copy path. This is scene-referred compositing, not display-referred tone mapping. Geometry resampling remains in HLG signal space. Existing full-range transform restrictions in the project model remain unchanged. Real Pixel footage, chroma-edge fidelity and 4K blend performance remain qualification gates.


### Whole-project HLG export with audio (2D.4)

```text
pixelcut export-project -color hlg -fps 30 project.pixelcut.json output.mp4
```

Exports the entire timeline to HEVC Main 10 HLG with AAC 48 kHz stereo when enabled audio contributes. Linked and independent/detached audio honor the shared track policies. Equal-level mixing uses bounded signed 16-bit PCM blocks, saturating only after summing; gaps and the final CFR padding are silent before AAC encoding. Up to eight simultaneous audio contributions. Video is encoded once, then stream-copied into a second staged MP4 while audio is encoded. The final audio presentation interval is verified in exact 48 kHz sample units, with both streams starting at zero. No enabled contributions means no audio stream. The earlier export-video command remains video-only and requires explicit discard consent when audio is enabled. Ctrl+C cancels the command; no existing destination is overwritten. Native GUI export is available in 2D.5; physical synchronization qualification remains upcoming.


### Exporting a complete project in Windows (2D.5)

Choose **Export...** in the toolbar (or Ctrl+Shift+E), then select a project frame rate (24, 25, 30, 50 or 60 fps). Choose a new MP4 filename and confirm the resolution, duration, HLG contract and audio policy. The exporter snapshots the current timeline, including unsaved edits. No selected clip is required. All enabled audio is included; selected-clip export is still available via Ctrl+E or the menu. During whole-project export, the status shows the current stage and submitted frame/sample count; the Export button becomes Cancel export. Esc also requests cancellation. Editing remains disabled until cleanup completes. A close request cancels and leaves the editor open; close again once export has finished. No output is overwritten, and publication requires hard-link support. CLI users may add `-progress` to export-project. Real Windows/Pixel HDR qualification remains pending.


### HDR export qualification (2D.6)

Whole-project export now verifies every video packet's presentation timestamp and duration on an exact integer CFR grid before publishing. All supported frame rates use a 240000 Hz MP4 video track/movie timescale; AAC presentation remains exactly 48000 Hz. An export with no sampled visible video pixels fails rather than producing an all-black result. See [export qualification](docs/export-qualification.md) for synthetic evidence, resource limits and the outstanding Windows/Pixel acceptance procedure. Native HDR fidelity and performance are not yet qualified.


### Audio processing foundation (2E.1)

Project schema 6 adds optional per-clip `audio_gain_percent` (0–400, omitted means 100). Existing versions 1–5 load with unity gain and retain their stored version on load/save. Non-unity gain requires version 6; older applications cannot read that schema. Split/copy/paste preserve gain; detach moves it to the independent audio clip and reattach restores that clip's current gain. The composite preview and whole-project exporter share one fixed-point stereo PCM mixer, rounding and clipping only after all contributions are summed. Zero gain suppresses audio decoding; the source preview is an explicitly separate audition path and does not apply non-unity gain yet. Selected-clip export refuses nonzero non-unity gain rather than ignoring it; zero gain exports video only. No GUI gain controls until 2E.2. See [audio processing contract](docs/audio-processing.md).


### Clip audio controls (2E.2)

Select an attached video or independent audio clip and open **Audio: 100%** in the clip properties panel, or right-click the clip and choose **Clip audio / volume...**. The native menu offers gain presets from 0–400% plus 1- and 10-percentage-point adjustments, and a separate reversible mute. Muting retains the stored gain; editing volume while muted does not unmute. Each edit participates in undo/redo; locked tracks, detached video and silent assets cannot edit clip audio. Timeline labels indicate muted/non-unity clips; waveforms still depict source audio, not post-gain samples. Source audition applies linear gain through mpv's lavfi volume filter (muted video disables audio). Whole-project export and composite preview use the shared PCM mixer. Selected-clip export can omit muted/zero-gain audio; positive non-unity audible gain requires whole-project export. Schema 7 adds optional `audio_muted`; versions 1–6 remain supported without load/save migration. Native mpv/Windows acceptance remains pending.

### Audio fades (2E.3)

Open the selected clip's **Audio** menu, choose **Fade in** or **Fade out**, and select a duration (off, 0.1, 0.25, 0.5, 1, 2, 3, 5, 10, 30 or 60 seconds). Remove both fades resets the ramps without changing gain or mute. Fades are linear amplitude ramps, anchored to the current clip edges and capped to the clip duration. Overlapping ramps multiply. Trimming re-anchors the fades; splitting retains the original outer fades only, so splitting within a ramp may reshape it. Use **Play composite (SDR, with audio)** to hear the processed result; source audition applies gain/mute but not fades. Whole-project export applies the same sample-level ramps as composite playback. Audible fades require whole-project export rather than selected-clip export. Schema 8 adds optional audio_fade_in_us and audio_fade_out_us fields; versions 1–7 remain readable. See [audio processing contract](docs/audio-processing.md).

### Volume envelopes (2E.4)

Hold Alt and click the waveform area of an editable audio clip (linked or independent) to add a volume keyframe, or Alt-drag an existing point to change its time and attenuation. Alt+Shift+click a point removes it; Escape cancels a pending drag. The Audio menu can remove the entire envelope, including keyframes outside the current trim. The yellow graph represents the envelope multiplier from 0–100% of configured clip gain, not measured output amplitude; the existing clip gain can amplify up to 400%. The first insertion seeds unity anchors at both current clip edges. Keyframes are independently owned by each clip and stored at source times, so moving, trimming, extending and splitting preserve the automation curve on retained source material. Edge-relative fades retain their previously documented separate semantics. Play composite to hear automation; direct mpv source audition is explicitly unprocessed for envelopes. Audible envelopes require whole-project export. Schema 9 adds optional audio_envelope; versions 1–8 remain readable. See [audio processing contract](docs/audio-processing.md).

### Audio meters (2E.5)

The stereo mix meter below the preview shows the latest completed composite PCM buffer's left/right sample peaks in dBFS, run-wide peak-hold markers and a latched red clipping count. The count measures channel samples that required saturation after summation and rounding, not merely samples reaching full scale. A new composite playback run resets the statistics. The meter also shows cumulative submitted PCM peaks during whole-project export; successful exports report peak and clipping statistics, and CLI progress mode reports submitted PCM statistics. Direct mpv source audition is labeled unmetered. These are digital sample peaks, not LUFS, true peak, speaker output or post-AAC measurements. No automatic normalization or limiter is applied.

### Audio qualification (2E.6)

Composite playback and whole-project export now share the same timestamp normalization, 48 kHz resampling and exact source-sample trim policy in addition to the shared mixer. Real-media regression tests compare pre-AAC PCM and metering across delayed 44.1 kHz and 48 kHz stereo AAC sources, off-grid edits, automation, gaps and grid-aligned seeks. Canonical decoding begins at source origin rather than seeking before resampling; late seeks into long sources can take longer. Off-grid playback starts use a run-relative sample grid. Direct mpv audition remains a separate path. AAC output is lossy and is checked by presentation timing and signal correlation rather than bit equality. See [audio qualification](docs/audio-qualification.md) for automated and physical acceptance gates.
