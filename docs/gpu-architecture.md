# GPU architecture and qualification

## 3A.1 status

CPU rendering is unchanged. GPU diagnostics are implemented; candidate CI and physical Windows acceptance are pending. No GPU rendering backend is qualified or enabled. Phase 2E.6 merged via PR #23 and passed both workflows (45871806630, 45871806823); physical audio and HDR gates remain open.

## Capability layers

1. Inventory: query the configured FFmpeg executable for version, hardware methods, supported hardware device types, decoders, encoders and filters. Keep a bounded allowlist of names relevant to HEVC and GPU processing. Unknown entries are intentionally omitted. Inventory errors fail the report instead of pretending missing support.
2. Initialization: optional synthetic 64x64 one-frame null-output process with a named default device. Report advertised, initialized or initialization-failed. Currently attempt CUDA, D3D11VA, QSV, VAAPI, Vulkan, D3D12VA and VideoToolbox only if advertised as device types. Other types stay advertised. Failure does not prevent reporting other devices unless the overall context expires. This does not prove hardware decode, transfer, filtering, encoding, adapter selection, physical GPU rather than software device, or 10-bit HDR support.
3. Qualification: PixelCut stage policy is independent of inventory and initialization. Auto selects CPU, explicit CPU selects CPU, every other backend is rejected as unqualified. Do not infer compatibility from names, vendor, hardware marketing, a successful init or a filter listing. No GPU path is invoked by existing rendering operations.

The CLI gpu-info command emits structured JSON. Optional -probe-devices attempts initialization; no source file or project required. GUI Settings opens a copyable report via the existing bounded background job and stops preview first. The configured FFmpeg executable is used; CLI uses normal PATH discovery. No automatic launch-time probe or persisted machine-dependent state.

Each invocation is bounded by 45 seconds overall, five seconds per command, a one-second Go WaitDelay, and 256 KiB captured output per command. Device failure detail is capped to 512 bytes. A cancellation or overall timeout fails the report rather than authorizing a partial result. No temporary media files are created. No shell interpolation.

## Stage boundaries and invariants

| Stage | Reference implementation | Future qualification |
| --- | --- | --- |
| Decode | FFmpeg software decode, exact frame-index ordinal selection | HEVC Main 10 support, normalized rotation, exact frame identity across VFR and seeks, 10-bit CPU-visible output; cancellation and source checks |
| Geometry | Existing CPU/FFmpeg filters and layer plan | Same crop, rotation, flip, scaling, placement, chroma/range and output extent; transfer costs |
| Composite | SDR diagnostic blend or HLG linear-light CPU reference | Separate SDR/HDR contracts; HLG inverse/OETF and source-over alpha, black background, precision and tolerance against reference |
| Encode | libx265 Main 10 HLG, hvc1 MP4 | Exact CFR PTS/duration, color tags, range, chroma, profile, orientation, audio mux and verified publication; lossy image comparison |
| Audio | Canonical 48 kHz PCM and shared mixer | Unchanged sample clock, source mapping, automation, clipping and AAC presentation contract |

Treat hardware surfaces as owned resources with explicit lifetime and memory domain. Do not let a GPU surface escape a canceled run, or share an unsafe context across jobs. A stage's public semantic output must match the reference; resource transfer and format conversion are part of its cost. Keep platform-specific backend implementation behind the media boundary, not in timeline or project state. Avoid a general-purpose GPU graph until real stage requirements justify it.

A future fallback must be observable and deterministic: only a recognized backend availability/qualification failure permits CPU retry, after retiring the failed stage and discarding partial output. Never retry canceled work, source changes, invalid projects, or destination collisions as GPU failures. Never splice hardware/CPU output mid-published export. Preserve no-overwrite verified publication. CPU remains available, and explicit forced GPU requests must fail with a reason rather than silently changing the selected backend.

## Evidence and acceptance

Automated: parser/allowlist and duplicate handling, advertised versus initialized/failure separation, unqualified stage rejection, malformed inventory, cancellation and bounded capture, real FFmpeg inventory smoke test, CLI argument and Windows Settings action tests. Linux CI installs FFmpeg; actual device initialization is not a required CI condition. Windows native dialog and driver behavior are physical gates.

Local command-level evidence (FFmpeg 7.1.5, Linux amd64): advertised CUDA, QSV, VAAPI, Vulkan plus other methods; HEVC encoders included NVENC, QSV and VAAPI. CUDA initialization failed because libcuda.so.1 could not load; QSV/VAAPI initialization failed with an external-library error; Vulkan failed with VK_ERROR_INCOMPATIBLE_DRIVER. This illustrates why advertised capabilities must not be treated as usable hardware. The local environment has no qualified GPU. Repository checkout was blocked by DNS, so full candidate suite is pending CI.

Physical 3A.1: run gpu-info with and without device initialization on Windows with an NVIDIA GPU and, if available, Intel/AMD or GPU-less system. Record tool versions, driver versions, GPU model, output JSON and dialog behavior; verify source preview/export remains CPU and no project mutation or unexpected probe at startup. Initialization failure is a valid diagnostic outcome, not a rendering failure. Subsequent slices must add actual stage-specific fixtures, exact timing/color assertions and performance comparisons before enabling GPU execution.

### Windows diagnostic observation

User-reported Windows amd64 diagnostic from FFmpeg N-123778-g3b55818764-20260331: CUDA, D3D11VA, D3D12VA and Vulkan initialized; QSV failed to find an Intel adapter and VAAPI initialization failed. HEVC NVENC and CUVID were advertised. This is evidence of device initialization, not actual decode or encode qualification. The empty GPU filter inventory exposed a parser compatibility issue: FFmpeg 8.1-era filter listings use two flags instead of three. The parser now accepts both formats, including Windows CRLF, with regression coverage. CPU policy is unchanged.

## 3A.2 renderer integration

PR #26 and #27 passed both workflows and merged. The user reported 240/240 exact raw frame matches on a 1920x1080 HLG source. Experimental explicit CUDA decode is now selectable for composite stills, persistent composite streams and whole-project export. Engine snapshots carry a typed decode preference; omitted/CPU and Automatic use the CPU reference, explicit CUDA uses native HEVC hardware frames, P010 download, planar 10-bit conversion, then the existing CPU processing graph. CUDA preview uses a strict source frame index and origin-based ordinal selection rather than approximate input seek; explicit cardinal source rotation follows download. Export retains strict ordinals and existing layer normalization/rotation. This can increase late-seek/indexing latency. No automatic fallback or GPU-resident geometry yet; mpv/source audition, selected-clip export, thumbnails and audio unchanged. The setting is optional in version-1 app preferences, not project schema. Existing job retirement prevents reusing stale preview output across mode changes. Diagnostic stage decisions now identify explicit experimental CUDA separately from advertised capabilities. Physical qualification of integrated processing and 4K/portrait/VFR sources remains pending. See docs/gpu-decode-qualification.md.

## 3A.3a scaling and transfer qualification

PR #28 merged at 5e8da14b957fa004a311ca0d755485120416f223; CI 45882080825 and paired performance 45882080659 passed. Integrated CUDA decode physical acceptance remains open. The first geometry slice adds an isolated CUDA scale qualification command that consumes hardware frames and downloads at the requested output dimensions. The existing bounded frame-hash, exact timestamp and source identity checks are reused. Nearest-neighbor is deliberately a narrow test contract, not the production geometry resampler. No product geometry mode is enabled, and the stage policy still rejects explicit CUDA geometry. See [GPU geometry qualification](gpu-geometry-qualification.md).

## 3A.3b preview scale candidate

PR #29 merged at 41d3ec06ff2c2b0ab7f3008b2d4905507739bf21; CI 45885116645 and performance 45885116756 passed. User reported 240/240 exact nearest-neighbor frame matches. The next candidate moves only a simple aspect-preserving preview downscale onto CUDA using Lanczos; the existing CPU tone map remains shared. Still and stream decoder integration has an internal qualification-only switch. Ordinary CPU and experimental CUDA decode behavior remain unchanged. No candidate geometry selected by the product stage policy. Qualification is against displayed SDR pixels, not HDR export precision.

## 3A.3g geometry decision and 3A.4a composition boundary

PR #36 merged at b70b7d9a23c4cdd181157d16fe6b1b2459701766; CI 45927059779 and performance 45927059761 passed. No physical hybrid/Vulkan reports are recorded. Production geometry remains CPU-only; activation is deferred, not a claim that Vulkan failed. Existing explicit experimental CUDA decode remains independent. No automatic backend preference or new fallback.

The 3A.4 candidate boundary begins with normalized limited-range BT.2020 HLG planar 10-bit 4:4:4 layer rectangles in CPU memory, with integer placement and opacity. It ends with a CPU-visible frame of the same representation before encoder chroma subsampling. Test the HDR compositor independently of decoding, geometry, tone mapping, encoding and audio. The CPU compositor remains the oracle; exact black/opaque pass-through (including excursion values), source-over ordering and a provisional one-code maximum error on mixed pixels are separate gates. See [HDR composition qualification](gpu-composite-qualification.md).
