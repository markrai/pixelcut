# Integrated GPU qualification (3A.6)

PR #46 merged at 62d2416c5acbcd6563c2fcdc69b4301b8815e856; CI 46082421625 and performance 46082421575 passed. Public explicit CUDA decode and experimental NVENC encode are independent. x265 and CPU decode remain the default. No automatic fallback, GPU geometry or GPU composition activation.

## 3A.6a: combined backend matrix

Run the actual public RenderProjectObserved pipeline, with the same project, source, canvas and CFR for each of CPU/x265, CUDA/x265, CPU/NVENC, CUDA/NVENC. NVENC uses the public fixed device 0, QP 16, no test-only overrides. First exercise the existing small synthetic VFR fixture with transformed translucent overlapping video, audio, cuts and gaps (one export per mode); then test a real source (first 10 seconds of a supported 10-60 second HLG video with audio, native displayed resolution, leading 100ms gap, 303 output frames). Three independent complete exports per mode for real footage, rotating/reversing order to reduce ordering bias, not a fully balanced experiment. Timing includes the entire public export including its internal verification and publication; separate post-export contract checks and comparisons excluded. Log explicit mode, timestamps, byte counts, FPS, medians and sampled Go heap. Go heap is not FFmpeg process or VRAM usage.

Exact CPU/CUDA pre-encode SHA256 per span and expected byte counts are mandatory. All outputs must pass the existing color, timing and audio presentation contract; check observed progress and selected backend warnings. Independently software-decode the last output of each mode and require identical decoded AAC hashes across all four. Require exact decoded video parity when only decode backend differs (same encoder), with per-frame and pooled PSNR and max code error recorded. Cross-encoder comparisons are diagnostic, not a new perceptual gate or proof of source-relative fidelity. A same-encoder mismatch warrants investigation of decode parity or encoder nondeterminism rather than weakening the gate. The comparison is streaming, not retaining whole decoded videos. Source SHA256 and project fingerprint unchanged, no staged files. Combined cancellation after at least one submitted frame and subsequent fresh verified export also required. A failed synthetic case stops the real workload. No production code or settings changes.

```cmd
git pull
set PIXELCUT_QUALIFY_GPU_MATRIX=1
set "PIXELCUT_GPU_MATRIX_SOURCE=C:\dev\project\pixelcut\test-media\test.mp4"
go test -count=1 -v -timeout 13m ./internal/projectexport -run "^TestIntegratedGPUBackendMatrix$"
set PIXELCUT_QUALIFY_GPU_MATRIX=
set PIXELCUT_GPU_MATRIX_SOURCE=
```

Use the same FFmpeg/FFprobe pair for a run. Test logs absolute paths and versions; 12-minute suite context. Ordinary CI exercises the matrix schedule and backend policy only; no physical GPU assumption.

## Remaining 3A.6 gates

3A.6b: physical resource sampling of host process tree and NVIDIA GPU memory/utilization during the timestamped runs; distinguish per-process and device-wide measurements. 3A.6c: review combined failure behavior, GUI/CLI public-path acceptance, CPU-only hardware compatibility, and documentation/architecture closeout. Explicit unavailable GPU must fail without publication, not fall back silently. Previous CUDA preview, HDR display, physical audio and deferred geometry/composition acceptance remain separate; do not infer those passed from the export matrix.

## 3A.6a physical result

PR #47 merged at 5660e3997c9a59546db3e411be047ab4f0dc6ec8; CI 46084546731 and performance 46084546689 passed. TITAN N-129308-g17de9f8597-20261126, fixed NVENC device 0/QP16. All four modes passed on 13-frame synthetic VFR/multilayer and 303-frame real portrait HLG (1080x1920, 484800 audio samples). Exact pre-encode hashes, same-encoder decoded YUV max 0/0/0, decoded audio equality, combined cancellation/recovery and publication checks. Real median wall CPU/x265 37.4786709s, CUDA/x265 37.7300045s, CPU/NVENC 11.8334383s, CUDA/NVENC 11.9889242s; speedups 1.000/0.993/3.167/3.126. Files 14978089 bytes x265 and 20356457 NVENC. No measurable additional benefit from CUDA decoding in this workload. CPU decode + NVENC is the measured fastest option, not a global default change. No peak host/GPU measurements supplied.

## 3A.6b: automated Windows resource qualification

Opt-in sampling during each real matrix export; synthetic cases and ordinary CI do not query NVIDIA. One baseline GPU query and Windows root snapshot before the timed run, then a 500ms ticker with native Toolhelp process enumeration and GetProcessTimes/GetProcessMemoryInfo for the root and live descendants, followed by a bounded nvidia-smi query. No separate console, continuous external loop, production instrumentation or dependencies. The query process is not running during host enumeration. Each completed export logs one JSON resource summary tagged by mode/round, in addition to the existing timed results. The collector stops and joins after each render, even on render failure. NVIDIA query timeout 3s plus bounded wait; no unsupported metric is treated as zero. Require at least two host and two GPU observations and an observed child; device identity must stay stable. Incomplete evidence fails qualification, not production export.

Host memory: sampled simultaneous sum of working sets (may double count shared pages), private commit bytes (not private working set), separate root and descendants plus total peaks. CPU: accumulated sampled CPU time deltas for stable PID/creation identity, plus newly observed processes created since previous sample; divide by sample-window seconds to report observed core equivalents, not percent of all logical CPUs. Missed short-lived/exiting processes and access failures can undercount; counts of failed descendant observations are reported, not proof of complete coverage. Sampling includes test-process overhead and does not measure exact process-lifetime peak or total machine RAM. GPU baseline and sampled peaks/means per NVIDIA UUID (index, model, driver, memory.used, GPU/decoder/encoder utilization), device-wide not process-attributed, no assumed NVENC-index-to-UUID equivalence. Means are arithmetic sample means, not precise time-weighted utilization. Query overhead and irregular intervals can affect timings; use the uninstrumented 3A.6a results as performance reference. No hard-coded resource budget or claim of resource PASS without physical review.

Run only the already-qualified real workload with sampling from one CMD window; keep other GPU-heavy applications idle. Save the complete terminal output or redirect to a new report file. No need to start nvidia-smi separately.

```cmd
git pull
set PIXELCUT_QUALIFY_GPU_MATRIX=1
set PIXELCUT_QUALIFY_GPU_RESOURCES=1
set "PIXELCUT_GPU_MATRIX_SOURCE=C:\dev\project\pixelcut\test-media\test.mp4"
go test -count=1 -v -timeout 13m ./internal/projectexport -run "^TestIntegratedGPUBackendMatrix$/^real-hlg$" > gpu-resource-qualification.txt 2>&1
type gpu-resource-qualification.txt
set PIXELCUT_QUALIFY_GPU_RESOURCES=
set PIXELCUT_QUALIFY_GPU_MATRIX=
set PIXELCUT_GPU_MATRIX_SOURCE=
```

Redirected output will not appear live; omit redirection for live progress. Send the full report. Windows CI tests native root and live child snapshots without a GPU; portable tests exercise parsing, aggregation and output cap. Actual NVIDIA physical resource qualification remains pending. If nvidia-smi is not on PATH or a utilization field is unsupported, report the failure rather than bypassing evidence.
