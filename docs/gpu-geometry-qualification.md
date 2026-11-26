# 3A.3 GPU geometry and transfer boundaries

## 3A.3a: isolated CUDA scaling check

The existing CPU reference and experimental CUDA decode renderer stay unchanged. The new qualification command compares two complete paths on the same HEVC Main 10 HLG source and ordinal window:

- CPU: explicit software HEVC decode, planar yuv420p10le, nearest-neighbor scale, planar yuv420p10le.
- CUDA: explicit native HEVC CUDA hardware frames, scale_cuda nearest with P010 output and passthrough disabled, hardware download, P010 to planar yuv420p10le.

Both select the same consecutive presentation ordinals after scaling, use the same SHA-256 framehash output and strict source index, compare exact frame size, pixel hashes, timestamps and durations, and fail on mismatch or hardware failure. No silent CPU fallback or output media. Existing three-minute deadline, 256 KiB per captured stream, source identity checks, cancellation/reaping and 1–300 frame bound apply. Output dimensions must be even, 2–8192 per side, at most 33,554,432 pixels. Input retains existing decode qualification restrictions. Scaling uses coded orientation, not display rotation or aspect-fit geometry. Arbitrary output dimensions are diagnostic, not a claim of support.

Nearest-neighbor is a conservative qualification target, not the production interpolation algorithm. CPU and CUDA coordinate conventions can differ; a mismatch is evidence and blocks use of that configuration, not a reason to relax equality. The check includes decode, scale, transfer and hashing, so it does not isolate scaler throughput or distinguish decoding from scaling errors; run gpu-decode-check on the same window first.

The JSON source width/height are coded source dimensions; output_width/output_height are requested dimensions. source_bytes_per_frame and download_bytes_per_frame are nominal tightly packed 10-bit 4:2:0 payloads (three bytes per luma pixel), not measured PCIe transfer or GPU allocation including stride/padding. Status matched is required before treating a configuration as qualified.

## Physical test on Windows

```cmd
go build -o pixelcut.exe ./cmd/pixelcut
.\pixelcut.exe gpu-decode-check -frames 120 "C:\Videos\pixel10.mp4"
.\pixelcut.exe gpu-scale-check -width 960 -height 540 -frames 120 "C:\Videos\pixel10.mp4"
.\pixelcut.exe gpu-scale-check -width 960 -height 540 -start-frame 120 -frames 120 "C:\Videos\pixel10.mp4"
set PIXELCUT_QUALIFY_CUDA_SCALE=1
go test -count=1 -v ./internal/media -run "^TestHEVCDecodeCheckRealVFR$"
set PIXELCUT_QUALIFY_CUDA_SCALE=
```

Adjust dimensions for portrait and UHD sources, and frame window for actual source length. Record GPU/driver, FFmpeg version, raw and scaled results, and failures. Repeat same-size, downscale, upscale and non-integer ratios, landscape/portrait, VFR and 4K. Ordinary CI covers CPU VFR frame selection for same-size, downscale and upscale, invalid size and exact command graph boundaries. GPU checks are opt-in.

Local FFmpeg 7.1.5 CPU command-level experiment on a generated 64x48 VFR HLG source produced four 32x24 planar frames of 2304 bytes at 100000, 133333, 200000 and 233333 microseconds. This is not candidate Go-suite or CUDA execution evidence. Local checkout blocked by DNS; candidate CI and all scale hardware checks pending.

## Remaining 3A.3 work

3A.3b: evaluate real production scale interpolation and chroma/range handling against reference; qualify crop/rotation/flip/placement and explicit transfer boundaries. Integrate only qualified operations and retain explicit CPU reference without silent fallback. Do not select CUDA geometry simply because scale_cuda is advertised or nearest-neighbor passes. Compare real preview/export geometry, cancellation, bounded resources and actual performance, including late seeks. GPU HLG compositing remains 3A.4; hardware encoding remains 3A.5. No project schema or preference changes in 3A.3a.
