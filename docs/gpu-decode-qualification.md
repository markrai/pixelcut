# 3A.2 CUDA HEVC decode qualification

## Scope and status

The first hardware decode implementation is an explicit qualification command, not an enabled preview/export backend. PR #24 merged with CI and performance passing. User-reported Windows diagnostics initialized CUDA, D3D11VA, D3D12VA and Vulkan; actual frame parity is not yet established. PR #26 corrects FFmpeg filter inventory independently. The qualification branch does not depend on that parser fix.

The decoder uses FFmpeg native HEVC with explicit CUDA hardware acceleration and CUDA hardware-frame output. Download to P010, then convert to planar yuv420p10le on CPU. The software reference explicitly disables hardware acceleration and uses the same planar format. Both disable automatic rotation, normalize source timestamps, decode from source origin and select the same consecutive presentation ordinals. No approximate input seek, scaling, tonemapping or geometry processing. A failed CUDA path cannot count software output as GPU success because the graph requires hardware-frame download. Compare SHA-256 per raw frame, exact frame dimensions, expected byte count (three bytes per luma pixel for 10-bit 4:2:0), PTS/duration, ordered frame count and source timestamp index (one microsecond rounding tolerance for CPU index comparison). Frames stay in the FFmpeg process; Go captures only bounded hashes and diagnostics. Source identity is checked before/after the decode passes. No output media file.

Supported input: exactly one ordinary HEVC Main 10 video stream with explicit BT.2020 HLG tags, explicit supported range/chroma, square pixels, even dimensions and cardinal rotation; dimensions capped at 8192 per side and 33,554,432 pixels. Source index uses the existing strict one-million-frame limit. Selected window 1–300 frames. Overall timeout three minutes, command output cap 256 KiB each for stdout/stderr, cancellation and child reaping via Go exec. A long clip may spend substantial time indexing and decoding its prefix; timings include process startup, filtering, transfer and hashing but exclude the shared index/inspection. They are not an isolated GPU throughput benchmark.

## Physical Windows procedure

Close running PixelCut executables, pull and rebuild CLI. Choose a short Pixel 10 HLG source whose original file is not being modified. In Command Prompt:

```cmd
go build -o pixelcut.exe ./cmd/pixelcut
.\pixelcut.exe gpu-decode-check -frames 120 "C:\Videos\pixel10.mp4"
.\pixelcut.exe gpu-decode-check -start-frame 120 -frames 120 "C:\Videos\pixel10.mp4"
```

Adjust the ordinal/count to the actual source length. Repeat on Pixel 9, portrait/rotated HLG, high-motion or VFR source, UHD and 60 fps if available. Verify status matched and matched equals frames for every case. Capture driver version, GPU model, FFmpeg version and JSON. Keep file paths private when sharing. A mismatch or hardware error is a failure, not permission to relax pixel equality or silently use CPU. For a generated VFR Main 10 fixture on the actual GPU:

```cmd
set PIXELCUT_QUALIFY_CUDA=1
go test -count=1 -v ./internal/media -run "^TestHEVCDecodeCheckRealVFR$"
set PIXELCUT_QUALIFY_CUDA=
```

Ordinary CI executes the CPU real-media reference and synthetic parser/selection/failure tests; CUDA execution is intentionally opt-in on a machine with the driver. CI alone cannot certify GPU support.

## Acceptance

| Gate | Evidence |
| --- | --- |
| D01 | Explicit hardware device and hardware surface transfer; fail closed on missing CUDA |
| D02 | Generated 10-bit nonconstant VFR fixture, full and interior ordinal selection, exact CPU/GPU pixel and timing parity |
| D03 | Actual Pixel 9/10 sources, landscape/portrait, representative CFR/VFR and long-GOP windows |
| D04 | Cancellation, missing/invalid input, source mutation and no partial output or leaked child |
| D05 | Measure CPU/GPU wall times at matched windows, record transfer cost; no assumption of speedup |
| D06 | Integrate qualified decode adapter into preview/export with existing frame-index, source checks, geometry and publication invariants; CPU fallback only with explicit observable policy |

Local FFmpeg 7.1.5 command-level check: CPU fixture 64x48, VFR 9 frames, selected ordinals 2–5 yielded exactly four 9216-byte planar frames at 100000, 133333, 200000, 233333 microseconds, matching the independent ffprobe index. Explicit CUDA decode failed with missing libcuda.so.1, rather than returning software output. Physical GPU parity, normal renderer integration and full candidate CI remain pending. No schema/dependency changes. No GPU acceleration speedup is claimed.
