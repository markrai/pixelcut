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
