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
