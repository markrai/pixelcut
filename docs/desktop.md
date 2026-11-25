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

- Graphical time ruler, zoom, selection, drag and trim handles, and a continuous playhead.
- IPC-controlled source seeking and pause state; crop-aware preview and preview/export equivalence.
- Project-wide render plan, track compositing, independent audio and mixing.
- Controlled SDR export and further HDR qualification.
