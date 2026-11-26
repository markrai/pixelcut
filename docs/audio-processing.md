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
