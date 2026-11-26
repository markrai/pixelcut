# Phase 2D.6: Whole-project HDR export qualification

Status: automated qualification and hardening implemented; **standard CI and physical acceptance NOT YET VERIFIED**. A merged feature or successful build is not evidence of native HDR fidelity.

## Automated gates

Run at the exact candidate commit:

```powershell
go test -count=1 -v ./internal/projectexport ./internal/media
go test -race -count=1 ./internal/...
go vet ./...
go test ./internal/projectexport -run '^$' -bench '^BenchmarkHLGBlend' -benchmem -benchtime=200ms -count=3
go build -o pixelcut.exe ./cmd/pixelcut
go build -o pixelcut-gui.exe ./cmd/pixelcut-gui
```

Standard CI must be green on Linux and Windows, including formatting, vet, portable race checks and real synthetic media tests on Linux. The previous main CI file contained duplicated workflow bodies embedded in benchmark shell commands; 2D.6 reconstructs the workflow, keeping the original gates and adding the 4K blend benchmark. Paired refactor performance qualification is separate and does not cover these export fixtures. Windows tests with missing media tools may skip integrations; do not count skipped cases as native acceptance.

| Gate | Automated check |
| --- | --- |
| EX-01 | Strict HLG stream contract, Main 10, hvc1, range/chroma, rotation normalization and metadata exclusion |
| EX-02 | Known 10-bit luma values, opaque overlays, black gaps, scene-linear RGB reference and translucent coverage |
| EX-03 | Full-range / center-sited input normalization, not merely relabeling |
| EX-04 | Irregular source timestamps at 0/100/300/400 ms; verify each rendered frame at 24/25/30/50/60 fps |
| EX-05 | Exact video packet PTS/duration grid, including B-frame reorder; reject duplicate, missing, negative or off-grid positions |
| EX-06 | Exact PCM cuts, silence, delayed 44.1 kHz source, 48 kHz stereo AAC duration, packet-preserving HEVC mux |
| EX-07 | Source change, cancellation during video/audio, late destination collision and no staged-file leak |
| EX-08 | Render refuses entirely unsampled visible video; bounded decoder/plan limits |
| EX-09 | CLI/GUI progress and command policy tests; report 640x360 and 4K blend allocations/time |

## Local command-level evidence

FFmpeg 7.1.5, Linux, independent smoke tests of the command paths (not the Go integration suite). Seven video frames at each supported FPS, stream-copy video plus AAC, verified zero starts, every video packet PTS/duration and audio duration in native timescale units:

| FPS | Video duration ticks at 240000 Hz | Audio samples at 48000 Hz |
| --- | ---: | ---: |
| 24 | 70000 | 14000 |
| 25 | 67200 | 13440 |
| 30 | 56000 | 11200 |
| 50 | 33600 | 6720 |
| 60 | 28000 | 5600 |

VFR source smoke: decoded timestamps 0, 100, 300, 400 ms, 10-bit luma 180, 280, 481, 580 (nominal 180, 280, 480, 580). Source frame ordinal selection preserved order; full project Go fixture still awaits CI. Local repository checkout failed due to DNS resolution. No CI result or physical run claimed here.

## Resource review

- Maximum canvas 4096 pixels per side and 8,847,360 pixels total; four active video decoders, eight active audio decoders. Video and audio render stages are sequential.
- Go raw video payload ceiling for one active span: two canvas buffers plus four layer rectangles at 6 bytes/pixel = 318,504,960 bytes (~304 MiB). Up to four cached source frame indexes at one million int64 ticks each = 32,000,000 bytes of logical index data. Slice growth, old span buffers awaiting GC, decoder internals and other allocations are additional. This is a live-payload estimate, not a process RSS cap.
- Final packet validation uses at most 648,000 bytes of bitset for 24 hours at 60 fps, plus bounded scanner buffers. No full output frame decode for this check.
- PCM uses 2400 stereo sample-frame blocks; no uncompressed project audio file. During mux, peak disk includes intermediate compressed video plus final compressed MP4; no fixed disk estimate because bitrate is content-dependent.
- FFmpeg/FFprobe child RSS, HEVC lookahead, native allocations and OS pipe buffering are not bounded by the Go buffer calculation. Report parent plus child peak working sets. The 4K scalar blend benchmark is CPU/Go allocation evidence only, not end-to-end encoding performance.
- Decode starts at the beginning of each source for each video span and each audio clip. Long source prefixes, repeated cuts and high-resolution translucent overlaps can be slow. No real-time or throughput guarantee. No GPU acceleration in this phase.
- File identity is path/size/mtime. Same-size same-mtime modifications, a race after final identity check, hard process termination and uninterruptible I/O are residual risks. Output publication requires same-directory hard-link support.

## Physical Windows / Pixel acceptance

Use disposable projects and retain originals outside the repository. Record candidate SHA, Windows build, CPU/GPU/RAM, display HDR mode, audio device, FFmpeg/FFprobe/mpv versions and source codec/range/chroma/orientation. Keep raw private recordings and local paths out of shared reports. Run five cold and five warm measurements for repeatable export scenarios; record raw times, medians, failure counts, parent and child peak working sets, disk space and child process cleanup. No fabricated pass results.

| ID | Scenario | Acceptance observation |
| --- | --- | --- |
| P-01 | Real Pixel 10 and Pixel 9 HLG, portrait/landscape, camera motion, irregular cadence | Source probe succeeds or useful fail-closed diagnostic; output plays without unintended frame skips or stretched geometry; inspect source/output metadata and timestamps |
| P-02 | Highlights, near-black, skin tones, saturated colors, gradients and chroma edges | Compare source, no-op project export and transformed export in same HDR player/display settings; no unintended tone mapping, washed-out black or obvious clipping/banding. Record differences; lossy encode is not bit-exact |
| P-03 | Crop, cardinal source rotation, edit rotation/flips, four layers, odd-bound placement, 0/50/100% opacity | Correct geometry, layer order, black padding; compare to numeric reference for simple patches |
| P-04 | Flash/beep fixture at start, off-grid cuts, 1/5/10 minutes and tail | Inspect decoded media alignment (within one output video frame for a frame-grid marker plus measured AAC onset tolerance); separately record physical player/display/speaker latency. No accumulating drift |
| P-05 | Linked/detached audio, mute/solo, overlapping audio, audio-only tail | Expected contributions and silence; clipping policy documented; no missing or duplicate audio |
| P-06 | 4K long source, repeated late trims, two/four opaque and translucent layers | Record wall time, output size, peak total working set and CPU. No unbounded growth with timeline duration; all children retire on finish/cancel |
| P-07 | Cancel during indexing, video, audio, final verification; close while busy; restart after cancel | No output if canceled before publication; no surviving children or staged files after normal cleanup; editor remains locked until cleanup and then usable |
| P-08 | Existing file, late collision, offline/changed source, insufficient disk, unsupported destination | Useful error; no overwrite, no corrupted published MP4; existing sources/projects untouched |
| P-09 | Export unsaved project without selection; all FPS choices; selected clip shortcut; preview after export | Correct output contract; dirty state unchanged; preview and media workers recover |
| P-10 | Repeat source trim/crop/remux export and playback after project export | Existing selected-clip HLG behavior preserved |

For inspection:

```powershell
pixelcut inspect output.mp4
ffprobe -v error -select_streams v:0 -show_packets -show_entries packet=pts,duration -of compact output.mp4
Get-FileHash source.mp4 -Algorithm SHA256
```

Record actual source hashes before and after, with hashes only if sharing evidence. Do not use screenshots of HDR playback as a calibrated luminance measurement. A visual pass is not a substitute for instrumental display measurement if exact physical fidelity is required.

## Sign-off

- Standard CI: pending candidate run.
- Real Windows UI acceptance: NOT RUN.
- Real Pixel source and physical HDR/A/V acceptance: NOT RUN.
- End-to-end 4K/time/RSS measurements: NOT RUN.

Phase 2D implementation is feature-complete when this slice merges, but qualification remains open until evidence is collected. Keep the distinction visible in the roadmap. Phase 2E may be developed separately without representing 2D as production-qualified.
