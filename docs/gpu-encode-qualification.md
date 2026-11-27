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
