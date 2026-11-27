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
