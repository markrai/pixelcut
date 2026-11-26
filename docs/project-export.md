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
- If enabled audio contributes: 48 kHz stereo AAC, equal-level offline mixing implemented in 2D.4. Otherwise no audio stream.
- Source metadata must be verified; only recognized Pixel MetaHandle/mett timed data may be excluded with a warning. Unknown auxiliary streams and multiple audio streams fail closed.
- Full-range and center-sited HLG receive explicit pixel normalization in 2D.3. Other unsupported metadata still fails closed.
- No SDR preview pixels enter the HDR pipeline. 2D.3 blends partial opacity in scene-linear BT.2020 RGB then re-encodes HLG. Fully opaque pixels retain a direct 10-bit path; physical equivalence remains a qualification gate.

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


## 2D.3: scene-linear opacity and normalization

The same experimental video-only command now supports partial opacity. No additional flags or schema changes. HLG input must retain explicit 10-bit 4:2:0 BT.2020 nonconstant-luminance signaling, square pixels and supported cardinal rotation. Range (full or limited) and chroma location (left or center) are read from fresh source metadata and used as zscale input parameters. Conversion produces limited-range HLG 4:4:4 before geometry; the final encoder performs left-sited 4:2:0 downsampling. Existing project model restrictions on full-range canvas transforms still apply; do not bypass them by changing metadata.

For a mixed pixel, normalized code values are Y'=(Y-64)/876, Cb=(Cb-512)/896, Cr=(Cr-512)/896. Recover R' and B' using BT.2020 Kr=0.2627 and Kb=0.0593, and G' using Kg=1-Kr-Kb. Clamp nonlinear RGB to [0,1] before inverse HLG OETF: E=E'^2/3 for E'<=0.5, otherwise E=(exp((E'-c)/a)+b)/12 (a=0.17883277, b=1-4a, c=0.55991073). For each covering layer from top to bottom, accumulate its scene-linear RGB multiplied by its opacity and remaining uncovered fraction. Remaining coverage is black; stop at fully opaque coverage. Apply the forward OETF once, convert to BT.2020 nonconstant-luminance Y'CbCr and round to legal 10-bit limited-range codes. No repeated quantization between layers. An entirely opaque visible pixel bypasses the conversion. Example: 50% HLG white over black yields luma code 828, not the code-space average 502. This is a scene-referred blend, not HLG display OOTF; out-of-gamut/excursion samples are clipped only on the mixed path. Chroma/geometry resampling still takes place in signal space. The scalar math is correctness-first and may be slow at 4K. No claim of real-time export, perceptual equivalence, or measured HDR display fidelity.

The previous 2D.2 opacity rejection and normalization limitations are superseded by this section. 2D.4 adds the audio-inclusive export-project command below. Source index/decoder limits and safe publication stay unchanged.


## 2D.4: audio-inclusive export

```text
pixelcut export-project -color hlg -fps 30 project.pixelcut.json output.mp4
```

Uses the same HDR renderer as export-video, then stream-copies the encoded HEVC into the final MP4 while encoding the offline mix as 48 kHz stereo AAC at 192 kb/s. The video-only command retains its explicit discard-audio option. No audible contributions means no audio stream, not a redundant silent AAC track. Audio respects shared evaluator policies (hide does not mute linked audio, mute does not hide video, solo applies globally, detach suppresses the linked contribution). At most eight simultaneous sampled audio contributions. No gain/fades or automatic normalization; unity-gain summation saturates at signed 16-bit limits, matching the elementary preview policy. Clipping of loud overlaps is possible; advanced gain control is Phase 2E.

Sample intervals use the global 48 kHz grid and half-open clip intervals. Source index at output sample n is n + floor((source in - timeline start)*48000/1,000,000), with at most one sample of source-time quantization. Source decoding begins at source start, preserving timestamps with copyts/start_at_zero and compensating discontinuities through aresample; finite sample trimming and padding follow. Video/audio cuts do not restart unrelated audio streams. Missing leading/tail samples are padded with silence; decoder failure fails export. PCM blocks are streamed directly into the AAC muxer and never accumulated into a whole-project PCM file. Exact input PCM count is ceil(project duration * 48000/1,000,000) plus silence until video frame count * (48000/FPS). AAC priming and trailing packet padding are handled by the MP4 presentation interval, not by claiming exact raw decoded packet length. Movie timescale 240000 avoids millisecond rounding. FFprobe must report zero starts and exact audio duration_ts at 1/48000, alongside the existing video contract. This verifies container timing, not physical speaker/display latency or lossy AAC sample equality.

Both compressed MP4 stages exist temporarily in the output directory during audio mux. Plan/source checks, cancellation and no-overwrite publication still apply. Long source trims remain slower because decode starts at the beginning. Synthetic regression tests cover off-grid cut sample intervals, equal-gain saturation, silent padding, nonzero audio start, sample-rate conversion, packet hash preservation on mux, exact MP4 presentation sample count and cleanup. Physical Windows and real Pixel audio/HLG/VFR qualification remain open.
