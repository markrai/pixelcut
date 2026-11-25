# Desktop prototype (Phase 1A / 1B)

## Project contract

The project format is versioned JSON with asset references, tracks, clips, integer microsecond timestamps and optional display-oriented crop rectangles. Source in/out intervals are half-open. Timeline placement is independent of source time. Clips may not overlap within a track; different tracks can overlap in the model. Splitting preserves the source interval and does not mutate the input project. Save As writes a new project file and refuses overwrite; absolute paths are stored. Source files are not copied or changed.

The model accepts multiple tracks, but the initial desktop shell supports one V1 track with an empty A1 placeholder. Audio remains attached to its video. Import currently accepts the validated HLG video profile; unsupported audio or auxiliary stream combinations may still fail at export. Missing or moved source files are reported when accessed.

## Native rendering feasibility

The first Windows prototype uses Win32 controls and an external mpv process embedded into a child HWND using mpv's documented --wid option. It keeps gpu-next, software decode and the same SDR target configuration as the qualified CLI preview. This avoids introducing Qt bindings, a C++ build toolchain or a browser color conversion path before the embedding experiment passes. Qt/libmpv remain possible production choices; the process boundary is not a commitment to the final GUI.

Preview plays a selected source interval rather than a composited timeline. Committed crop is visualized using the shared FFmpeg-compatible filter plan; other transforms and timeline composition are not yet supported. Export dispatches a selected clip to the existing audiovisual remux or HDR render path. No project-wide exporter is provided yet. Do not equate native embedded SDR preview with native HDR output qualification.

## Windows acceptance procedure

1. Build and launch the desktop executable from a command prompt. Import an original Pixel HLG recording. The media bin should show its displayed portrait dimensions.
2. Add it to V1. Select the clip and play it; verify video renders inside the preview pane, audio works, and appearance agrees with the existing CLI preview. Stop and repeat playback, including several different source intervals. Verify closing the application terminates its player.
3. Set source in to 2 seconds, source out to 7 seconds, and crop to 40,40,1000,1840 for a displayed 1080 x 1920 source. Apply, export to a new MP4 and inspect it with the CLI. Expect five seconds, 1000 x 1840, 10-bit HLG and no rotation requirement.
4. Split the clip at a timeline position strictly inside it; verify that the resulting intervals meet without overlap. Undo and redo. Save as a new project, reopen and verify the edits survive.
5. Verify invalid intervals, overlaps, odd crop coordinates, missing media and existing output files fail without altering sources. Verify the desktop stays responsive during import/export.
6. Compare export playback against the original and existing CLI export in VLC and mpv. Record Windows Media Player discrepancies separately. Confirm embedded preview does not degrade appearance.

The project model can be tested automatically; cross-compilation and a Windows build are not evidence of successful native playback. Physical Windows display, embedded player lifecycle, layout/DPI, and user interaction are explicit acceptance gates.

## Next slices

- Crop-aware preview and preview/export equivalence.
- Project-wide render plan, track compositing, independent audio and mixing.
- Controlled SDR export and further HDR qualification.

## Graphical timeline and desktop styling

The Windows shell uses a dark native GDI palette, owner-drawn buttons, Segoe UI, a resizable window and a double-buffered timeline. The video lane is editable; the audio lane is a representation of linked source audio, not an independent track. Clip body drag changes placement, edge drag changes source in/out and left edge placement. Changes are committed only on mouse release after project validation, with invalid overlap rejected. The model tests cover clamping, movement and overlap rejection. Undo restores committed edits. A ruler click sets the playhead and split time; Play clip starts at that point when inside the selected clip. During playback the playhead follows mpv's observed time-pos through a per-session Windows named pipe. Ruler scrubbing seeks within or across clips; sequential cross-clip playback is not yet implemented. Zoom and horizontal scroll affect view state only.

Acceptance: resize the window, import a clip, drag it, trim both edges, split at a clicked ruler position, undo, zoom and scroll, and confirm source preview and export remain consistent. Verify at the display's actual DPI. The media encoding path is unchanged.

## Keyboard and cursor acceptance

- Space toggles source playback; Esc cancels an active drag or stops playback. The embedded player uses a temporary input configuration for Space/Esc, separate from user mpv settings. Check both timeline and embedded preview focus, natural end-of-file, repeated play/stop, and focus after playback ends.
- Ctrl+N/O/S/I/E invoke new/open/save-as/import/export. Ctrl+Z, Ctrl+Y and Ctrl+Shift+Z invoke undo/redo. Ctrl+K or S splits the selected clip at the timeline playhead; Delete removes it. F fits the timeline; plus/minus zoom. Left/Right moves the marker by 100 ms; Home/End moves to the project boundaries. These commands seek the preview but are not frame stepping.
- Tab and Shift+Tab navigate the editor controls; Enter applies numeric properties while focused in a property field. Native text selection, deletion and undo are preserved. F1 opens the shortcut reference. No global keyboard hook is installed; mpv focus supports pause keys but not all editor commands.
- Hover over a video clip body, left edge, right edge, ruler and empty timeline. Confirm four-way move, horizontal resize, crosshair and normal arrow cursors respectively. Invalid overlapping drags show a prohibited cursor. Linked audio shows a selection hand only where audio exists. Clip hit testing is shared by cursor selection and drag initiation.
- Check that holding Space or Delete does not repeatedly trigger actions, Escape cancels an uncommitted drag, and shortcut commands do not execute while an import/export is active. Verify the source and HDR export path are unchanged.

## Playback-synchronized playhead

The editor polls mpv's JSON IPC time-pos, seeking, pause and eof-reached properties through a unique Windows named pipe for each preview. The UI samples the latest reported position every 33 ms; no wall-clock estimate is used. Source timestamps are clamped to the clip's source interval and translated into project time by adding the timeline start and subtracting the source in point. The accent playhead crosses both lanes, the timestamp display shows milliseconds, and the timeline scrolls to keep the playhead visible. The eof-reached property moves the marker to the clip end. Manual pause retains the decoded frame; Space resumes. Ruler clicks and scrubs send absolute exact seeks mapped to source coordinates. No automatic next-clip transition or frame-accurate seeking guarantee is provided yet.

Acceptance: play an untrimmed clip, watch the playhead advance, stop with Space, verify the marker freezes, play again, and confirm playback resumes at the marker. Repeat with a trimmed clip placed at a nonzero timeline start. Zoom until the playhead reaches the edge and verify the view follows it. Let playback reach the end and test Space/Esc with focus on the embedded player. Windows CI covers bidirectional timestamp conversion and seek-request coalescing; native pipe connection, timing, scrolling and focus require physical Windows testing.

## Bidirectional preview seeking

The preview stays loaded when paused. The same IPC worker issues seek and pause commands and polls player state; mpv's own on-screen controller and default key bindings are disabled to avoid seeking outside the editor's clip boundaries; UI events publish only the latest desired seek rather than blocking on the pipe. Ruler scrubbing temporarily pauses and resumes the prior playback state on release. Click a clip or use arrow/Home/End shortcuts to seek. When crossing clip boundaries manually the player is replaced; a gap clears the preview. Clip end is fixed to the source out-point, independent of the initial seek location. Test forward/backward seeks while playing, paused and after EOF, rapid scrubbing, trimmed/moved clips, cross-clip jumps, gaps and repeated Space presses. Exact seek may be slower for long-GOP files; crop is visualized through the separate visual crop workflow. Physical Windows IPC responsiveness and displayed-frame accuracy remain qualification items.

## Phase 2B.2a: Visual cropping

The embedded mpv process loads a bundled Lua script from a temporary file. The script draws a shaded full-source crop rectangle, thirds guides and eight handles using mpv's own OSD coordinates. The crop session pauses playback, keeps a draft separate from the project and uses even, display-relative pixels. Shift held at drag start locks the initial ratio approximately, subject to pixel rounding and source bounds. Apply commits one undoable project change, Cancel leaves the project unchanged, and Reset removes the crop. A committed rectangle is translated to the encoded orientation using mpv's rotation property and applied through the shared geometry filter plan; no preview transcode or export pipeline change is needed. The source recording is never modified. Position/scale on a project canvas, rotation and flip are deferred until their preview and export contracts are implemented.

Windows acceptance: import test.mp4, seek to a recognizable frame, enter Visual crop, drag each handle, drag the interior, try Shift-drag, apply and verify the resulting composition. Compare an asymmetric crop on portrait and landscape footage against CLI export, not merely dimensions. Test cancel, reset, undo/redo, reopening a project with crop, resizing the editor and playback after apply. Confirm the source rectangle remains within bounds and even-aligned. Test the visual crop with mpv 0.42 on TITAN; actual overlay interaction and orientation equivalence remain unqualified until this passes.

## Phase 2B.2b: Position and scale

Optional per-clip transform fields extend version 1 projects without changing existing clip behavior. Canvas dimensions are explicit even pixel sizes. Uniform scale is relative to the fitted cropped source, and X/Y offsets are relative to the centered scaled image. The planner calculates even-aligned visible intersections and rejects fully off-canvas images and excessive allocations. Crop precedes scale, clipping and black padding, with square-pixel output. The same filter plan is used by FFmpeg export and mpv lavfi preview; the preview explicitly normalizes cardinal source rotation and clears rotation on the filtered output. Only limited-range HLG is supported for transforms until full-range padding is qualified. A numeric inspector previews valid changes after a short debounce; Apply is atomic and undoable, Cancel restores the committed transform, Reset draft restores identity. Mouse-driven transform handles, arbitrary rotation and project-wide compositing remain future work.

Acceptance: select a portrait clip; open Transform; choose 1080 x 1920, scale 60, X 80, Y -100. Verify positioning and black borders in preview and exported MP4. Repeat at 150% to verify clipping, on a landscape source and on a cropped clip, then test reset/cancel, save/open, split, undo/redo and source preservation. Compare actual visible content, not just dimensions and signaling. The mpv filter-chain orientation and color behavior still require native Windows qualification.

## Phase 2B.2c: Rotation and flips

The Transform inspector includes Rotate left/right (90 degrees per click) and independent horizontal/vertical flip toggles. Quarter-turn controls also swap the canvas width and height; scale and offsets retain their existing values in output canvas coordinates. Source display rotation is normalized first, followed by source-relative crop, user rotation, horizontal and vertical flips, scale, clipping and padding. The shared planner rejects non-cardinal angles and preserves the existing limited-range HLG transform contract. Orientation fields are optional in version 1 projects and default to zero rotation and no flips. Apply, Cancel, Reset, split and undo/redo use the same transform transaction. No arbitrary-angle rotation or mouse-driven rotation handles yet.

Acceptance: rotate a portrait clip once in each direction, verify the canvas swaps, repeat four quarter turns, toggle each flip on and off, combine crop with rotation and flip using an asymmetric scene, and compare the actual preview against the exported MP4. Test reset, cancel, save/open and split. Native mpv visual equivalence remains a separate gate.

The root run.bat builds and launches the Windows editor from the repository directory. Build failures stop launch and leave a readable error; existing media dependencies are not installed automatically.

## Phase 2B.2d: Transformation equivalence

Qualification is split into independent gates:

1. Automated FFmpeg regression: an asymmetric 10-bit HLG luminance fixture is encoded, remuxed with cardinal display rotations, and processed by the shared geometry planner. Compare unencoded frames from explicit preview-style normalization and FFmpeg autorotation byte-for-byte, verify expected output dimensions, compare actual decoded HEVC export luma with the unencoded reference (mean absolute error at most 8 ten-bit code values), and verify black padding is limited-range code 64. The fixture is synthetic and grayscale; these are not measurements of chroma fidelity or native mpv rendering.
2. Interactive Windows test: `qualify.bat` runs the automated FFmpeg test and `TestNativePreviewEquivalence` with `PIXELCUT_QUALIFY_PREVIEW=1`. The latter uses installed mpv's gpu-next output, the production crop script and the same SDR target settings as the editor. For each of four source orientations, it compares native screenshots of crop-only, rotated/mirrored crop, and scaled/padded crop against screenshots of their corresponding HEVC exports. It requires equal dimensions, global RGB mean absolute error at most 8 on an 8-bit code-value scale, and worst of sixteen spatial blocks at most 18. These are initial engineering acceptance thresholds, not perceptual standards. The script waits for preview readiness and screenshot capture, times out on failure, and uses only temporary synthetic media. It does not require real personal footage. The normal Windows CI job compiles the native test but skips the GPU/display portion without the opt-in environment variable.
3. Manual integration check: on TITAN, use real Pixel 10 footage in the embedded editor, apply asymmetric crop and transforms, pause and scrub, compare against the exported MP4 in the same mpv SDR target, then check cancel/reset/undo and reopening. Confirm orientation, framing, color appearance and responsiveness. The native screenshot gate uses standalone gpu-next windows rather than the embedded child HWND and is not a measurement of monitor HDR output. Neither screenshots nor these grayscale fixtures certify full chroma fidelity, arbitrary HDR formats, or Windows Media Player behavior.

A failure blocks claiming preview/export equivalence. Record exact mpv and FFmpeg versions and failing case; do not relax thresholds merely to make a test pass. The local FFmpeg reference experiment independently checked all eight orientation/filter combinations and limited-range padding. Full native Windows acceptance remains pending.

### Crop-only qualification follow-up

The first TITAN native run passed ten of twelve cases; crop-only at source rotations 0 and 180 failed. Crop-only previously used mpv's video-output crop property, unlike the passing combined transform cases. Committed crops now use the same lavfi filter planner as the export and combined transforms. Clearing a crop or entering crop edit mode removes the filter chain. The limited-range canvas restriction remains specific to transforms, not crop-only. The original screenshot tolerances are unchanged. The exact prior failure metric was not captured in the reported summary; the updated test logs the composition and mpv output geometry for diagnosis. Rerun all twelve native cases before closing the gate.
