# Desktop prototype (Phase 1A / 1B)

## Project contract

The project format is versioned JSON with asset references, tracks, clips, integer microsecond timestamps and optional display-oriented crop rectangles. Source in/out intervals are half-open. Timeline placement is independent of source time. Clips may not overlap within a track; different tracks can overlap in the model. Splitting preserves the source interval and does not mutate the input project. Save As writes a new project file and refuses overwrite; absolute paths are stored. Source files are not copied or changed.

The model accepts multiple tracks, but the initial desktop shell supports one V1 track with an empty A1 placeholder. Audio remains attached to its video. Import currently accepts the validated HLG video profile; unsupported audio or auxiliary stream combinations may still fail at export. Missing or moved source files are reported when accessed.

## Native rendering feasibility

The first Windows prototype uses Win32 controls and an external mpv process embedded into a child HWND using mpv's documented --wid option. It keeps gpu-next, software decode and the same SDR target configuration as the qualified CLI preview. This avoids introducing Qt bindings, a C++ build toolchain or a browser color conversion path before the embedding experiment passes. Qt/libmpv remain possible production choices; the process boundary is not a commitment to the final GUI.

Preview plays a selected source interval rather than a composited timeline. Crop, other edits and timeline placement are not visualized in the preview. Export dispatches a selected clip to the existing audiovisual remux or HDR render path. No project-wide exporter is provided yet. Do not equate native embedded SDR preview with native HDR output qualification.

## Windows acceptance procedure

1. Build and launch the desktop executable from a command prompt. Import an original Pixel HLG recording. The media bin should show its displayed portrait dimensions.
2. Add it to V1. Select the clip and play it; verify video renders inside the preview pane, audio works, and appearance agrees with the existing CLI preview. Stop and repeat playback, including several different source intervals. Verify closing the application terminates its player.
3. Set source in to 2 seconds, source out to 7 seconds, and crop to 40,40,1000,1840 for a displayed 1080 x 1920 source. Apply, export to a new MP4 and inspect it with the CLI. Expect five seconds, 1000 x 1840, 10-bit HLG and no rotation requirement.
4. Split the clip at a timeline position strictly inside it; verify that the resulting intervals meet without overlap. Undo and redo. Save as a new project, reopen and verify the edits survive.
5. Verify invalid intervals, overlaps, odd crop coordinates, missing media and existing output files fail without altering sources. Verify the desktop stays responsive during import/export.
6. Compare export playback against the original and existing CLI export in VLC and mpv. Record Windows Media Player discrepancies separately. Confirm embedded preview does not degrade appearance.

The project model can be tested automatically; cross-compilation and a Windows build are not evidence of successful native playback. Physical Windows display, embedded player lifecycle, layout/DPI, and user interaction are explicit acceptance gates.

## Next slices

- Continuous playback-synchronized playhead and timeline scrubbing.
- IPC-controlled source seeking and pause state; crop-aware preview and preview/export equivalence.
- Project-wide render plan, track compositing, independent audio and mixing.
- Controlled SDR export and further HDR qualification.

## Graphical timeline and desktop styling

The Windows shell uses a dark native GDI palette, owner-drawn buttons, Segoe UI, a resizable window and a double-buffered timeline. The video lane is editable; the audio lane is a representation of linked source audio, not an independent track. Clip body drag changes placement, edge drag changes source in/out and left edge placement. Changes are committed only on mouse release after project validation, with invalid overlap rejected. The model tests cover clamping, movement and overlap rejection. Undo restores committed edits. A ruler click sets a static playhead and split time; Play clip starts at that point when inside the selected clip. No live playback synchronization or continuous scrubbing yet. Zoom and horizontal scroll affect view state only.

Acceptance: resize the window, import a clip, drag it, trim both edges, split at a clicked ruler position, undo, zoom and scroll, and confirm source preview and export remain consistent. Verify at the display's actual DPI. The media encoding path is unchanged.

## Keyboard and cursor acceptance

- Space toggles source playback; Esc cancels an active drag or stops playback. The embedded player uses a temporary input configuration for Space/Esc, separate from user mpv settings. Check both timeline and embedded preview focus, natural end-of-file, repeated play/stop, and focus after playback ends.
- Ctrl+N/O/S/I/E invoke new/open/save-as/import/export. Ctrl+Z, Ctrl+Y and Ctrl+Shift+Z invoke undo/redo. Ctrl+K or S splits the selected clip at the static timeline marker; Delete removes it. F fits the timeline; plus/minus zoom. Left/Right moves the marker by 100 ms; Home/End moves to the project boundaries. These are marker movements, not frame stepping or live scrubbing.
- Tab and Shift+Tab navigate the editor controls; Enter applies numeric properties while focused in a property field. Native text selection, deletion and undo are preserved. F1 opens the shortcut reference. No global keyboard hook is installed; mpv focus supports stop keys but not all editor commands.
- Hover over a video clip body, left edge, right edge, ruler and empty timeline. Confirm four-way move, horizontal resize, crosshair and normal arrow cursors respectively. Invalid overlapping drags show a prohibited cursor. Linked audio shows a selection hand only where audio exists. Clip hit testing is shared by cursor selection and drag initiation.
- Check that holding Space or Delete does not repeatedly trigger actions, Escape cancels an uncommitted drag, and shortcut commands do not execute while an import/export is active. Verify the source and HDR export path are unchanged.
