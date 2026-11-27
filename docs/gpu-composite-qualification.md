# 3A.4 HDR composition qualification

## Boundary and non-goals

3A.4a adds a test-only oracle matrix and candidate comparator next to the existing projectexport compositeHLG implementation. No new compositor, production dispatch, dependency, project schema or GPU code. Existing geometry is CPU-only; experimental CUDA decode remains independent. Do not conflate SDR preview compositing with HDR export. The input is normalized planar yuv444p10le, limited-range BT.2020 nonconstant-luminance HLG, integer rectangles and percent opacity, bottom-to-top ordering, up to four layers. The output is the same planar canvas before encoder chroma subsampling. Decode, scaling, tone mapping, encoding and audio are not measured by this harness.

## Qualification contract

- All pixels uncovered or covered first by an opaque layer must be exact, including source excursion codes. The fixture comparator derives the expected code from the topmost nonzero covering layer independently of the production compositor. Uncovered black is (64,512,512).
- Mixed pixels must remain legal limited-range 10-bit and differ by at most one code in each channel against compositeHLG (provisional candidate screen, not a physical HDR display claim). No tolerance for opaque or black pixels. Alpha ordering and single final quantization follow docs/project-export.md.
- The deterministic nine-case matrix covers gaps, odd rectangle placement, excursion bypass, 50% white analytic anchors, overlapping half-white layers, zero opacity, opaque over translucent, disjoint layers and four-layer gradients with 1%/99% opacity. Repeated CPU evaluation must be exact; the comparator's one-code mixed allowance is for future candidates, not CPU repeatability.
- Comparator rejects wrong frame lengths, exact-path deviations, out-of-range mixed samples and more than one-code mixed deviations. Future GPU tests must also verify actual transfer representation, device identity, cancellation/reaping, no reuse of stale output and resource ceilings.

## CPU baseline

```cmd
go test -count=1 -v ./internal/projectexport -run "^TestHLGQualification"
go test -run "^$" -bench "^BenchmarkHLGQualificationCPU$" -benchmem -benchtime=3x ./internal/projectexport
```

The benchmark compares two full-canvas opaque layers vs 100% bottom + 50% top at 640x360, 1920x1080 and 3840x2160, using deterministic 10-bit patterns. Input allocation and black initialization are outside timing; full opaque bottom ensures complete destination coverage on every iteration. Output byte rate is a normalization metric, not measured memory bandwidth. This is composition-only; no FFmpeg process startup, GPU transfers or encoder overhead. Record machine, Go version, repetitions and allocation measurements. Large 4K cases are opt-in benchmarks, not required ordinary tests. Candidate GPU timings must include transfers and setup separately and report comparable end-to-end cost.

3A.4b will choose and qualify a GPU candidate at this exact boundary. Do not enable a GPU compositor until exact-path and mixed-pixel checks, independent physical timing and integration gates pass. 3A.4c and 3A.4d remain pending.

## 3A.4b: isolated OpenCL candidate

PR #37 merged; CI 45931536401 and paired performance 45931536468 passed. CPU performance numbers from the physical machine have not yet been recorded. The new test-only candidate uses FFmpeg's program_opencl (https://ffmpeg.org/ffmpeg-filters.html#program_005fopencl) with an embedded OpenCL C kernel and an explicitly selected GPU device. No new Go module, native library linkage, production integration or external shader download. A private Go test temp directory holds the kernel source and is removed by testing cleanup. The FFmpeg process runs there, so filter arguments never include an interpolated absolute path.

Transport packs each Y/Cb/Cr code and integer opacity into four 16-bit lanes of rgba64le, used as opaque storage rather than RGB color. Four full-canvas rectangles are stacked vertically in one atlas; absent/transparent positions have opacity zero. No color conversion, chroma resampling or alpha normalization through FFmpeg. GPU image reads recover integer codes from UNORM16 and output a fixed marker lane of 100; Go checks size, marker and 10-bit code bounds before unpacking. The kernel computes topmost-first source-over using float32 HLG inverse/forward OETF and the same BT.2020 coefficients, with exact opaque and black paths. It emits one final quantized sample per channel. Compare against the original CPU float64 oracle using unchanged one-code mixed tolerance. The atlas is deliberately simple and bounded: at most 1920 pixels per side and 1920x1080 pixels total; four layers, 32 bytes of atlas per canvas pixel and 8 bytes per packed output pixel. 4K atlas support, more compact transport, persistent sessions and throughput are deferred to later qualification. Raw stdout and diagnostics capped, process deadline 30 seconds, suite deadline three minutes, no fallback.

```cmd
go test -count=1 -v ./internal/projectexport -run "^TestHLGOpenCLTransport$"
set PIXELCUT_QUALIFY_OPENCL_COMPOSITE=1
set PIXELCUT_OPENCL_DEVICE=0.0
go test -count=1 -v ./internal/projectexport -run "^TestHLGOpenCLQualification$"
set PIXELCUT_QUALIFY_OPENCL_COMPOSITE=
set PIXELCUT_OPENCL_DEVICE=
```

The selected FFmpeg build must provide OpenCL, program_opencl and rgba64le image upload/download; the driver must expose an OpenCL GPU. Choose the platform.device index using FFmpeg verbose device initialization logs; index 0.0 is not proof of NVIDIA selection. The test logs tool identity and startup-inclusive candidate time (packing, process launch, GPU work, transfer, unpacking), not kernel throughput or sustained performance. Device/driver runtime and physical results remain outstanding. 3A.4c should measure independent CPU/GPU timings including persistent setup and transfers, cancellation and resource use; do not enable compositor from CI or a single fixture. 3A.4d integration or deferral follows.

Local command-level verification: Clang OpenCL 1.2 syntax check passed for the kernel; local FFmpeg exposes program_opencl and its size option. Physical OpenCL initialization failed with no available OpenCL platforms (-1001), so the actual device transport and kernel output are not verified here. Full Go suite remains pending CI.

## 3A.4c: persistent strip qualification

PR #38 merged; CI 45934421879 and performance 45934421959 passed. No physical 3A.4b report or CPU benchmark has been supplied. The existing single-frame test remains mandatory. New opt-in finite persistent batches reuse one FFmpeg process and kernel across strips. The shader writes 100 + zero-based FFmpeg kernel frame index into the transport marker; the host rejects stale, reordered, truncated and extra strip outputs. The existing single-frame marker remains 100. See FFmpeg program_opencl source (https://ffmpeg.org/doxygen/trunk/vf__program__opencl_8c_source.html).

Two logical frames per resolution are processed in 120-row strips, 6/18/36 strips at 640x360, 1920x1080 and 3840x2160. The workload alternates two deterministic strip patterns, with full opaque background and inset opaque or 50% layer. It represents the same per-pixel operation count as two full frames but not arbitrary full-frame placement, frame residency, or real-video throughput. No cross-strip filtering occurs at this composition-only boundary. Input atlas is reused, output packed and planar buffers reused, no full 4K atlas or accumulated output frames. At 4K a strip atlas is 14,745,600 bytes and packed output 3,686,400 bytes; these are Go buffer sizes, not a measured process/GPU memory ceiling. Transport volume is still 32 bytes uploaded and 8 bytes downloaded per canvas pixel, plus CPU repacking and pipe copies. No claim of zero-copy. Each batch is bounded to 120 strips and 90 seconds, whole suite eight minutes; no fallback. Actual FFmpeg and GPU allocations remain to be measured.

Separate verification pass compares every strip against the unchanged oracle. Three independent CPU and GPU batches alternate order; timings exclude fixture generation and comparator work but include CPU hash or GPU packing, startup, transfer, unpack and hash. First strip latency includes process startup and buffering; it is not pure setup time. Timed output hashes must equal the verified GPU batch. CPU timing uses the same strip pattern and hash, not the full-canvas baseline benchmark. Active cancellation on first output and a fresh restart are tested physically. Ordinary CI exercises fragmented transport, marker and extra-output rejection, finite input, pre-cancel and limits. No production compositor changes.

```cmd
set PIXELCUT_OPENCL_DEVICE=0.0
set PIXELCUT_QUALIFY_OPENCL_COMPOSITE=1
go test -count=1 -v ./internal/projectexport -run "^TestHLGOpenCLQualification$"
set PIXELCUT_QUALIFY_OPENCL_COMPOSITE=
set PIXELCUT_QUALIFY_OPENCL_STREAM=1
go test -count=1 -v -timeout 10m ./internal/projectexport -run "^TestHLGOpenCLStreamQualification$"
set PIXELCUT_QUALIFY_OPENCL_STREAM=
set PIXELCUT_OPENCL_DEVICE=
```

Record actual device/driver and peak process/GPU memory separately. Do not infer physical pass or beneficial acceleration from CI. 3A.4d remains gated on both physical suites, resource review and meaningful measured end-to-end gain; defer if not worthwhile.

## 3A.4d: activation deferred

PR #39 merged at 44a4700643efdc0693ca8efda972e3cc74efafb2; CI 45935143312 and performance 45935143309 passed. Physical single-frame and persistent OpenCL reports, meaningful comparative performance and peak memory evidence have not been supplied. Production composition stays CPU-only. This is a reversible deferral, not a claim that OpenCL failed. Existing test harness and thresholds remain available. 3A.5 encoding proceeds independently.
