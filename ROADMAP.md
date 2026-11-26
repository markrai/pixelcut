# PixelCut roadmap

## Status definitions

- Pending: no implementation started.
- Implemented: changes committed; automated verification may be outstanding.
- CI passed: the specific commit passed automated checks.
- Runtime qualified: the documented physical acceptance procedure was run and
  its evidence recorded. This is not implied by CI.
- Deferred: deliberately moved out of the active milestone.

## Current product status

Baseline revision: `94f0402a59aa543beb0383e40151e145bea01ec6`
([CI passed](https://github.com/markrai/pixelcut/actions/runs/45683622460)).
The previous single-track editor, lifecycle, relinking, caching and settings
features are implemented. 2C.1 supports multiple stored tracks and active-track
editing, not simultaneous composition or independent audio playback. Native
Windows acceptance for these recent slices is not recorded here.

| Phase | Scope | Status |
| --- | --- | --- |
| 2B.1–2B.3 | Preview, transformations, timeline interactions | Implemented; CI passed |
| 2B.4.1 | Project lifecycle | Implemented; CI passed |
| 2B.4.2 | Per-project recovery and backups | Deferred |
| 2B.4.3 | Missing media and relinking | Implemented; CI passed |
| 2B.4.4 | Persistent caching | Implemented; CI passed |
| 2B.4.5 | Preferences and gap-start playback | Implemented; CI passed |
| 2B.4.6 | Project portability | Deferred |
| 2C.1 | Independent track model and active-track editing | Implemented; CI passed |

## 2C.R: Architectural stabilization

New product features require stabilization acceptance before merge/release. 2C.2 was merged at the user's direction after CI; physical qualification remains outstanding. 2C.3 and 2C.4 passed automated CI and are merged. 2C.5 is merged with automated CI and paired performance qualification passed; native qualification remains outstanding.
Each slice must independently preserve existing functionality and project/recovery
compatibility. Ownership rules are in [ARCHITECTURE.md](ARCHITECTURE.md).
The next slice cannot be accepted until its predecessor's verification is recorded.

| Phase | Deliverable | Exit evidence | Status |
| --- | --- | --- | --- |
| 2C.R.1 | Baseline, architecture, formatting, CI, fixtures, benchmarks | Format/vet/race/tests, coverage and benchmark reports | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45691058409) |
| 2C.R.2 | Model identity, time/intervals, validation, clone and schema contracts | Cross-entry invariants, compatibility and failure tests | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45696335973) |
| 2C.R.3 | Platform-independent editor and stable selection references | Selection, deletion and track switching tests without Win32 | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45704971859) |
| 2C.R.4 | Unified transactions, typed commands and history | Atomic failure/no-op tests, one history entry per edit | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45706103738) |
| 2C.R.5 | Pure timeline geometry, hit testing and gestures | Shared geometry for draw/input/drop, snapping regression tests | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45711670944) |
| 2C.R.6 | Deterministic composition evaluator | Video/audio contribution and time mapping tests independent of selection | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45712233364) |
| 2C.R.7 | Project playback controller, separate mpv backend | Simulated backend state/seek/boundary/stale event tests | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45713898037) |
| 2C.R.8 | Jobs, source monitoring, recovery and resource ownership | Canceled/stale results rejected, bounded shutdown, no periodic slow UI I/O | Implemented; verification pending; explicit lifecycle I/O remains |
| 2C.R.9 | Regression and performance qualification | Baseline comparison, real Windows and media fidelity acceptance | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45716772873); [paired benchmarks completed](https://github.com/markrai/pixelcut/actions/runs/45716773218); native acceptance and performance review pending |

Keep snapshot undo until measurements justify a replacement. Preserve the current
preview capabilities during extraction; a project clock alone is not a compositor.
Record deviations and unresolved qualification items rather than claiming completion
from a build alone.

## Feature development after stabilization

| Phase | Scope | Status |
| --- | --- | --- |
| 2C.2 | Stacked lanes and cross-track dragging | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45722841420); merged; native acceptance pending |
| 2C.3 | Independent audio playback, detach/relink | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45725905248); merged; native acceptance pending |
| 2C.4 | Track visibility, mute, solo, lock | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45729406556); merged; native acceptance pending |
| 2C.5 | Layering, opacity and picture-in-picture | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45732847195); merged; native acceptance pending |
| 2C.6 | Composite preview and continuous timeline playback | In progress; 2C.6.1–2C.6.6 merged to main via PR #11; native qualification remains open |
| 2D | Whole-project export and explicit output color contracts | 2D.1–2D.6 merged; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45843280827); physical acceptance pending |
| 2E | Audio envelopes, fades, levels and mixing | 2E.1–2E.6 merged and CI passed; physical acceptance pending |
| 3A | GPU acceleration | In progress; 3A.1 merged and CI passed; 3A.2 decode-check merged; 240/240 physical frame parity on user 1080p source; experimental renderer integration merged; CI passed; physical acceptance pending |
| 3B | Transitions, keyframes, speed effects, titles and subtitles | Pending |
| 4A | Packaging and distribution | Pending |

## 2C.6 staged delivery

| Slice | Deliverable | Status |
| --- | --- | --- |
| 2C.6.1 | Timestamped still decoder, bounded source/frame cache | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45742732607); merged to main |
| 2C.6.2 | Static SDR composite, geometry/opacity and Windows preview | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45742732607); merged to main; native acceptance pending |
| 2C.6.3 | Continuous multi-layer video playback with persistent decoding | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45808454654); merged to main via PR #7; native acceptance pending |
| 2C.6.4 | Audio synchronization, elementary mixing and master clock | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45811817941); merged to main via PR #7; native acceptance pending |
| 2C.6.5 | Integrated seeking, scrubbing, gaps and clip boundaries | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45813556936); merged to main via PR #11; native acceptance pending |
| 2C.6.6 | Performance, cancellation/resource management and HDR display fidelity qualification | Cache/lifecycle improvements, diagnostics, synthetic tests and [qualification matrix](docs/composite-qualification.md) implemented; CI, performance review and physical Windows/HDR acceptance pending |

2B.4.2 and 2B.4.6 will be scheduled separately. No release dates are assumed.

## Slice acceptance

- Go formatting, tests, vet, portable race checks and Windows builds pass.
- New regression tests cover the slice's changed ownership and behavior.
- Existing project and recovery formats remain readable.
- No undocumented changes to editing or media fidelity.
- Update architecture and roadmap with the exact verification evidence.
- Physical runtime acceptance is recorded separately, with tool versions and
  test environment where applicable.


## 2D staged delivery

| Slice | Scope | Status |
| --- | --- | --- |
| 2D.1 | Explicit HLG output contract, immutable timeline plan, source preflight and CLI inspection | Merged to main via PR #12 |
| 2D.2 | Native 10-bit HLG opaque video rendering, geometry, gaps, CFR and verified publication | Merged to main via PR #13; physical qualification pending |
| 2D.3 | Multi-layer HDR composition with explicit linear-light blending and alpha, full range/chroma normalization qualification | Merged to main via PR #14; physical qualification pending |
| 2D.4 | Offline audio mix, AAC mux, exact output frame/sample duration verification | Merged to main via PR #15; physical qualification pending |
| 2D.5 | CLI and Windows export orchestration, progress, cancellation and destination safety | Merged to main via PR #16; physical Windows acceptance pending |
| 2D.6 | Synthetic/reference and real Pixel HDR qualification, bounded resource/performance review | Merged via PR #17; CI passed; physical acceptance pending |

See [project export contract](docs/project-export.md). Whole-project HDR video and audio export is available from the Windows Export menu in 2D.5. The existing selected-clip HLG export remains independent.


2D.6 acceptance is tracked in [HDR export qualification](docs/export-qualification.md). Do not mark 2D fully qualified until the required CI and physical Windows/Pixel evidence exists. Phase 2E audio work can be planned independently; changes to the export path must preserve the qualification gates.


## 2E staged delivery

| Slice | Deliverable | Status |
| --- | --- | --- |
| 2E.1 | Versioned per-clip gain model, shared fixed-point PCM mixer, compatibility and parity tests | Merged via PR #18; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45846574383) |
| 2E.2 | Per-clip volume and reversible mute controls, native audition behavior | Merged via PR #19; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45853835545); native acceptance pending |
| 2E.3 | Fade-in and fade-out, timeline edit semantics | Merged via PR #20; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45855331369); native acceptance pending |
| 2E.4 | Volume envelopes and draggable keyframes | Merged via PR #21; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45858633368); native acceptance pending |
| 2E.5 | Audio level meters and clipping indicators | Merged via PR #22; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45860773291); native acceptance pending |
| 2E.6 | Audio preview/export parity, performance and physical qualification | Merged via PR #23; [CI passed](https://github.com/markrai/pixelcut/actions/runs/45871806630); physical acceptance pending |

See [audio processing contract](docs/audio-processing.md). The shared mixer consumes decoded PCM; decoder timing and lossy AAC output remain separate qualification issues. 2D physical acceptance remains open.

2E.6 acceptance is tracked in [audio qualification](docs/audio-qualification.md). The canonical decoder deliberately trades seek-first latency for exact source-sample mapping. Do not mark audio physically qualified until the Windows audio-device, long-source seek, sustained-load and Pixel-media gates are recorded. Phase 3A GPU planning may proceed independently, but must preserve these contracts.

## 3A staged delivery

| Slice | Scope | Status |
| --- | --- | --- |
| 3A.1 | Bounded capability inventory, optional device initialization and CPU-first stage policy | Merged via PR #24; CI passed; Windows CUDA/D3D11VA/D3D12VA/Vulkan initialization observed |
| 3A.2 | Hardware HEVC decode, exact frame selection and CPU-readable 10-bit parity | PR #27 merged; CI passed; user 240/240 frame parity; experimental renderer integration merged via PR #28; CI and performance passed; physical acceptance pending |
| 3A.3 | GPU geometry, scaling and transfer boundaries | 3A.3a merged via PR #29; CI and performance passed; user 240/240 nearest-neighbor frame parity at 1080p to 540p. 3A.3b merged via PR #30; CI and performance passed; real source rejected before comparison, synthetic GPU quality failed max-delta gate (19 > 16); diagnostic follow-up implemented; activation and remaining geometry pending |
| 3A.4 | GPU layer compositing with explicit HLG linear-light contract | Pending |
| 3A.5 | Hardware HEVC Main 10 encode with verified color/timing/publication | Pending |
| 3A.6 | GPU/CPU parity, fallback, performance and physical qualification | Pending |

See [GPU architecture and qualification](docs/gpu-architecture.md). Capability advertisement, successful device initialization and qualified rendering are independent gates. The CPU reference remains mandatory and no GPU stage is selectable in 3A.1.

See [CUDA decode qualification](docs/gpu-decode-qualification.md). Ordinary preview/export defaults to CPU; explicit experimental CUDA mode applies to composite preview and whole-project export. [CUDA geometry qualification](docs/gpu-geometry-qualification.md) begins with an explicit scale check. Product geometry remains CPU-only.
