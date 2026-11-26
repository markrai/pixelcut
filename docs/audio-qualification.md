# Phase 2E audio qualification

## Status and evidence

2E.1–2E.5 are merged; PR #22 standard CI 45860773291 and performance qualification 45860773375 passed. 2E.6 merged via PR #23; standard CI 45871806630 and paired performance qualification 45871806823 passed. no physical Windows audio-device or Pixel acceptance is claimed. Existing 2D HDR physical acceptance also remains open.

### Local command-level evidence

FFmpeg 7.1.5, synthetic AAC fixture (44.1 kHz 997 Hz sine): old input seek at 0.250019 s and origin-resample trim beginning at sample 12000 differed in 4792 of 4800 compared channel samples, maximum absolute difference 425 PCM units. This is a diagnostic comparison, not a product defect severity measurement.

Synthetic 64x48 HLG/AAC fixtures (delayed 44.1 kHz stereo and independent 48 kHz stereo) with the new canonical command:
- Delayed source samples [960,20160): 19200 stereo frames, approximately 67 ms wall time.
- Same source samples [7679,20160): 12481 stereo frames, approximately 64 ms, exact equality to the corresponding suffix of the first decode.
- 48 kHz source samples [2400,14400): 12000 stereo frames, approximately 67 ms.
- Delayed source leading window: zero peak.
- Independent Python reference mix with gain/fades/envelope and AAC 192 kbps round-trip: normalized correlation 0.999980608; reference sample peaks 7440/5562. This is a command-level/reference experiment, not execution of the Go mixer or complete product exporter.

Git checkout was attempted and failed DNS resolution. The full Go repository suite and Windows physical acceptance were not run locally. The merged CI passed the automated gates. Local timings are one-off short-fixture observations, not latency guarantees.

## Automated gates

| ID | Gate | Evidence / acceptance |
| --- | --- | --- |
| AQ01 | Schema and editing | Versions 1–9; gain, mute, fades, envelope persistence, lock/undo/redo, trim, split, copy, detach/reattach |
| AQ02 | Shared mix | Exact synthetic samples, clipping/peak stats, silence and order independence; sample-level automation |
| AQ03 | Source mapping | Signed floor, off-grid boundaries, overflow and invalid decode intervals; one canonical decoder command |
| AQ04 | Real PCM | TestAudioQualificationRealDecoderParity: delayed 44.1 kHz and 48 kHz AAC, independent/linked clips, gain/fades/envelope, overlap, mute and gaps; byte equality before encoding and matching meter totals |
| AQ05 | Seeks | Exact aligned seek suffix; off-grid seek duration and stream retirement; no universal bit equality for arbitrary off-grid starts |
| AQ06 | Resource lifecycle | Normal EOF, exact expected sample interval, process reaping, discontinuity restart, eight-stream cap, cancellation and source mutation gates |
| AQ07 | Final mux | Real project render with submitted statistics equal to reference, decoded AAC correlation greater than 0.95; existing exact presentation duration, video packet preservation, no overwrite and cancellation tests |
| AQ08 | Processing cost | BenchmarkCompositePreviewAudioAutomation (unity vs eight streams with fades/envelope); report ns/op, B/op and allocs/op in existing composition benchmark artifact |
| AQ09 | CI | Formatting, Linux tests/coverage, race, vet, Windows tests/vet/build and paired performance qualification |

The test compares the complete content interval, not just a steady-state window. Export silence padding is accounted for separately. Correlation is not a perceptual quality metric or bit-exact AAC guarantee.

## Physical acceptance checklist

Record commit, Windows build, FFmpeg/mpv versions, CPU/GPU, RAM, audio device/sample format, source file identity and test duration. Retain actual measurements and failures, not just a checkbox.

| ID | Physical gate | Acceptance |
| --- | --- | --- |
| AP01 | Pixel 9 and Pixel 10 HLG recordings | Confirm linked and detached audio, source preservation, correct color and audible cuts |
| AP02 | Stereo and gain | Unequal-channel reference, 25/50/100/200/400%, mute/unmute, solo, silent assets; compare confirmed peaks and clipping to reference |
| AP03 | Automation editing | Native fade menus, draggable keyframes, capture loss/Escape, undo/redo, trim/split/extend and save/reopen |
| AP04 | Seek and transport | Start, pause, resume, seek across gaps and trims, aligned and off-grid; no stale audio or meter counts from retired run |
| AP05 | Long sources | 10-, 30-, 60-minute sources; cold seek near beginning/middle/end, one and eight contributions; record first completed audio buffer, max block time, timeouts and CPU/RSS |
| AP06 | Sustained playback | 10-minute and 30-minute composite runs, record confirmed frames, starvation episodes, audio/video clock lag, process peaks and retirement; assess audible gaps and A/V sync |
| AP07 | Device behavior | Actual Windows waveOut device, output routing, device removal/failure, cancellation and clean restart; digital peak does not measure physical speaker output |
| AP08 | Meter and layout | Resize, DPI, native repaint, stereo peak hold/clipping, no stale values after project switch, source audition explicitly unmetered |
| AP09 | Export | All supported CFR rates, long A/V duration, AAC presentation, sample peak/clipping report, source hash and no overwrite, canceled exports leave no published output |
| AP10 | Physical sign-off | No unaccounted timing or resource regressions; explicitly review any seek timeout/latency and document supported usage before marking audio qualified |

### Resource and timing contract

The shared canonical decoder normalizes timestamps from source origin, resamples to 48 kHz, pads and trims by exact sample indices. It does not seek before resampling. Source index at run-relative frame n is floor((source in + run start - timeline start) * 48000 / 1000000) + n. The preview grid begins at the requested playback start; export begins at zero. Integer-microsecond starts divisible by 125 align with the export grid. Off-grid starts differ in phase by less than one sample period; exact PCM suffix equality is not promised there.

A maximum eight decoders are active. One 2400-frame stereo int16 block is 9600 bytes, one raw read buffer is 9600 bytes, the int64 mixer accumulator payload is 38400 bytes. The existing two-block producer queue and three native queued buffers hold at most 48000 bytes of PCM payload combined (excluding current block, headers, allocations and process memory). Statistics are fixed-size. No full-source PCM cache or full timeline PCM temporary file. These are logical payload bounds, not process RSS.

Canonical origin decoding costs O(source prefix) work at cold late seek or new trimmed clip. No performance claim for long sources from short fixtures, and the 30-second preview per-block timeout remains in place. This is the principal regression risk introduced by the exact decoder path. A source-indexed/checkpointed cache or exact seek strategy may be needed after AP05. Do not silently restore approximate input seeking. Direct mpv audition is unchanged and does not share all automation or metering. No LUFS, true-peak, DAC or post-AAC identity claim. Source identity checks use path/size/mtime, not a lock or cryptographic content guarantee.
