# Phase 2D: Whole-project HDR export

## 2D.1: preflight only

Use the existing executable:

```text
pixelcut plan-export -color hlg -fps 30 project.pixelcut.json
```

This command reads the saved project, builds an immutable timeline plan and re-inspects contributing media. It emits JSON to stdout; it does not encode or publish a video. Both flags are required. The report contains local source paths, clip identifiers and file identities; keep it local or redact before sharing. A valid plan is not proof that a renderer or encoder supports all its contents.

## Output contract

- Container MP4; HEVC hvc1 Main 10-compatible 10-bit 4:2:0, BT.2020 primaries, ARIB STD-B67 HLG transfer, BT.2020 nonconstant luminance matrix, limited range, left chroma siting and square pixels.
- Project composition canvas; source cardinal rotation normalized into pixels.
- Explicit integer CFR 24, 25, 30, 50 or 60 fps. No implicit VFR preservation claim for project export. Original selected-clip export retains its separate policy.
- If enabled audio contributes: 48 kHz stereo AAC, initial equal-level mixing policy to be implemented offline in 2D.4. Otherwise no audio stream.
- Source metadata must be verified; only recognized Pixel MetaHandle/mett timed data may be excluded with a warning. Unknown auxiliary streams and multiple audio streams fail closed.
- Visible full-range or center-sited HLG requires real color/chroma conversion, not a tag rewrite. This slice rejects these sources until conversion is qualified.
- No SDR preview pixels enter the HDR pipeline. A later multi-layer renderer must convert to a defined linear-light BT.2020 working domain for alpha blending, then encode the HLG signal; test numeric/reference values before claiming fidelity. Opaque geometry may have its own direct 10-bit path if its equivalence is qualified.

## Timing

All project intervals are half-open microseconds. Frame index n samples at floor(n * 1,000,000 / fps) ticks, and a span [a,b) owns [ceil(a * fps / 1,000,000), ceil(b * fps / 1,000,000)). Audio uses the same ceiling rule at 48,000 Hz. No per-frame array is allocated. Output frame count is ceil(project duration * fps / 1,000,000), so output may extend less than one frame beyond the logical end. If audio is present, it is padded with silence to the exact CFR output duration. Visible spans with no frame sample are reported, never silently promised a rendered frame. Leading/interior gaps and audio-only tails are represented as empty layer spans (future renderer emits black); opacity zero suppresses video but not linked audio. Mute, hide, solo and detach come from the shared evaluator. Lock is an editing restriction, not an export exclusion.

## Preconditions and limits

At least one enabled nonzero-opacity video layer, positive timeline, maximum 24 hours, 10,000 spans and 100,000 total stored video/audio contributions. Source metadata must match the project: display dimensions, duration, audio presence and declared range. Preflight snapshots absolute path, size and modification time and rechecks before returning. This is not a content hash or a guarantee against a same-size same-mtime modification; future execution must verify sources during render and immediately before safe publication. No persisted format change.

## Remaining slices

2D.2 opaque 10-bit video render and verified publication; 2D.3 linear-light multilayer blending and normalization; 2D.4 offline audio and mux; 2D.5 CLI/GUI orchestration, progress/cancel and destination safeguards; 2D.6 reference tests, actual Pixel footage and resource review. The 2D.2 experimental video-only command below is now available; it is not yet the complete audio-capable export.


## 2D.2: experimental opaque video-only export

```text
pixelcut export-video -color hlg -fps 30 [-discard-audio] project.pixelcut.json output.mp4
```

The encoder operates on the original 10-bit signal, not SDR preview pixels. Supports opaque overlapping layers (bottom-to-top), source rotation, crop, transform, picture-in-picture, hidden tracks, solo, and black leading/interior/trailing gaps. Rejects sampled partial opacity, excess layers, unsupported color normalization, and enabled audio unless explicitly discarded. Silent projects need no discard flag. A 24-hour logical duration is still the planner limit; execution additionally caps output at 4096 pixels per side and 4096x2160 total pixels, intermediate/source images at 16,777,216 pixels, and four visible layers. Source frames are selected by strict presentation-order index; no approximate seek is used. Decode begins at the source start for each span and emits only needed source ordinals. This favors deterministic timing over long-trim performance; real VFR, rotation, chroma and HDR fidelity remain unqualified beyond synthetic tests.

The encoder receives one frame per CFR sample. Raw working frames use 10-bit limited-range BT.2020 HLG Y'CbCr 4:4:4; explicit chroma up/downsampling surrounds geometry and opaque copying. No partial-opacity blending or SDR tone mapping. MP4 is staged in the output directory and inspected for color, geometry, HEVC Main 10 profile, exact frame count, CFR duration and no audio. Sources are rechecked before publication; existing files are never overwritten. Hard-link support is required, as for selected-clip export. Ctrl+C cancels CLI rendering; child FFmpeg processes and temporary output are cleaned up on normal error/cancel. A hard process termination may leave a temporary file. No physical HDR display or perceptual equivalence claim follows from metadata verification.
