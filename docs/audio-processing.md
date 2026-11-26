# 2E audio processing contract

## Phase 2E.1

Status: implementation committed; candidate CI and physical audio acceptance pending. The 2D.6 baseline passed standard CI run 45843280827 and paired performance run 45843280833. Native Windows/Pixel qualification is still outstanding.

### Model and compatibility

- Schema 6 clip field: optional integer `audio_gain_percent`. Missing or null is 100 (unity). 0 is silent; 1–400 is linear amplitude gain, not a decibel value. 50 is approximately -6 dB, 200 approximately +6 dB.
- Versions 1–5 remain supported with their existing rules. Version 5 canvas/opacity fields remain valid. Loading or saving alone does not migrate old projects. Setting a non-unity gain upgrades to version 6; reset to unity omits the field without downgrading the project. New projects use version 6; existing edit operations that upgraded to the current schema continue doing so. Older PixelCut builds will reject version 6 rather than silently ignore gain.
- Gain requires a source with audio, on attached video or an independent audio clip. Detached video has no audio gain; detaching moves the gain to the independent clip, reattaching moves its current gain back. Split, trims, moves and clipboard operations preserve gain; cloned pointer fields do not alias.
- Gain mutation validates the whole project and honors track locks. Track mute and solo still suppress contributions. Video visibility and opacity do not control audio. Zero gain removes the contribution without shortening the project or suppressing video; if all contributions are suppressed the project exporter emits no AAC stream.
- 2E.1 exposes the model API, not a new editing control. A reversible clip mute distinct from gain, native audition behavior and UI are 2E.2.

### Shared DSP

For each channel of each output sample, accumulate integer signed PCM samples multiplied by their integer gain percentages into a signed 64-bit sum. Divide the final sum by 100, round to nearest with half ties away from zero, and saturate once to [-32768,32767]. No clipping or rounding before summation. Eight simultaneous contributions maximum, 2400 stereo frames maximum per block. One accumulator uses at most 38400 bytes of logical payload. There is no automatic loudness normalization, limiter, compressor or dithering. Source decoders already produce signed 16-bit PCM, so gain cannot recover information lost in decoding/downmixing.

Both composite preview and offline export call the same accumulator. The two timing adapters and source decode methods remain separate; equal input samples yield equal mixed PCM, but real source timing, resampling and AAC encoder delay/loss remain qualification concerns. Direct active-track mpv audition is not composite playback: zero gain suppresses it, positive non-unity gain is explicitly labeled as unchanged source gain. Selected-clip export rejects positive non-unity gain; zero gain produces silent video. Whole-project export applies gain.

### Verification

Tests cover schema 5 canvas/opacity compatibility, no-op unity edit, explicit zero persistence, invalid gains, locked tracks, silent assets, split/copy independence, detach/reattach transfer, selected-clip export policy, exact symmetric rounding, cancellation of large opposing contributions, saturation, order independence, contribution cap and shared preview/offline PCM equality across off-grid boundaries and zero gain. Existing unity-gain tests must remain unchanged. Physical audio and real decoder parity must be tested in 2E.6, including real Pixel audio at different source sample rates, nonzero stream start times, mute/solo, short clips, long cuts, seeks, and AAC presentation timing.

No UI volume control, gain automation, fades, independent clip mute, metering or limiter in this slice.


## Phase 2E.2

Status: implemented; CI and physical Windows/mpv acceptance pending. 2E.1 passed standard CI 45846574383 and paired performance 45846574362.

Schema 7 introduces optional boolean `audio_muted` (default false). Version 6 gain remains valid, and legacy projects retain their stored version when loaded/saved. A mute edit upgrades to version 7; older binaries cannot read version 7. Gain remains independently editable while muted. Effective gain is zero when muted, otherwise configured gain. Detach transfers both gain and mute to independent audio; reattach transfers both back. A muted clip contributes no audio while preserving the video and timeline duration. Zero gain remains distinct from reversible mute. Track mute/solo can still suppress otherwise unmuted clips. Active-track audio-only playback skips suppressed clips when advancing; composite playback retains silent timeline intervals.

The Audio button in clip properties and timeline context menu are available for a single editable clip with attached/independent source audio, including offline clips. Native popup provides 0/10/25/50/75/100/125/150/200/300/400% presets, 1- and 10-point adjustments with bounds, and mute/unmute. Every action commits through normal history; no-op gain does not create history. Editing stops stale preview, and subsequent playback uses the new gain. Mute is visible in labels and dimmed audio lanes. Raw source waveforms are deliberately not post-processing meters.

Source audition uses mpv `--af=lavfi=[volume=0.75:precision=double]` (example 75%) for audible non-unity gain, with no gain filter at unity. Muted or zero-gain video uses `--aid=no`; muted audio-only audition is suppressed rather than launching a player with no media streams. This is separate from the shared composite/offline DSP. Physical mpv audio output can differ due to resampling, downmixing, device conversion and player behavior. Selected-clip export honors mute/zero and existing track mute; audible non-unity gain is still rejected with the whole-project export instruction. Whole-project export is the supported processed output path.

Local isolated FFmpeg gain filter smoke check: signed samples 1000/-1000 become 250/-250 at 25%, 750/-750 at 75%, unchanged at 100%, and 4000/-4000 at 400%; amplified large samples saturate at signed PCM limits. This does not test the actual mpv process or the Go suite.

Native acceptance: choose a linked clip and independent audio clip, set 75%, mute, change to 125% while muted, unmute, undo/redo, save/reopen, detach/reattach and verify labels. Play active track and composite at 25/100/200%, verify mute and zero, mute the track and ensure clip unmute cannot override it, then export a project and inspect the mix. Attempt editing silent/detached/locked/multi-selected clips. Check transform inspector hides/restores all ordinary controls. Verify source audition on actual installed mpv including amplification and silent video. Record candidate revision and audio device/tool versions; do not claim physical parity from command construction tests.

## Phase 2E.3

Status: implemented; CI and physical acceptance pending. PR #19 merged with standard CI 45853835545 and paired performance 45853835463 passing.

Schema 8 adds optional integer `audio_fade_in_us` and `audio_fade_out_us`, zero/omitted means off, each bounded to 60 seconds. Version 7 mute and earlier gain/canvas schemas remain readable; load/save alone does not migrate. Gain and mute remain independent. The validated SetClipFades operation requires attached or independent source audio and respects track locks. Each UI change is one normal undoable project commit.

### Envelope semantics

Let D be the current clip duration, t the elapsed clip time, I the smaller of configured fade-in and D, O the smaller of configured fade-out and D. Inside the clip the fade multiplier is min(1,t/I) times min(1,(D-t)/O), with either factor equal to one when its duration is zero. The interval is half-open; samples outside contribute nothing. Both ramps are linear in amplitude individually; overlap multiplies the two factors, not equal-power crossfade or a limiter. Configured gain scales the result. The requested duration survives trims and extensions, re-anchored to the new edges. Move, copy and duplicate preserve settings; split preserves fade-in only on the left and fade-out only on the right, with no additional fades at the split. This intentionally reshapes the ramp if split inside it. Detach and reattach transfer both durations with gain/mute. Short clips clamp effective duration without modifying stored settings.

### Processing

Composite preview and offline export evaluate the same envelope per sample using their existing 48 kHz timeline grid, including on nonzero playback starts, off-grid cuts and unrelated span boundaries. Timeline tick is floor(sample offset * 1000000 / 48000) plus the run start. Fixed-point fade factors have millionth precision, their product is rounded at that scale, and PCM accumulation uses hundred-millionths of sample units. Final symmetric rounding and saturation occur once. No fade-specific restart or large envelope buffer. Equal decoded input/timing yields identical mixed PCM; different decoder adapters and lossy AAC still require physical qualification.

The existing Audio menu exposes independent duration presets (off, 0.1, 0.25, 0.5, 1, 2, 3, 5, 10, 30, 60 seconds) and remove both fades. Short-clip presets explicitly note duration limiting. Labels indicate configured fades; source waveform remains raw. The source audition path does not apply fades because mpv seek/filter timing is separate; it explicitly labels fades as composite-only. Choose Play composite to hear the processed mix. Selected-clip export rejects audible fades, while mute/zero can export silent video. No draggable handles or arbitrary envelope keyframes until 2E.4.

### Verification

Model tests cover version compatibility, persistence, validation, trim/split/copy/detach/reattach and selected-clip policy. Mixer tests cover endpoints, overlap, short-clip caps, positive/negative PCM, nonzero seek and sub-block progression. Synthetic PCM preview/export parity test uses different block sizes, overlapping fades and off-grid cuts. A local independent arithmetic check reproduced the expected ramp samples (0/4, 10000/10004 and 10000/9996 for a 20000-amplitude input). This is not execution of the Go suite. Git checkout remains unavailable due to DNS. Native acceptance: set both ramps on linked and detached audio, audition composite from start and mid-ramp, trim/split/copy, undo/redo, save/reopen, export and inspect decoded waveform. Verify source audition limitation and selected-clip rejection are visible, and record Windows, media tool and audio device versions. Real decoder/AAC timing and physical output remain open.

## Phase 2E.4

Status: implemented; CI and native acceptance pending. PR #20 merged with standard CI 45855331369 and paired performance 45855331358 passing.

### Envelope model

Schema 9 adds optional `audio_envelope` object with a `points` array of `source_us` (integer microseconds) and `percent` (integer 0–100). Missing/null means unity. A present envelope requires 1–128 points, strictly increasing source times within [0, asset duration], and attached or independent audio. The asset endpoint is a valid interpolation anchor, not an audible sample outside the clip. Versions 1–8 are supported; existing projects do not migrate on load/save alone. SetClipEnvelope validates whole-project invariants, respects locks, copies input, and preserves an unchanged fingerprint on no-op. Reset removes the envelope without changing clip gain, mute or fades.

Each point is relative attenuation of configured clip gain, not absolute loudness or dB. Interpolate linearly in amplitude, hold the first value before the first point and last value after the last point. A single point is constant. No automatic normalization or limiter. At timeline time t, evaluate the envelope at clip.In + (t - clip.Start). Trim and extension preserve the source-time points, including hidden points outside the current clip. Split copies the entire curve independently to both sides, preserving the envelope on retained source material; edge fades still follow 2E.3 rules and may reshape at a split. Move/copy preserve anchors, detach/reattach transfer the curve, and changes to one copy never mutate another. A shorter replacement asset is rejected if any retained point exceeds its duration. Clear removes hidden and visible points.

### Shared PCM and UI

Use the same 48 kHz sample grid as fades and linear gain. Evaluate envelope and fade multiplier in millionths, combine once at that precision and accumulate scaled signed samples in the existing int64 mixer. Interpolation uses 128-bit intermediate multiplication and 64-bit quotient from math/bits, including long valid source intervals. The envelope never amplifies beyond configured gain, and therefore does not increase the 2E.3 accumulation bound. Constant-gain path is unchanged. No new decoder boundary at keyframes. Zero configured gain or mute still suppresses decoding; an envelope with zero-valued points remains a valid contribution because it can vary or be edited, and does not change timeline length or audio stream policy by itself.

Hold Alt and click the audio graph area to insert a point, or Alt-drag an existing point to adjust time and attenuation. For the first insertion, unity anchors are seeded at the current clip edges; insertion directly on an edge replaces that anchor. Alt+Shift+click near a real point removes it. Escape cancels a drag, capture loss discards it, release commits one validated project transaction if the selection and fingerprint match. Horizontal movement clamps to current clip bounds and between neighbors (including hidden neighbors), vertical movement clamps to 0–100%. No crossing/duplicate keyframe timestamps. The timeline shows the yellow curve over raw source waveform; hidden points aren't draggable until revealed by extending the clip. The Audio menu's Remove volume envelope clears all points. Ordinary clip dragging remains unchanged without Alt. No live audio preview while dragging; composite playback hears the committed envelope. Direct mpv audition remains explicitly unprocessed for envelopes and fades; selected-clip export rejects audible automation, but muted/zero gain can export silent video.

### Verification

Regression coverage includes endpoint hold, interpolation and long-time overflow, validation and schema compatibility, input/accessor and split clone isolation, trim continuity, clipboard, audio ownership, lock/mute/export policy, point insertion/hit mapping and movement bounds, gain/fade/envelope multiplication, and synthetic sample-by-sample preview/offline parity using different block sizes and off-grid clip cuts. An isolated local Go reproduction of the interpolation and mixing arithmetic passed seven interpolation cases, a MaxInt64 interval, and four signed PCM cases (0, 5000, 20000, 30000). This is not the full repository suite. Git checkout remains blocked by DNS.

Native acceptance: exercise linked and independent audio, add several points, drag horizontally/vertically, delete, cancel using Escape and capture loss, undo/redo, trim/extend and split, copy, detach/reattach, save/reopen, mute and combine with fades. Check scrolled/zoomed timeline, short clips, locked tracks, no unintended ordinary clip drag, source audition labels, composite playback and processed export. Actual Windows UI and real decoder/AAC parity remain open.

## Phase 2E.5

Status: implemented; CI and physical acceptance pending. PR #21 merged with standard CI 45858633368 and paired performance 45858633271 passing.

The stereo meter is post-gain/fades/envelope/summation, at the signed-16-bit PCM output boundary. Peak is maximum absolute emitted sample per channel, normalized to 32768; dBFS is 20 log10(peak/32768), with silence displayed as -inf. Clipping counts each channel sample whose rounded pre-saturation value exceeds [-32768,32767]. A full-scale legal sample is not itself evidence of clipping. No automatic gain reduction. There is no true-peak oversampling, RMS/LUFS estimate or device/encoded output measurement. Source decoding may already have lost information; this meter cannot reconstruct it.

For composite playback each PCM block includes fixed-size levels, merged into the existing run metrics only after waveOut buffer completion and valid sample-clock confirmation. Canceled/unplayed queued buffers are not counted by reset cleanup. The latest completed buffer drives the bars, from -60 to 0 dBFS with green/yellow/red bands at -12/-3 dBFS. Run-wide peak hold and clip count survive until the next run; idle composite display explicitly labels last-run peak hold. Live bars blank when no completed buffer arrives for 250 ms. Polling is on the existing UI timer. Direct mpv audition is unmetered and labeled accordingly. Diagnostics retain confirmed frame counts and levels.

Whole-project export reports cumulative submitted pre-AAC PCM levels, including final silence padding. Statistics are emitted only after successful complete writes and retained through finalization. Successful GUI export status reports the peaks and clipping count; CLI progress prints the submitted PCM summary. Neither a progress event nor meter proves successful publication. A canceled/failed export does not claim success. No stats persisted to project files.

Verification: tests cover full-scale versus clipping, cancellation between contributions, stereo peaks, silence, aggregation, off-grid overlap, preview/export stats parity with distinct block sizes, padding, failed writer, run reset, logarithmic mapping, and export summary retention. Native acceptance: play a synthetic stereo sine with unequal channels; confirm approximately -6 dBFS for 50% full-scale amplitude, verify zero/mute/silence, gain/automation, overlap clipping and cancellation, stop/seek/restart, source audition label, short exports and final summary. Compare confirmed frame counts and submitted PCM peaks to an independent decoded fixture; do not equate them with DAC or post-AAC levels. Native layout, flicker, audio-device behavior and real-media parity remain unqualified.

Local arithmetic check (Go 1.23.2, isolated reproduction, not repository suite): legal full-scale samples 32767/-32768 and saturated 40000/-40000 produce identical output limits but only the latter contribute one clipping event per channel. Peak hold [32767,32768], clipping [1,1], half-scale -6.020600 dBFS, silence -inf; aggregate silence leaves peak and clip totals unchanged. Git checkout still fails DNS resolution. The meter uses buffered painting with a direct-paint fallback and clears old composite diagnostics when switching projects.

## Phase 2E.6

Status: implementation and automated qualification tests committed; candidate CI and physical acceptance pending. PR #22 merged with standard CI 45860773291 and paired performance 45860773375 passing.

The previous preview seek-first and offline origin-resample adapters were not guaranteed to produce identical decoded PCM. Both now use the same canonical sample-index mapping and FFmpeg command. Origin-based decode keeps resampling phase and delayed audio handling consistent; the cost is potentially slower late seeks, especially for long sources and multiple simultaneous clips. Playback retains its existing 30-second per-block timeout and bounded queue. No silent fallback to approximate decoding. Source audition remains distinct. Exact sample-grid-aligned seek comparison is supported; off-grid seek starts use a run-relative sample grid. AAC remains lossy.

See [audio qualification](audio-qualification.md) for local command-level evidence, the real fixture test, benchmark, and outstanding physical requirements. Earlier phase limitations about separate decoder timing are superseded by the canonical mapping for composite playback and whole-project export, not by a claim that native source audition, DAC output or arbitrary seek grids are identical.
