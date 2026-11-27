# 3A.5 hardware HEVC encode qualification

3A.5a is an isolated test candidate, not a product encoder option. The existing libx265 argument sequence is extracted into shared input/output helpers without changing the production command. The candidate replaces only encoder-specific options: hevc_nvenc, explicit GPU index, Main 10, p6/hq, constqp 16 and p010le encoder input. FFmpeg performs the existing CPU zscale conversion from limited-range 10-bit HLG 4:4:4 to left-sited 4:2:0 before the encoder-specific format conversion. Both paths retain hvc1 MP4, BT.2020/HLG/nonconstant matrix, limited range, left chroma and exact 240000 Hz track/movie timebase. No automatic backend fallback. No claim that constqp 16 equals x265 CRF 16 in bitrate or perceptual quality.

The fixture emits twelve temporally distinct flat-color frames with known 10-bit Y/Cb/Cr values. Verify existing export contract (including Main 10 profile, orientation, frame count, duration and zero start) and exact packet grid via existing production verifier. Independently software-decode output as planar 10-bit 4:2:0, check every sample against the analytically known flat input; reject missing/extra decoded frames, codes outside 10-bit and any per-plane maximum error above 12 or minimum per-frame PSNR below 40 dB (provisional flat-field smoke gate only). No claim of general lossy quality or spatial/chroma edge parity. The exact packet-grid gate is not relaxed. Raw frame memory bounded to one frame at each stage; per-test temporary MP4 cleanup. CPU-only synthetic reference runs in ordinary CI when FFmpeg and FFprobe are available. Physical suite uses the same CPU and NVENC path for 360p/24, 1080p/30, portrait 1080p/60 and 4K/30. Log resolved executable and versions, chosen device index, file size and elapsed feed+encode+finalization time. This includes raw fixture generation and process setup, not sustained encoding or transfer-only throughput. Device index must be independently confirmed with driver tools.

```cmd
go test -count=1 -v ./internal/projectexport -run "^Test(NVENCQualificationArguments|EncoderFixturePlane|HLGEncoderQualification)$"
set PIXELCUT_QUALIFY_NVENC=1
set PIXELCUT_NVENC_DEVICE=0
go test -count=1 -v -timeout 6m ./internal/projectexport -run "^TestHLGEncoderQualification$"
set PIXELCUT_QUALIFY_NVENC=
set PIXELCUT_NVENC_DEVICE=
```

Requires FFmpeg with hevc_nvenc and a compatible NVIDIA driver for the opt-in path. Explicit failure if unavailable or metadata/packet/quality verification fails. No publishing or project mutation. Next: diverse spatial and real-media content, bitrate/quality comparison at comparable quality, longer independent timing and memory tests before experimental integration. CPU encoder remains default and only production encoder.

## 3A.5b: spatial, motion and real-source rate/quality comparison

PR #40 merged; CI 46007589621 and performance 46007589542 passed. PR #41 static HDR import fix also merged (CI 46039697815 and performance 46039697772 passed). Physical 3A.5a results have not been supplied.

This test-only comparison isolates the encoder after the production 4:4:4-to-left-sited-4:2:0 normalization boundary. It creates a finite raw planar 10-bit reference once per case (maximum 1 GiB), then passes exactly that file to both encoders. Synthetic testsrc2 is a moving spatial/color-edge pattern, not physically captured HDR. Optional real HLG uses software decoding, explicit source range/chroma normalization to 4:4:4, cardinal rotation and the shared output subsampling graph; no source modifications. The first consecutive decoded frames are re-clocked at 30 fps for this encoder experiment; this is not source VFR or timeline timing qualification. Reference generation excluded from encode timing. Each run verifies the existing output color, Main 10, frame count, duration, zero start and exact packet grid; independently software-decodes and compares every planar sample to the same reference. Report maximum code errors and both worst-frame and pooled MSE PSNR per Y/U/V plane (pooled is not the arithmetic mean of frame PSNRs). Reject wrong counts, extra data and invalid 10-bit codes.

x265 CRF 16 remains baseline. NVENC p6/hq constqp sweeps 12/16/20/24. Select the smallest MP4 among candidates within 1 dB below CPU worst-frame AND pooled PSNR in every plane. This is a provisional relative screen, not perceptual equivalence or an absolute lossy quality gate. Report all tested sizes, errors and timings; no passing candidate fails that case. Selected QP is local to a case, not a production setting. Three independent alternating CPU/selected GPU runs reverify output; report median wall speedup and sweep size ratio. Timing includes disk feed, process initialization, format conversion, transfer, encode and MP4 finalization, excludes comparison and fixture creation. Container bitrate includes overhead; neither pure kernel speed nor sustained long-project throughput. The flat-field gate in 3A.5a remains unchanged.

Physical matrix: 120 frames 360p, 90 frames 1080p, 40 frames 4K, optionally up to 60 real-source frames subject to the same byte cap. Reference plus temporary output in Go test temp directory, no publishing. At 4K the reference alone is about 950 MiB. Disk throughput/cache can affect timing; record peak process and GPU memory, device/driver separately. One overall 15-minute deadline, no fallback. No production integration yet. Ordinary CI runs a small CPU smoke and metric/argument checks.

```cmd
go test -count=1 -v ./internal/projectexport -run "^Test(EncoderMotionMetric|HLGEncoderMotionQualification)$"
set PIXELCUT_QUALIFY_NVENC_MOTION=1
set PIXELCUT_NVENC_DEVICE=0
set "PIXELCUT_NVENC_MOTION_SOURCE=C:\dev\project\pixelcut\test-media\test.mp4"
go test -count=1 -v -timeout 17m ./internal/projectexport -run "^TestHLGEncoderMotionQualification$"
set PIXELCUT_QUALIFY_NVENC_MOTION=
set PIXELCUT_NVENC_DEVICE=
set PIXELCUT_NVENC_MOTION_SOURCE=
```

Run the preceding 3A.5a physical test as well. 3A.5c remains gated on physical color/packet/quality results, representative quality/size tradeoffs and a useful performance benefit, with longer workload/resource review before enabling any experimental encoder.

Local FFmpeg 7.1.5 command-level CPU smoke produced exactly 110592 reference bytes (12 frames at 64x48), HEVC Main 10 HLG limited-range left-sited output, 12 packets with unique 8000-tick intervals, timebase 1/240000, zero start and duration 96000 ticks. Full Go checkout failed DNS resolution; CI and actual NVENC comparison remain unverified locally.

## 3A.5b.1: correct raw input color contract before interpreting quality

PR #42 merged at 6e93e39631d64d38005ca6c9019eb8f8d20cf3c2; CI 46041018160 and performance 46041018257 passed. Physical TITAN (FFmpeg N-129308-g17de9f8597-20261126, device index 0) flat-field suite passed at all four resolutions/rates, NVENC max Y/U/V 0/1/0. Motion suite passed its original comparative screen but is not valid encoding-only fidelity evidence: synthetic worst-frame luma PSNR approximately 25 dB for both encoders, almost invariant across QPs. Preliminary median speedups 1.563x/1.520x/1.933x/3.834x for 360p/1080p/4K/Pixel portrait, selected QP 24/24/24/16, corresponding output size ratios 0.568/0.527/0.550/1.371. These numbers are historical, not authorization to select QP or integrate.

Local FFmpeg 7.1.5 reproduced the defect with the same synthetic 64x48 reference and x265 lossless control. Verbose graph inserted auto_scale_0 between setsar and format, converting unknown matrix/range to BT.2020 nonconstant-luminance/limited via intermediate RGB. Untagged lossless decode differed from reference (maximum Y/U/V 81/115/81, pooled PSNR approximately 30.72/31.59/32.85 dB); adding explicit input color options before -i gave byte-identical decoded output (SHA-256 match). Rawvideo files carry no color metadata. Correct both CPU and NVENC motion input arguments with explicit BT.2020, HLG, limited range and left chroma; the output contract stays unchanged. Add an x265 lossless control for each case, including ordinary CPU smoke, requiring exact decoded Y/U/V and existing packet/metadata checks before any lossy comparison. Lossless is a test control only, not a new product setting. This guards the shared CPU raw input boundary, not an assertion of exact hardware P010 conversion. The comparison still requires actual NVENC results on the updated harness. No thresholds weakened, no production encoder changes. Re-run the 3A.5b commands above after merge.

## 3A.5c.1: private project encoder integration, public activation gated

PR #43 merged at 6c184cb5b8f1445939cb7cf8276b8c3df0b84c69; CI 46047281893 and performance 46047281919 passed. Corrected physical 3A.5b.1 results are not recorded. Keep all public export entry points on the exact existing x265 command; no GUI or CLI option or environment-variable product selection. A private encoder specification chooses CPU (zero value) or NVENC with explicit bounded device/QP. Both qualification helpers and the real render pipeline use the shared encoder command boundary. The private path uses existing composition, progress, staged audio mux, metadata and exact timing verification, source revalidation and no-overwrite publication. No fallback. Report the candidate in warnings and encoder errors. Only test code calls the NVENC branch.

New opt-in physical integration test creates a small HLG fixture with audio, gap, transformed translucent overlap, independent audio and CFR tail. Render via CPU and qualification NVENC with the same project and CPU decoding; check progress, output contract, packet timing, exact decoded AAC equality, analytic luma anchors, and minimum per-frame/channel 40 dB CPU-vs-GPU PSNR (integration smoke only, not a source-relative rate/quality test). Report maximum deltas and startup-inclusive project time, verify original source and no leaked stage files. QP 16 is a provisional test default, not a production decision; override explicitly for experimentation. Ordinary CPU tests verify unchanged argument selection and reject invalid options. No production activation until corrected motion quality, physical integration, longer workload and resource evidence are available. 3A.5d fault/cancellation/no-overwrite qualification remains separate.

```cmd
set PIXELCUT_QUALIFY_NVENC_PROJECT=1
set PIXELCUT_NVENC_DEVICE=0
set PIXELCUT_NVENC_PROJECT_QP=16
go test -count=1 -v -timeout 3m ./internal/projectexport -run "^TestProjectNVENCIntegration$"
set PIXELCUT_QUALIFY_NVENC_PROJECT=
set PIXELCUT_NVENC_DEVICE=
set PIXELCUT_NVENC_PROJECT_QP=
```

## 3A.5d: runtime safety and longer-project qualification

PR #44 merged at 38290445f095a0f7682cecab4675cddb4ea3edd2; CI 46052915596 and performance 46052915527 passed. TITAN corrected motion results (FFmpeg N-129308-g17de9f8597-20261126, GPU index 0): all five lossless controls exact, all four motion workloads selected QP 16. Median encoder-stage speedups 2.645x/3.629x/4.029x/8.223x for 360p/1080p/4K/real Pixel portrait. Corresponding size ratios 1.150/1.070/1.110/1.368. Real-source pooled PSNR Y/U/V CPU 46.985/53.221/55.795 dB; NVENC 48.335/54.017/56.417 dB. Physical private project integration passed: 17 frames, 27200 audio samples, identical decoded AAC, minimum PSNR 69.980/100/100 dB, maximum deltas 8/0/0, CPU/NVENC wall 387.683/563.165 ms, output 16442/14214 bytes. Tiny-project startup cost is not a long-export performance result.

Runtime safety: ordinary CI exercises CPU, optional physical test adds NVENC QP16. Cancellation at preflight, after a submitted video frame, video finalization, after submitted audio, audio finalization, verification and immediately before publication. Existing/late collision, corruption of staged video and final staged mux before verification, source mutation of a disposable copy before publish, and known-unavailable NVENC device (default index 31, override on a multi-GPU machine). Every failure must return an error without StageDone, preserve existing output where applicable, and leave no staged files. Fresh verified export after faults demonstrates recovery. Cancellation is tested at progress boundaries, not arbitrary driver hangs or the atomic hard-link commit itself. Preserve original error plus encoder process exit/stderr after abort/reap on span failure; cancellation returns the parent context error on video span/finalization. No fallback or new production selection.

Long physical test requires a supported 10-60 second HLG source with audio, uses first 10 seconds at native display resolution with a leading 100ms gap (303 output frames at 30 fps). Same CPU decode/composition/audio and production verification; three alternating full-project runs per encoder, output bytes and median throughput, sampled Go heap start/peak during progress (not total process working set or GPU memory). Last pair independently software-decodes every frame with two bounded frame buffers, logs comparative PSNR and maximum code errors as diagnostic (no new perceptual threshold), requires exact decoded audio SHA256 equality, source SHA256 unchanged and stage cleanup. Timings include all project rendering and publication, not later comparison. Whole-test deadline 8 minutes. No 4K claim unless tested with a 4K source. Capture device/driver and peak host/GPU resource evidence separately; Go heap is not FFmpeg/VRAM usage. During the long test, a second CMD can record GPU samples with nvidia-smi; stop it with Ctrl+C after the run. GPU memory/utilization are device-wide, not attributable to PixelCut without isolation. Record test process/FFmpeg peak working sets with OS monitoring; absence of that evidence remains a gate before public activation.

```cmd
set PIXELCUT_NVENC_DEVICE=0
set PIXELCUT_NVENC_PROJECT_QP=16
set PIXELCUT_QUALIFY_NVENC_SAFETY=1
go test -count=1 -v -timeout 5m ./internal/projectexport -run "^TestProjectEncoderRuntimeSafety$"
set PIXELCUT_QUALIFY_NVENC_SAFETY=
set PIXELCUT_QUALIFY_NVENC_LONG=1
set "PIXELCUT_NVENC_LONG_SOURCE=C:\dev\project\pixelcut\test-media\test.mp4"
go test -count=1 -v -timeout 9m ./internal/projectexport -run "^TestProjectNVENCLongQualification$"
set PIXELCUT_QUALIFY_NVENC_LONG=
set PIXELCUT_NVENC_LONG_SOURCE=
set PIXELCUT_NVENC_DEVICE=
set PIXELCUT_NVENC_PROJECT_QP=
```

```cmd
nvidia-smi --query-gpu=timestamp,index,name,driver_version,memory.used,utilization.gpu,utilization.encoder --format=csv -lms 500 -f nvenc-project-gpu.csv
```

Public activation remains gated on physical runtime, longer export performance, quality review and resource evidence; QP16 is the provisional candidate only.

## 3A.5c.2: public experimental selection

PR #45 merged at 0012f88e8fd3ea44825803eb587a7d1daccca8b5; CI 46057390415 and performance 46057390355 passed. TITAN runtime safety PASS (25 CPU/NVENC subtests, including device 31 unavailable with NVENC diagnostic, no staging leaks and successful recovery). Longer real Pixel portrait project (303 frames, 484800 audio samples, source SHA256 83723ff5b2896315b557cde36db0ddd99c23b0cc83e4dfa60129927182a49466) passed twice: nightly N-129308 CPU/NVENC median 36.715/12.202s (3.009x); Gyan 9.1 CPU/NVENC 36.838/11.979s (3.075x). Both reports: NVENC 20356457 bytes, CPU approximately 14978080 bytes, identical decoded audio SHA256 and CPU-vs-NVENC worst-frame PSNR 45.539/52.611/55.074 dB, pooled 47.408/53.984/56.509 dB, maximum deltas 147/64/52. Sampled Go heap about 41 MiB both. These are diagnostic CPU-relative lossy comparisons and one real workload, not a general perceptual guarantee. Peak host/VRAM measurements not supplied.

Decision: allow explicit opt-in experimental public export on the functional and performance evidence while keeping CPU default and no automatic selection. This is a narrower acceptance than default/general release qualification; outstanding resource evidence remains a gate for broader qualification. Public engine policy uses fixed device 0 and QP16; no public custom QP/device or fallback. CLI -encode cpu|nvenc on export-project/export-video, GUI checked Settings entries with experimental confirmation and encoder shown in export confirmation/result. Encoder-only preference does not alter preview engine. Existing project schema and selected-clip export unchanged. Default physical integration test now goes through public RenderProjectObserved at device0/QP16, other qualification configurations stay private. Re-run integration and manual Windows export, restart/reset preferences, independent decode selection, cancellation and unavailable GPU failure; compare public CLI CPU and NVENC output. 3A.6 retains combined CUDA/NVENC and resource qualification.
