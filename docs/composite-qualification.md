# 2C.6.6 composite preview qualification

Status: implementation and automated evidence collection; **native acceptance NOT RUN**. Do not mark the gate complete on a successful build alone.

## Automated evidence

Use the exact candidate SHA. Normal CI includes format, tests, vet, race, Windows build and native-independent desktop tests. Linux runs the real-media synthetic HLG/AAC fixture and reports cold still, cached stream startup and steady frame render duration. It checks exact SDR still/stream pixel equivalence at the aligned first timestamp, two real audio streams summed at unity, stream reuse and retirement, and original source SHA-256 preservation. Existing HDR transform/export tests remain separate. The fake-decoder composite allocation benchmarks measure Go blending/mixing overhead only; review raw values and allocations, not an invented FPS guarantee. The paired refactor benchmark continues to measure the original project workload, not media performance.

## Physical Windows acceptance

Use disposable projects, keep original Pixel recordings outside the repo, and keep recovery/cache tests in a separate Windows account. Record revision, Windows build, CPU, GPU, RAM, display, HDR state, audio output/device driver, FFmpeg, FFprobe and mpv versions. Record five cold and five warm measurements of each repeatable action with raw values, medians, failures/timeouts and cache state. Use Task Manager or Process Explorer for parent/child process count, working set and peak memory; inspect all FFmpeg children, not just the GUI. No source paths or footage should be uploaded.

| ID | Scenario | Required observation |
| --- | --- | --- |
| C6-01 | One HLG video, two overlapping HLG videos, four visible layers | Correct geometry and opacity; <=4 video decoders; compare cold/warm startup, steady render time and process memory |
| C6-02 | Four video plus eight linked/independent audio contributions; exceed each limit separately | Peak counts within limits; excess fails explicitly, never silently drops layers |
| C6-03 | Leading/interior gap, off-grid cut, short overlay, audio-only tail | No outgoing picture beyond boundary; black/silence in gaps; exact end marker; document missed short clips |
| C6-04 | 20 rapid seeks, scrubbing, pause/resume, Stop/Escape and project switch during indexing | Latest marker wins; old audio/pictures do not replay; no overlapping replacement runs |
| C6-05 | Source replacement or removal while decoding; device disconnected; close during cold seek | Useful errors; UI remains responsive; no surviving child processes after normal retirement |
| C6-06 | Clear cache during composite preview and repeat cold/warm seek | Cache clear waits for owners, then regenerates; no stale in-memory stills; record startup difference |
| C6-07 | Known flash/beep sync fixture and actual Pixel VFR footage | Measure physical A/V offset/drift at start, cuts, seek and 1/5/10 minutes; record underruns and speaker/display latency separately |
| C6-08 | HDR-enabled and SDR display, portrait/landscape, skin/highlights | Composite labeled SDR; no unexpected clipping/range/gamut shifts relative to the SDR tone-map reference; compare original and selected-clip HDR export under the same HDR player settings |
| C6-09 | Source audition, trim/crop/transform/remux export after composite use | Existing HLG Main 10 metadata, orientation, audio, frame timing and media gates unchanged; compare source and remux decoded hashes |
| C6-10 | Repeated start/stop and 10-minute preview, including warmed cache | No accumulating live decoders or native buffers; record memory trend, dropped frames and actual effective preview FPS |

For C6-07 use a known synchronized audiovisual fixture (flash and beep generated at the same source timestamps). Use a camera/microphone capture or equivalent physical measurement method; log method, uncertainty and device settings. Clock-relative diagnostics alone are not physical evidence. Compare against the original playback path and the intended preview quality; do not silently accept drift or invent a performance threshold after measurement.

## Copyable report

After pausing a run, choose Track > Composite playback diagnostics. Wait for Workers finished: true before interpreting opened/retired counts. Ctrl+C copies the dialog. First-frame time measures from run creation to UI acceptance, not photon output. Maximum clock lag compares frame timestamps with completed PCM blocks. Queue starvation is a software queue observation, not a hardware underrun counter. A seek starts a new report. Retain individual reports for all repetitions.

## Color contract

The current multi-layer compositor is intentionally 8-bit tone-mapped BT.709 SDR and blends in display RGB. It is not scene-linear HDR or whole-project HDR export. Source audition and selected-clip HLG export remain separate. Follow [Pixel media qualification](qualification.md) for real HDR display and export evidence. Matching HLG metadata or matching SDR pixels does not prove physical HDR fidelity. Do not reuse the SDR compositor output for 2D HDR export.

## Evidence record

- Candidate SHA and CI URL:
- Paired refactor and composite allocation benchmark review:
- Windows environment and tool versions:
- C6-01 through C6-10 (PASS / FAIL / NOT RUN, raw evidence):
- Cold/warm startup, seek latency, decoder counts, working sets:
- Measured physical A/V latency, drift and underruns:
- HLG display/export evidence:
- Remaining blockers and accepted deviations:

Keep 2C.6 native acceptance open until evidence has been reviewed. Automated tests are a regression gate, not a substitute for physical tests.
