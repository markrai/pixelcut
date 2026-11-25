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

No new product features until this gate closes, except regression fixes.
Each slice must independently preserve existing functionality and project/recovery
compatibility. Ownership rules are in [ARCHITECTURE.md](ARCHITECTURE.md).
The next slice cannot be accepted until its predecessor's verification is recorded.

| Phase | Deliverable | Exit evidence | Status |
| --- | --- | --- | --- |
| 2C.R.1 | Baseline, architecture, formatting, CI, fixtures, benchmarks | Format/vet/race/tests, coverage and benchmark reports | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45691058409) |
| 2C.R.2 | Model identity, time/intervals, validation, clone and schema contracts | Cross-entry invariants, compatibility and failure tests | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45696335973) |
| 2C.R.3 | Platform-independent editor and stable selection references | Selection, deletion and track switching tests without Win32 | [CI passed](https://github.com/markrai/pixelcut/actions/runs/45704971859) |
| 2C.R.4 | Unified transactions, typed commands and history | Atomic failure/no-op tests, one history entry per edit | Implemented; verification pending |
| 2C.R.5 | Pure timeline geometry, hit testing and gestures | Shared geometry for draw/input/drop, snapping regression tests | Pending |
| 2C.R.6 | Deterministic composition evaluator | Video/audio contribution and time mapping tests independent of selection | Pending |
| 2C.R.7 | Project playback controller, separate mpv backend | Simulated backend state/seek/boundary/stale event tests | Pending |
| 2C.R.8 | Jobs, source monitoring, recovery and resource ownership | Canceled/stale results rejected, bounded shutdown, no slow UI I/O | Pending |
| 2C.R.9 | Regression and performance qualification | Baseline comparison, real Windows and media fidelity acceptance | Pending |

Keep snapshot undo until measurements justify a replacement. Preserve the current
preview capabilities during extraction; a project clock alone is not a compositor.
Record deviations and unresolved qualification items rather than claiming completion
from a build alone.

## Feature development after stabilization

| Phase | Scope | Status |
| --- | --- | --- |
| 2C.2 | Stacked lanes and cross-track dragging | Pending |
| 2C.3 | Independent audio playback, detach/relink | Pending |
| 2C.4 | Track visibility, mute, solo, lock | Pending |
| 2C.5 | Layering, opacity and picture-in-picture | Pending |
| 2C.6 | Composite preview and continuous timeline playback | Pending |
| 2D | Whole-project export and explicit output color contracts | Pending |
| 2E | Audio envelopes, fades, levels and mixing | Pending |
| 3A | GPU acceleration | Pending |
| 3B | Transitions, keyframes, speed effects, titles and subtitles | Pending |
| 4A | Packaging and distribution | Pending |

2B.4.2 and 2B.4.6 will be scheduled separately. No release dates are assumed.

## Slice acceptance

- Go formatting, tests, vet, portable race checks and Windows builds pass.
- New regression tests cover the slice's changed ownership and behavior.
- Existing project and recovery formats remain readable.
- No undocumented changes to editing or media fidelity.
- Update architecture and roadmap with the exact verification evidence.
- Physical runtime acceptance is recorded separately, with tool versions and
  test environment where applicable.
