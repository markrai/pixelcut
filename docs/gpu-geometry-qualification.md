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

## 3A.3b candidate and provisional quality gate

PR #29 passed CI 45885116645 and performance 45885116756 and merged. User reported 1920x1080 to 960x540 nearest-neighbor parity:

| Ordinals | Matched | CPU seconds | CUDA seconds |
| --- | --- | --- | --- |
| 0–119 | 120/120 | 0.9600741 | 3.5319224 |
| 120–239 | 120/120 | 1.8340004 | 0.6370063 |

These timings include prefix decoding and startup; they do not establish steady-state speed. Nominal frame payload falls from 6220800 to 1555200 bytes.

The preview candidate is strictly limited to unrotated, uncropped, untransformed limited-range left-sited Main 10 HLG with a source-sized canvas and even aspect-preserving downscale up to 960 pixels per side. The CPU reference uses the existing Lanczos preview graph. The CUDA graph scales hardware frames with Lanczos before downloading and runs the same CPU tone map. Still and persistent preview decoders share an internal candidate graph builder; the switch is not exposed to normal users. Crops, rotation, placement, odd dimensions, differing aspect ratios, other chroma/range and exports are not qualified by this check.

The qualification command uses two concurrent FFmpeg processes, bounded one-frame-per-path BGRA payload buffers, bounded frame hash streams, exact source ordinal selection and timestamp/duration comparison. Each raw frame is independently hashed and checked against the emitted FFmpeg framehash. No partial success, output file or fallback. The quality report measures RGB code values after SDR tone mapping: minimum PSNR over every frame and channel (identical capped at 100 dB), maximum absolute channel delta and mean absolute delta; opaque alpha required. Provisional screening thresholds are 40 dB minimum and 16 maximum 8-bit codes. These are not perceptual HDR guarantees; a failure blocks activation and should be investigated before modifying limits. Wall time is combined concurrent comparison time, not CPU/GPU individual performance.

```cmd
go build -o pixelcut.exe ./cmd/pixelcut
pixelcut gpu-preview-scale-check -width 960 -height 540 -frames 120 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -width 960 -height 540 -start-frame 120 -frames 120 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -width 640 -height 360 -frames 120 "test-media\test.mp4"
```

Run the raw nearest/decode checks first. Record output for real landscape, portrait without rotation metadata, detailed edges, gradients, motion and UHD footage. Candidate CI and physical Lanczos parity are pending. Normal renderer activation, persistent decoder measurements, remaining geometry and HDR export remain outstanding. Do not treat status within_limits as blanket GPU qualification.

For opt-in integration testing on a CUDA workstation, including actual still and persistent stream decoding with discontinuity retirement:

```cmd
set PIXELCUT_QUALIFY_CUDA_PREVIEW_SCALE=1
go test -count=1 -v ./internal/media -run "^TestExperimentalCUDAPreviewScale$"
set PIXELCUT_QUALIFY_CUDA_PREVIEW_SCALE=
```

The small fixture reports aggregate sequential stream call timings including startup, seek and transfer; this is not sustained playback throughput. Ordinary CI compares two software preview processes on the existing VFR fixture. Local FFmpeg 7.1.5 command-level CPU check on that fixture produced four 3072-byte BGRA frames at 100000, 133333, 200000 and 233333 microseconds; all raw SHA-256 hashes matched the simultaneous framehash stream and alpha was opaque. Local repository checkout remains blocked by DNS; this is not candidate Go suite or CUDA evidence.

## Physical 3A.3b observation and diagnostic follow-up

PR #30 merged, CI 45886342264 and performance 45886342125 passed. User reported that the 1080p recording was rejected before scaling at 960x540 (both windows) and 640x360; the previous broad eligibility error did not identify which source property failed. The synthetic CUDA fixture compared all 10 frames, minimum frame/channel PSNR 42.72219584041022 dB, max channel delta 19, mean absolute delta 0.5875192901234568 and zero identical frames. This failed the unchanged max-delta gate of 16 despite passing the PSNR gate of 40 dB. The opt-in test stopped before still and stream qualification.

The follow-up reports exact rejection reasons and source coded dimensions, range, chroma location and display rotation in the JSON. For compared frames it reports number of RGB channel samples compared, count strictly above the existing delta limit, and first occurrence of the worst sample with zero-based frame offset, row-major pixel index, channel and CPU/CUDA values. Frame ordinal is first_frame + frame_offset; pixel x is pixel_index modulo output_width; y is integer division by output_width. Exact frames have no worst sample. This is diagnostic evidence, not a change of eligibility or quality limits. The pre-tone-map and post-tone-map contributions still need isolation; no GPU geometry activation or speedup is claimed.

## Cardinal display rotation follow-up

PR #31 merged, CI 45887172992 and performance 45887172988 passed. User report confirms coded 1920x1080, limited range, left chroma and -90 degree display rotation. The diagnostic command previously rejected this valid portrait recording. The candidate now accepts existing validated cardinal display rotations. Output width and height are in displayed orientation: for this source use 540x960 or 360x640. GPU scaling uses coded-axis dimensions (960x540 or 640x360 respectively), downloads reduced 10-bit frames, then applies the same explicit CPU rotation mapping used by CUDA decode. The CPU reference applies rotation before its existing Lanczos geometry graph. Order and chroma resampling differences are included in the same SDR quality gate; no claim of commutation or parity is made.

```cmd
pixelcut gpu-preview-scale-check -width 540 -height 960 -frames 120 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -width 540 -height 960 -start-frame 120 -frames 120 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -width 360 -height 640 -frames 120 "test-media\test.mp4"
```

Generated cardinal display-matrix fixtures exercise the coded/display dimension boundary and CPU VFR paired comparison. Candidate CI and real GPU rotation quality remain pending. No changes to thresholds or ordinary rendering; earlier synthetic max-delta failure remains unresolved.

## 3A.3c: stage isolation

PR #32 merged, CI 45890749200 and performance 45890748922 passed. Physical portrait comparison and the prior synthetic max-delta failure are still unresolved. Add optional -diagnose to gpu-preview-scale-check to run three paired pre-tone-map planar 10-bit comparisons after the existing BGRA comparison (including when the BGRA quality gate is exceeded). The command still returns failure on the existing SDR quality gate. Stage diagnostics never authorize GPU activation.

- cuda_against_reference: production CPU geometry before tone mapping versus CUDA scaled/downloaded/rotated geometry before tone mapping.
- cpu_order_against_reference: the same CPU reference versus a CPU Lanczos scale in coded axes followed by display rotation, controlling for geometry order and elimination of the reference intermediate scaling step.
- cuda_against_cpu_order: the reordered CPU control versus the CUDA candidate, isolating remaining combined decoder/scaler differences (verify raw decode parity separately).

Each comparison reports Y, U, V code-value counts, differing samples, max and mean absolute deltas, minimum per-frame PSNR (peak 1023, exact capped at 100 dB), exact frames, and concurrent wall time. Planar geometry output is explicitly normalized to yuv420p10le at the boundary; these are not HDR perceptual or export guarantees. Compare SDR and planar results without equating 8-bit and 10-bit PSNR. The control is not proof that rotation alone caused a difference: the reference can have two scale stages. Two processes at a time, two frame buffers, bounded hash capture and existing shared three-minute timeout; no media files. Stage failure is an error, never success. SDR metrics remain in the report even when diagnostics fail. No interpolation or threshold changes.

```cmd
pixelcut gpu-preview-scale-check -diagnose -width 540 -height 960 -frames 30 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -diagnose -width 540 -height 960 -start-frame 120 -frames 30 "test-media\test.mp4"
```

Use short windows first because the diagnostic runs four comparisons total. No local GPU results available; candidate CI pending.

## 3A.3d: chroma controls and independent timing

PR #33 merged (CI 45893321213, performance 45893321189 passed). Physical observations, 30 frames at coded 1920x1080 to 960x540: March 31 FFmpeg N-123778-g3b55818764 rotated RGB max 53, PSNR 43.34, Y/U/V max 131/42/17; November 26 N-129308-g17de9f8597 rotated RGB max 27, PSNR 51.63, Y/U/V max 1/26/13. Same video stream remuxed without display rotation: RGB max 47, PSNR 52.19, mean 0.11244, 1386 over-limit samples; Y/U/V max 1/30/15. CPU geometry order exact 30/30 without rotation. New build synthetic smooth fixture passes BGRA max 13 and still/stream max 11, lifecycle 2 opened/2 retired/peak 1. Small fixture sequential CPU/GPU stream timings 91.27/313.48 ms do not establish throughput. Real-source quality still fails.

Reports now identify resolved FFmpeg and FFprobe paths and version lines, and use those absolute paths for the duration of qualification. This is path/version evidence, not a binary fingerprint or driver qualification. Optional -chroma implies -diagnose and compares four explicit software scaling hypotheses in coded axes before rotation: left to left, left to center, center to left, center to center. Uses scale in_chroma_loc/out_chroma_loc (requires a supporting FFmpeg build). These are controls, including deliberate metadata overrides, not permissible production conversions. Low error against a control does not authorize changing source tags. Original CPU reference and CUDA candidate stay unchanged. New edge and moving impulse fixtures use constant luma and lossless HEVC encoding; normal CPU fixture checks run without a GPU, explicit physical qualification runs both fixtures and retains the existing SDR gate.

Optional -benchmark times the existing BGRA paths sequentially, three startup-inclusive repetitions per backend, alternating order, reports individual times and medians. Includes decode from source start, scaling, rotation, tone mapping, download where applicable, raw pipe and hash verification. Runs after fidelity comparison even if the quality gate is exceeded; not pure GPU kernel time or sustained playback throughput. All flags share the existing three-minute deadline. Use short frame windows if combining diagnostics.

```cmd
pixelcut gpu-preview-scale-check -chroma -width 540 -height 960 -frames 30 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -chroma -width 960 -height 540 -frames 30 "test-media\test-unrotated.mp4"
pixelcut gpu-preview-scale-check -benchmark -width 540 -height 960 -frames 120 "test-media\test.mp4"
pixelcut gpu-preview-scale-check -benchmark -width 960 -height 540 -frames 120 "test-media\4k-landscape.mp4"
set PIXELCUT_QUALIFY_CUDA_CHROMA=1
go test -count=1 -v ./internal/media -run "^TestPreviewQualificationChromaFixtures$"
set PIXELCUT_QUALIFY_CUDA_CHROMA=
```

4K command assumes a real unrotated 3840x2160 limited-range left-sited HEVC Main 10 HLG recording with at least 120 frames; substitute path or swap output dimensions for a portrait recording. 1080p/4K are source resolutions, not preview output sizes. No new activation, thresholds, schema or dependencies. Candidate CI and hardware runs pending.

Local command-level FFmpeg verification: both generated fixtures probe as Main 10, left-sited 10-bit 4:2:0, four frames. The impulse fixture produced four distinct raw SHA-256 results for the four named chroma-location controls, each 82,944 bytes (four 96x72 planar frames). This validates fixture and option execution on the local build only; candidate Go suite and CUDA remain untested locally. Ordinary CI exercises the explicit CPU location control when its FFmpeg advertises both named options; older builds still exercise baseline and timing.
