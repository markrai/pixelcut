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

## 2C.R.3 editor state ownership

`internal/editor.State` is the single owner of the current project, active
track ID, selected asset ID, selected clip ID set, primary clip ID and range
anchor ID. Clip references are track-scoped using the active track and globally
unique clip IDs. Selection remains single-track; cross-track selection is not
introduced by this refactor. Listbox indices are derived at UI and recovery
boundaries, never stored as live selection. Membership checks are constant-time;
ordered selection is derived in track storage order. Ctrl toggles, Shift ranges,
select all, selection reconciliation and track switching are portable operations.
After adopting a validated project snapshot, the controller follows a surviving
primary clip across track transfers, removes missing references, and falls back
to V1 when the active track disappears. A removed primary falls back to a
surviving selected clip in the active track. Asset deletion retains the former
nearest-list-row behavior in the GUI. Group and primary references are reconciled
before refreshing native controls and writing a checkpoint. Existing index-based
session JSON remains unchanged and is converted to IDs on restore; the range
anchor is reconstructed from the primary, not serialized.

New and Adopt take ownership of already validated project values; callers
must relinquish mutation of their slices. Project returns a borrowed read-only
value for transitional model queries; use Clone for independent edits. Selected
returns a detached clip. This intentionally avoids a full-project copy per paint
or query. Direct mutations of borrowed project slices are prohibited. The GUI
still owns history, dirty baseline, view coordinates, clipboard, draft gestures,
preview and native resources. CommitWithSelection installs the validated project
and selects new IDs before the UI refresh; this is a transitional adapter, not
the final transaction API. Crop and transform still have special live-preview
commit paths. 2C.R.4 will move history, dirty state and validated transactions into
the controller, including complete selection/history semantics. No schema or
media processing changes are introduced.

## 2C.R.4 transaction ownership

The portable editor controller owns committed project snapshots, selection,
history and the saved fingerprint. Commit validates the candidate project and
typed selection update before publishing either; it deep-copies caller-owned
project data and uses the canonical fingerprint to distinguish a no-op from
an edit. A selection-only transaction does not add history or discard redo.
A changed transaction records one before/after entry, including active track,
asset, selection, primary and anchor. Undo/redo restore those recorded editing
selections rather than the selection at the time the history command is invoked.
History is limited to 50 edits, remains in memory, and uses immutable project
snapshots shared between adjacent entries. A new changed commit discards redo.
The saved fingerprint is independent of history; returning to the saved project
is clean, and an unknown recovery baseline remains dirty until saved.

All ordinary edits and live crop/transform apply use State.Commit. The native
adapter selects preview invalidation and applies optional view changes only
after success; crop/transform keep their live preview. Native controls, gesture
drafts, clipboard and timeline view are not undoable history data. New/open and
recovery initialize a fresh controller; save marks its saved baseline only after
successful file persistence. Project returns a borrowed read-only value; callers
must clone it before editing. No new project or recovery JSON fields are added.

Portable command requirements cover single clip, video clip, group selection,
asset selection, clipboard and history availability. Native IDs map to these
requirements for command dispatch and menu filtering. Menus retain additional
position/source-specific checks; individual operations still validate arguments
and reject incompatible candidates. Native modality, busy and gesture handling
remain at the presentation boundary. Toolbar appearance is not yet dynamically
bound to all eligibility conditions. Geometry, playback and service lifecycle
remain later extraction slices.

## 2C.R.5 timeline extraction

`internal/timeline` owns the portable viewport, coordinate conversion, current
active-track lane constants, clipped clip spans, trim-edge hit testing,
linked-audio hit testing, marquee intersection, drop-zone eligibility, zoom,
scroll, ruler spacing, edit-boundary navigation and drag evaluation. The
native adapter supplies the viewport and track/asset metadata; painting,
cursor feedback, context menus and selection use the same clip geometry.
Thumbnail and waveform source mapping consume the same fractional pixel
origin. Native mouse capture, focus, GDI drawing, playback requests, worker
requests and committed edit publication remain in the Win32 adapter.

Drag evaluation takes a stable ClipRef, pixel displacement, initial scale,
marker, selection IDs, snapping policy and optional decoded frame index. It
returns a candidate clip or candidate project with validity and snap metadata;
it does not mutate the input or publish an edit. The existing project-domain
methods remain authoritative for overlap, movement, ripple and source limits.
The shared magnetic tolerance is eight pixels capped at two seconds. Frame
snapping retains priority over magnetic snapping; Shift bypasses snapping.
The ten-millisecond ordinary drag grid and four-pixel gesture threshold remain.
Invalid candidates are for preview only, never committed. The existing fixed
active-track layout is deliberately preserved; stacked lane geometry and
vertical transfer are deferred to 2C.2. No JSON schema or media backend changes.

The geometry now makes visible one-pixel clips hittable, ignores offscreen
clips during marquee selection, and only recognizes linked-audio hits for
sources with audio. This corrects a prior mismatch between rendering and input.
No new timeline interaction feature is introduced. Portable tests exercise
viewport bounds, clipping, hit testing, drop zones, snapping, group movement,
ripple trimming and frame-snap precedence. Native physical interaction and
performance qualification remain outstanding.

## 2C.R.6 deterministic composition evaluation

`internal/composition.Compile` validates a project once and constructs an
immutable indexed snapshot with detached clip geometry and asset metadata.
It sorts clips within each track by start time and indexes distinct clip
starts and exclusive ends. Compile does not mutate the supplied project,
probe files, launch processes, or depend on native selection. Recompile on
committed project changes; do not compile for every playback tick. An
Evaluator supports concurrent read-only At calls, each returning detached
crop and transform properties. Callers must not mutate the input concurrently
with compilation.

At(t) evaluates integer project microseconds in [0, Duration()]. Negative or
past-end timestamps return ErrOutsideTimeline. At the exact end, including
an empty project, there are no contributions and NextBoundary equals End.
Before the end, NextBoundary is the earliest strictly subsequent clip start
or end across all tracks; the active contribution set is constant until that
boundary, although its source timestamps advance. Leading and interior gaps
have empty contribution sets. An audio-only interval has no video layers.
The evaluator does not skip gaps or decide whether transport should advance.

Each contribution identifies its clip and track, asset metadata, half-open
project and source intervals, and exact source timestamp (from Clip.SourceAt).
Video layers include the existing crop and transform values without copying
or reimplementing the media placement algorithm. Video order is bottom to top
in stored project track order, not lexical or numeric track ID order. Audio
contributions follow stored track order. A video clip with HasAudio contributes
linked audio regardless of video occlusion. An independent audio clip contributes
its referenced audio; equal source assets are not deduplicated because distinct
clips are distinct contributions. A silent video contributes no audio. The
model records HasAudio rather than individual stream identity; actual stream
selection and mix policy remain backend work. Offline media is still a logical
contribution and must be handled explicitly by the eventual backend.

Evaluation uses binary search in sorted nonoverlapping track clips and the
boundary index, without file I/O or full-project validation per timestamp.
It produces a logical contribution description, not decoded frames, PCM or a
render graph. There is no new canvas/color-conversion policy, opacity, gain,
visibility, mute, solo, transitions or speed mapping. Source range (including
unknown legacy range) is preserved as recorded, not inferred or converted.
No project or recovery schema change is required. Existing single-clip preview
and export remain unchanged in this slice; the playback controller in 2C.R.7
will consume this contract, and whole-project rendering follows later.

Portable tests cover unsorted clips and assets, layer order, linked and
independent audio, adjacency, gaps, exact final boundaries, maximum timestamp,
legacy compatibility, invalid-project rejection, snapshot isolation and
concurrent evaluation. CI also records compilation and evaluation benchmarks
for a 12-track, 2400-clip metadata-only workload. Physical playback and HDR
qualification are explicitly outside this phase.


## 2C.R.7 portable transport and preview adapter

The existing single-source preview is a compatibility backend, not the
authority over timeline targeting. Play and ordinary video seeks resolve a
stable clip reference using the compiled composition evaluator and project
time. The adapter explicitly supplies its active video track because the
current backend cannot composite multiple layers. The compatibility policy
skips leading/interior gaps on Play, preserves exact seeks into gaps, and
does not wrap at the final end. Independent audio editing and selected-clip
crop/transform preview remain explicit special paths. Selection is not an
input to normal transport target resolution. The GUI caches the evaluator
by the controller's committed project fingerprint; edit, undo, redo and
project replacement select a new snapshot when necessary.

`internal/playback.Controller` owns the active preview segment's project-time
position, transport state, requested pause/speed, seek serial and accepted
backend observations. It is independent of Windows, process handles, crop
drafts and source identity. Loading, Seeking, Playing, Paused, Finished,
Failed and Stopped are explicit states. Status distinguishes confirmed
backend observations from pending intent. The controller rejects stale
serials, out-of-segment observations, exclusive-end seeks and events after
failure or stop. A new session receives a distinct controller; native process
completion retains its existing generation guard. Request and observation
access are synchronized. The source-to-project mapping is shared and clamps
source observations at interval boundaries, while seeks target the last
representable source tick rather than the exclusive end.

`internal/playback/mpv` implements the JSON IPC command/reply protocol
independently of Win32. The Windows mpv adapter owns preview launch,
temporary files, pipe polling, source-time conversion, crop/transform IPC
and process cleanup. The native editor binds the transport status to
controls and playhead, requests cancellation on IPC failure, and handles
native focus and completion notifications. Existing HDR selection, preview
speed, exact seeks, crop scripts and live transform behavior are retained.
The portable controller is tested with simulated backend observations, and
the IPC protocol is tested with in-memory streams.

This is not yet a continuous project-wide render clock: the current
transport is bounded to the backend's active source segment, and the GUI
retains the inactive timeline marker and native gesture/view state.
Transitions between sources still reopen the single-source backend, and
completion does not automatically traverse later clips. Continuous mixed
audio and video composition are future product features. The current
backend still has synchronous cancellation waits and potentially blocking
pipe I/O; bounded resource teardown, unified job lifecycle and asynchronous
services are 2C.R.8 work. No project format, source color contract or
external dependency changes. Physical native playback, VFR seeking and HDR
display qualification remain outstanding.


## 2C.R.8 background services

The portable internal/service package provides a single-owner latest-value
worker, foreground job identity and completion delivery, source inspection,
and context-bounded worker waits. Latest retains at most one pending input
and one completed result. Work runs without holding the mailbox mutex.
Inputs must be immutable for their lifetime; project snapshots are already
detached on editor commit, and recovery borrows those immutable snapshots.
A submission sequence is not a durable acknowledgement. Only completed
successful Store.Save results are considered persisted. The session store
remains single-writer, with the same checksummed two-slot format and
atomic replacement. A failed write leaves the last valid slot intact.
Rapid edits can coalesce intermediate recovery snapshots. The close path
disables editing and polls for the final sequence, with a five-second
deadline and explicit warning before closing without a successful
checkpoint. The instance lock is never released while a recovery write
may still run in this process; process exit releases it if necessary.

Source monitoring now performs Identify/stat calls on a dedicated worker,
not on the native timer. Immutable path lists are rebuilt only when the
project fingerprint changes, and scan epochs reject results from previous
path lists. New or changed sources are shown as CHECKING until inspected;
only checked missing paths count as offline. Periodic scans are scheduled
at two-second intervals when the previous scan completes. Startup preview
restoration waits for the initial scan and is skipped if the user has moved
the marker or changed the project/selection in the meantime. File identity
is still path/size/mtime, not content identity. Explicit preview and relink
operations retain fresh source checks for safety.

Foreground media operations retain the existing single-job busy policy.
The runner attaches a job ID and project epoch; the native adapter also
checks the originating committed fingerprint before applying a result.
Canceled work cannot publish into a later job. Completion notifications
are hints; the UI timer can consume results even if a notification is
lost, and duplicate notifications cannot block the event loop.
The cancellation contract suppresses stale publication; it does not
promise to forcibly interrupt arbitrary OS operations.

Preview stop now cancels without waiting in the UI callback. A new preview
allows at most a 250 ms wait for the retired process to release its host;
if it has not stopped, startup fails explicitly and can be retried.
The adapter closes its pipe on cancellation. Late process completions
continue to use generation guards. Media-worker retirement is nonblocking;
new workers use their own memory caches and the tool/source-keyed disk
cache. Retired workers are tracked until completion and restart is deferred
if four retired workers remain. Clear-cache waits for retired workers on
the background job with a five-second deadline, and aborts rather than
clearing while a producer can still write. Shutdown cancels workers
without an unbounded native wait. OS calls such as stat, fsync, and named
pipe operations may still take longer than cancellation; no claim of
forced termination of a blocked OS call is made.

Explicit open/save, preferences/recent persistence, startup recovery read,
single-source availability checks and final relink revalidation remain
synchronous. These are distinct user-triggered durability/safety paths;
a fully nonblocking lifecycle UI would require additional continuation
handling and is not claimed here. Cache scheduling and native worker
implementation remain separate for thumbnail and waveform policies.
No schema or external dependency changes. Physical Windows cancellation,
shutdown and recovery acceptance and performance measurements remain pending.


## 2C.2 stacked timeline lanes

The portable timeline Stack computes video/linked-audio and independent audio lane bounds, viewport clamping, reveal and hit testing. The Windows adapter applies those same bounds to painting, pointer input, marquee, context menus and asset placement. Later video tracks appear above earlier ones; independent audio tracks follow in storage order. Vertical scroll is presentation state only. Existing time mapping is unchanged.

Project.MoveClipsTo generalizes group movement to a compatible destination track, using destination edges, project origin and playhead for magnetic snapping. The operation preserves IDs, source intervals, transformations, group spacing and source references; it validates the entire detached candidate. Invalid candidates can be drawn as ghosts but never committed. The portable gesture evaluator rejects cross-kind moves and cross-track trims. The existing editor transaction commits a successful move with destination selection in one history entry. Asset and Explorer drops capture the target track ID rather than the current active track. No persistent schema, media engine or playback behavior changes. Cross-track selection and composition remain deferred.

This feature is staged separately from the stabilization gate: R.9 automated CI and paired benchmark collection passed, while performance review and physical Windows acceptance remain outstanding.


## 2C.3 independent audio audition and detach/reattach

Project schema version 3 adds the optional video-only AudioDetached flag. Versions 1 and 2 remain readable; detach upgrades the project. Older releases reject version 3 rather than silently restore audio. Ordinary clip split, copy, trim and transfer retain the flag. Detach inserts an independent audio clip with exactly the same asset and source/timeline interval and suppresses attached source audio. Reattach requires an explicitly identified independent clip with exactly matching asset and intervals. The UI only offers reattach for a unique match; it never discards a misaligned edit. No hidden pairing, group synchronization or source-file changes. Both operations are atomic editor transactions.

The composition evaluator excludes detached source audio and resolves independent audio targets using the same project clock boundaries. The existing single native preview process plays either the active video track (source audio unless detached) or the active audio track (mpv video disabled, no window). It reuses existing IPC seek, pause, speed, stale generation and shutdown ownership. This is isolated audition, not simultaneous video/audio composition. Selected-clip export of a detached video omits audio, uses lossless video-only remux for untouched clips and verifies zero audio streams and the existing HLG color contract on rendered clips. Independent audio export, audio-only file import, concurrent mixing, and automatic reattachment across misaligned edits are deferred.
