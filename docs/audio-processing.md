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
