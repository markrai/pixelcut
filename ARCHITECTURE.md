# PixelCut architecture

Status: 2C.R architectural stabilization, baseline before extraction.

PixelCut is a source-fidelity-first, non-destructive video editor. The current
desktop uses Go and native Win32, with separate FFmpeg, FFprobe and mpv processes.
This document defines ownership boundaries for the refactoring, not a claim
that all target packages already exist. See [media architecture](docs/architecture.md)
for the supported HLG contract and [qualification](docs/qualification.md) for
the physical media and display gates.

## Non-negotiable invariants

- Original media is read-only. Project files store references and edits.
- Source color signaling is an input contract. Unsupported or contradictory
  sources fail explicitly; do not silently convert HDR to SDR.
- Timeline positions and source intervals use integer microseconds, with
  half-open intervals. Convert to floating point only at defined boundaries.
- IDs, not slice indexes or native listbox positions, identify entities.
  Existing index-based UI state is transitional debt.
- Same-track clips must not overlap under the current model; separate tracks
  may overlap. No operation may bypass project validation.
- Project mutations are transactional; rejected operations must not alter
  committed state, history or selection. Temporary drafts are not committed edits.
- Project and recovery formats evolve explicitly; retain version-1 and version-2
  compatibility until a separately reviewed migration changes the contract.
- Preview and export must converge on shared composition semantics. Do not
  equate matching color tags with equivalent display rendering.
- Keep the native UI responsive. Slow I/O, decoding and process shutdown
  belong outside the event loop; current synchronous paths are known debt.
- Do not add abstractions or dependencies without a concrete ownership,
  testability or performance benefit.

## Current ownership and known debt

| Component | Current responsibilities | Stabilization target |
| --- | --- | --- |
| cmd/pixelcut | CLI arguments, media invocation | Thin command adapter |
| cmd/pixelcut-gui | Win32, editor state, selection, timeline, jobs, mpv IPC | Thin native presentation adapter |
| internal/project | Assets, clips, tracks, validation, editing, file format | Authoritative domain model |
| internal/media | Probe, validate HLG, process arguments, filters, encode, preview | Media backend, not editor authority |
| internal/session | Recovery slots, preferences, recent paths | Persistence of immutable state |
| internal/mediacache | Bounded persistent media cache | Disposable derived-data storage |
| internal/thumbnail, internal/waveform | Source tile and audio peak algorithms | Backend-neutral analysis |

At the baseline, the GUI still owns most application orchestration, a hidden
clip list and index-based selection, and active-track-only preview. Playback
has a single-source clock. The UI timer synchronously checks source availability
and recovery state. The project model imports media geometry/validation types;
this is an existing dependency, not permission for the media backend to import
project or editor. Refactor these paths incrementally rather than adding a
second authoritative model beside them.

## Target ownership

Packages below are to be extracted only as their corresponding refactoring
slice requires them.

| Owner | Authority | Must not own |
| --- | --- | --- |
| internal/project | Persistent model, validation, identity, time and edit calculations | UI handles, selection, process lifetime |
| internal/editor | Committed project, active track, stable selection, edit transactions, history, dirty baseline | Native controls, media process execution |
| internal/timeline | Pure coordinate conversions, lane layout, hit testing and gesture evaluation | Window handles, committed state mutation |
| internal/composition | Deterministic video/audio contributions, source-time mapping, layer order | GUI selection, process lifetime, encoder choices |
| internal/playback | Project clock, transport state, backend coordination | Project mutations, native paint |
| internal/media | Decode/encode/filter/preview backend implementation | Editor selection/history, timeline policy |
| internal/session | Snapshot serialization, recovery, preferences, recent paths | Live controller or native window state |
| internal/mediacache | Cache keys, integrity, budgets, eviction | Authoritative project or recovery data |
| cmd/pixelcut-gui | Native events, layout application, GDI, menus, dialogs, view binding | Independent editing semantics |

## Dependency and concurrency rules

The native adapters may depend on core packages; core packages must never import
a cmd package or Windows GUI bindings. Project and composition must not depend
on editor, playback, session, or native UI. Composition evaluates the project;
playback consumes evaluation and drives a narrow backend contract. The existing
project-to-media dependency may remain until 2C.R.2 establishes a justified
replacement. Do not create circular dependencies, shared global event buses or
generic dumping-ground packages. Preview and export consume the same evaluated
composition, but may use different execution backends.

The UI event loop owns presentation state. The editor controller owns the
committed state and is mutated by one serialized owner. Background work receives
immutable snapshots or explicit inputs, supports cancellation, and returns
results tagged with their job and project generations. Stale results must not
mutate current state. Cache workers never change committed edits. Recovery
writes are serialized and must preserve the last valid snapshot on failure.
Bound waits and keep slow work out of native callbacks.

## Engineering policy

Feature development is frozen through 2C.R.9 except reviewed regression fixes.
Each slice updates [ROADMAP.md](ROADMAP.md), passes the existing and new tests,
maintains format compatibility, and documents intentional behavior changes.
Keep mechanical formatting separate from semantic refactors.

Use standard Go formatting, vet and race detection for portable packages.
Run Windows builds and desktop tests. Compare benchmarks with the same fixture,
Go version, OS and hardware before drawing performance conclusions. Coverage
is evidence of exercised statements, not a correctness metric or a substitute
for native interaction tests. Do not introduce arbitrary coverage or latency
thresholds without an observed baseline.

CI success and physical acceptance are distinct gates. Native mpv embedding,
HDR display output, real Pixel VFR footage and interaction responsiveness
require recorded Windows runtime acceptance. See [ROADMAP.md](ROADMAP.md)
and [baseline measurements](docs/refactor-baseline.md).

## 2C.R.2 project contracts

`Project.Validate` is the authoritative persisted-project contract, including
schema version, base track layout, maximum track count, globally unique clip
IDs, per-kind edit constraints, asset metadata and nonoverlap. The previous
ValidateEditorLayout entry point delegates to it for compatibility. File load,
save, recovery and model operations use the same validation. Version 1 remains
readable without migration; adding additional tracks or independent audio
upgrades the candidate to version 2. No JSON fields or recovery envelope versions
changed. Files previously accepted by the low-level model but rejected by the
editor (invalid layouts) are now consistently rejected, as are explicit color
ranges other than tv or pc. Unknown legacy range (empty) remains valid.

`ClipRef` carries a track ID and clip ID; `LocateClip` resolves the globally
unique clip ID after a track transfer. `ClipIndex` derives transient indexes;
`Clip` returns a detached copy. Stable references are available for 2C.R.3,
but native selection state has not yet been migrated. Asset and track lookup
remain linear until benchmarks justify an index. `Clip.Clone` is the shared
nested-copy operation used by project snapshots, split, clipboard and drag.
Project slices are still exported; callers must not mutate committed snapshots.

`Interval` uses nonnegative half-open bounds. `TimelineInterval` validates
source duration and overflow before adding timeline ticks; `End` returns zero
for invalid intervals. `SourceAt` rejects the exclusive end. Decimal seconds
input retains float64 parsing and microsecond rounding, rejecting rounded
values outside int64; exact decimal parsing at extreme durations is not claimed.
Model editing and playback-target calculations reuse these operations. Native
geometry and preview time mapping will migrate in their own slices.

Validation failures are classifiable with errors.Is for unsupported versions,
invalid assets/tracks/clips, overlap and missing references in migrated paths.
Existing operations may return detached invalid drag/group candidates with an
error for red ghost rendering; an error result must never be committed. These
are not mutations of the input project. Unified commit and history ownership
remain 2C.R.4. The project-to-media geometry dependency remains intentional
until composition requires a narrower shared contract; no duplicate geometry
algorithm or new dependency is introduced.
