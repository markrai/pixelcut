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


## 2C.4 track controls

Version 4 adds optional Hidden, Muted, Solo and Locked flags to Track. Versions 1–3 retain their prior interpretation; version 3 detached audio remains valid. TrackPolicies computes global solo inclusion and independent video/audio gates before the composition evaluator compiles immutable track entries. Hidden and mute override solo. Disabled contributions do not shorten project time. The isolated audition target skips suppressed clips and preserves gap-start/end behavior; hidden video with audible attached audio uses the existing audio-only transport, and a muted video uses the existing video-only mpv mode. No video codec or color transformation changes. Selected-clip export remains an explicit isolated operation: honor mute/detach, ignore hide/solo.

Lock is an edit constraint, not an output constraint. Project operation guards reject locked source/destination edits and changes to referenced assets. State.Commit additionally compares previously locked track contents and referenced asset metadata, so direct inspector candidates or combined unlock-and-mutate edits fail before history or selection changes. Track flag changes remain allowed while locked; unlock must be a separate committed transaction. Undo/redo restores historical snapshots as before. Timeline/header menu and cursor eligibility mirror the constraint; locked clips stay visible and selectable. Visibility/mute/solo and lock are saved and recovered as project state. Physical acceptance is separate from automated CI.


## 2C.5 layer geometry contract

Version 5 adds optional composition canvas width/height and per-video-clip opacity_percent (nil means 100; explicit 0 is transparent). Older versions 1–4 remain readable with their original control semantics. Clip cloning, history, clipboard and validation preserve opacity, and the lock guard protects clip-level layer edits. The composition canvas is independent of clip-local canvases: automatic uses the earliest clip of the bottommost populated video track, crop/transform dimensions when applicable, with bounded orientation fallback. Explicit canvas controls are global and undoable; changing the project canvas does not mutate clip data, including locked clips. A layer preset freezes the resolved canvas and stores an ordinary validated transform in one transaction. Existing isolated transform preview/export retain their black-padding semantics; layer planning treats that padding as transparent and records both local content placement and outer canvas fit. Bounds describe the resulting content, not a flattened black-padded frame.

Evaluator.PlanLayers returns ordered immutable contributions with source interval/time, crop, transform, opacity and geometry. Later stored video tracks are above earlier tracks, matching the stacked timeline. Hidden/solo policy filters video; zero-opacity layers are omitted from the layout plan without suppressing audio. The plan is a geometric contract only. The Windows owner-drawn schematic consumes the same plan and does not decode frames or claim HDR rendering. It is closed when switching to source preview or changing the timeline. Existing source playback ignores opacity and labels that limitation; selected-clip export rejects nonopaque layers. Decoded compositing, blending/color-space policy, synchronized media and whole-project export are intentionally deferred. No change to FFmpeg/mpv color pipelines.


## 2C.6.1–2C.6.2 paused composite preview

The still decoder is a single-worker-owned, portable media backend. It revalidates each source's HLG metadata and display geometry, indexes actual normalized decoded frame timestamps, and chooses the last indexed timestamp not greater than the requested source time. FFmpeg performs an accurate input seek to that index timestamp; timestamp quantization and VFR boundary behavior require physical qualification. Four source indexes (each bounded by the existing one-million-frame cap) and an LRU of at most 64 decoded regions / 24 MiB are kept in memory, keyed by path/size/mtime, timestamp and filter graph. Cache hits and outputs are detached copies. No persistent cache changes. A single request uses sequential source decoding, a 90-second deadline, 960-pixel output cap, 16-megapixel scaled intermediate cap, and 32-megapixel source cap. Cold full-file indexing is intentionally not the continuous playback design. Unsupported media, missing frames and changed sources fail rather than silently dropping a layer. The existing source identity contract is not a content hash.

The portable composition.RenderStill consumes the evaluated layer plan and draws only content regions, not black padding. The decode backend applies crop, orientation, scale and clipping from the same placement geometry, then maps the visible region to the bounded preview dimensions. Empty gaps are black. Source pixels are converted from HLG to SDR BT.709 using the existing thumbnail tone mapping policy; opacity is blended in 8-bit display RGB, not scene-linear HDR. This deliberately approximate, labeled SDR diagnostic is not an HDR master or an export renderer. Full-range HLG is supported for untransformed stills; the existing limited-range transform restriction remains. Do not reuse this SDR output for HDR export.

The native UI submits one immutable plan at a time to a single latest-value worker with per-request cancellation; sequence, project fingerprint, marker and source identity gate delivery. New requests cancel superseded work and do not spawn parallel decoder processes. Stop, edit, project switch and window shutdown invalidate outstanding work. The existing owner-drawn preview surface displays completed pixels via GDI; the UI timer does not decode or wait for processes. Source monitoring invalidates an already displayed frame on a detected change. Static preview is separate from existing mpv audition and from continuous transport. No project version or existing encode changes. Continuous decoders, clock/audio ownership, VFR frame scheduling and HDR output remain later slices.


## 2C.6.3 continuous composite transport

The continuous preview is an explicit silent SDR diagnostic, separate from the selected-source mpv transport. The same evaluated layer plan and RenderStill blending path consume a new sequential decoder interface; the only added request identity is the globally unique clip ID and source duration. Each active clip gets an FFmpeg raw-BGRA pipe with its own cancelable session context, validated source geometry and shared indexed predecessor timestamp. The FFmpeg filter samples a 10 fps grid relative to the first requested source time; timestamp phase is approximate (FFmpeg seek and filter rounding), not VFR exact. The stream is reused for consecutive 100 ms source ticks with the same geometry and source identity; inactive streams are canceled and reaped before opening new ones. A discontinuity replaces the affected stream. Four simultaneous streams are permitted; excess active layers fail explicitly, never silently disappear. Each frame reads an exact bounded region and is checked against source identity before and after decoding. Full-file cold indexing, source/intermediate allocation caps, and the existing tone-mapped SDR color contract remain. A small bounded tail pad compensates for sampling near source EOF; the model's exclusive duration remains the request bound. No source files are written.

RenderSequence advances integer project ticks by 100 ms, clamps to the final endpoint, and renders the final black frame. The preview is capped at 640 pixels on its longest side. A 90-second per-frame deadline includes cold indexing; process lifetime belongs to the run and cancellation interrupts active reads. Frames are paced at a maximum target of 10 fps, not a guaranteed realtime rate. A single pending delivery provides backpressure; no audio or wall-clock master exists yet. The native UI owns the current run pointer and last displayed time; it ignores results from canceled runs, rejects changed project fingerprints and source versions, and only advances the marker when consuming a valid frame. Stop retains the last displayed frame; a seek stops the run and enters the paused still path. Edits and project switch cancel the run; restart is rejected until retired decoders are reaped. Shutdown cancels before recovery and waits within the existing close deadline. No unbounded native join or new persisted settings. A sampled clip shorter than 100 ms may be skipped. Continuous HDR, sample-accurate boundary scheduling, speed changes, and audio/master clock are later slices, not properties of this renderer. No project schema or export changes.


## 2C.6.4 audio-confirmed composite clock

The portable composition mixer evaluates the existing audio contributions at each half-open project boundary, maps boundary ticks to a 48 kHz sample grid (ceiling), and fills up to 2400 stereo frames per block. Independent and linked clips are distinct contributions; mute, global solo and AudioDetached are evaluated upstream. Gaps and audio-less video produce explicit zero PCM. Samples are summed in int32 and saturated to signed 16-bit; no auto gain, normalization or fade is introduced. Up to eight concurrent audio clip streams are supported; excess contributions fail explicitly. FFmpeg selects the first source audio stream, uses accurate input seek and resampling with timestamp compensation and trailing silence padding. Persistent per-clip pipes are retired at contribution changes; read lengths, source identities and cancellation are checked. Source audio offset, resampling and exact boundary behavior require physical qualification; arbitrary multichannel downmix follows FFmpeg's default. No persisted audio settings or export changes.

The playback.SampleClock advances only by monotonic completed PCM frame counts and clamps the final sample to the exact project end. Windows waveOut uses the default device and up to three native-memory buffers, with at most two additional queued producer blocks. The audio worker owns the device and all native buffer operations; no driver retains Go memory. Playback starts after the first composite frame is available and the initial audio queue is filled. Completion polling is on a worker, not the UI thread. Cancellation resets the device and unprepares buffers before freeing; failed driver cleanup poisons subsequent composite audio attempts until restart rather than repeatedly allocating unreleased device buffers. A stalled device with outstanding buffers fails explicitly. Driver calls may still block, and physical driver behavior is not qualified by CI.

The composite run owns three workers: PCM production, device output, and video rendering. They share a cancelable context; the first error cancels siblings. Video renders from the same layer plan, samples at 100 ms on a grid anchored to the start marker, waits for the audio clock before publishing and discards frames over 200 ms late. UI playhead reflects the audio-confirmed position independently of video; excessively stale video is blanked. A single video delivery slot bounds backpressure. The clock stalls on audio starvation; it is not a hardware sample-position query and has up to 50 ms buffer granularity plus device latency. Audio is required even for silence; no unconfirmed wall-clock fallback. Pause cancels the run and requests an exact paused still if the displayed frame differs from the marker. Resume opens fresh streams. Retirement gates switching between source and composite playback. The previous frame-paced RenderSequence remains a portable testable compatibility path, not a second clock driving native composite playback. Exact VFR, low-latency seek and device drift/underrun qualification remain 2C.6.5–2C.6.6. HDR output, project export and advanced audio remain separate.


## 2C.6.5 navigation and cut validity

The compiled evaluator indexes visible, nonzero-opacity video boundaries separately from all edit boundaries. The clocked scheduler chooses the union of the run-anchored 100 ms grid and visible video boundaries, catches up to the latest eligible sample on clock jumps, and attaches an exclusive ValidUntil tick to each picture. Both producer and UI apply the same CurrentAt predicate; a late outgoing frame cannot cross a cut even if within the existing 200 ms lateness budget. Final black frame is valid only at exact end. Audio-only and hidden layer boundaries do not force video resampling. Irregular source tick requests can restart a persistent video pipe; cold seek/index costs and short clips below the physical audio/UI sampling interval remain limitations.

The UI keeps one pending resume intent and one debounced still deadline, guarded by project fingerprint and marker. A seek cancels old audio/video workers and still requests, clears stale pixels, and updates the marker immediately. Ruler drag seeks remain paused and coalesce at 80 ms inactivity; release resumes only if playing on entry and not at end. Click/keyboard seek preserves playing state, Stop/Escape cancels pending resume, edits/project switch discard the intent, and retired workers gate restart. Clip and marquee gestures freeze composite audio before capture. Track selection alone no longer exits composite mode; project-wide edit navigation uses the compiled all-track boundary index. Selected-source frame steps still use actual indexed timestamps and preserve composite view after a cold frame index. No project format or export changes. Reopening decoders is deliberate; this is bounded restart-based seeking, not an in-place decoder seek or guaranteed gapless scrub. Physical A/V latency, exact VFR seek, native interaction and resource qualification remain 2C.6.6.


## 2C.6.6 resource qualification

Still and streaming source validation still inspects HLG metadata on a cold decoder, but frame indexing now uses Engine.CachedFrames with the existing bounded media cache. Cache key, checksum, tool/source identity, and post-index identity checks remain unchanged. Clearing the disk cache explicitly stops composite playback, closes the still worker, and includes both in the existing bounded worker drain before disk deletion. A pause requests a deferred still rather than starting it while the prior run is retiring. No new persistent format, dependencies or automatic upload.

Video and PCM decoder owners count opened and retired pipes, peak active streams, and decoded units. Retirement means Wait returned after cancellation/close, not that FFmpeg exited with status zero. The run aggregates immutable decoder counts on retirement and mutex-protected observation counts while running. A video observation records the successful render duration and whether the frame was eligible at delivery; the UI separately counts stale delivery and presentation with the audio-confirmed clock difference. Queue starvation counts transitions to an empty outstanding device queue after start with samples still to submit; it does not measure hardware underruns. Metrics hold only scalar values and no paths or media. Each session has its own report; a seek creating a new run replaces the report. Diagnostics remain accessible after pause and retirement. Real media and allocation tests are automated; the Windows driver, physical clock, display HDR signal and full process RSS need measurement outside CI. The SDR compositor remains a diagnostic and never feeds the HDR export path.


## 2D.1 whole-project export boundary

A new internal/projectexport package consumes the immutable composition evaluator rather than the preview renderer. The plan stores half-open edit spans, planned layer geometry and independent audio contributions, and integer CFR/sample grid indices. It has explicit 24-hour, 10,000-span and 100,000-contribution limits; it does not allocate per-frame arrays. Output color is a separate media.ExportContract (HEVC hvc1, 10-bit BT.2020 HLG limited range, left chroma, square pixels, normalized orientation and selected integer CFR; AAC 48 kHz stereo only when audible contributions exist). Input validation re-probes every contributing source, rejects unsupported auxiliary streams and multiple audio streams, records recognized Pixel metadata exclusion, checks project metadata and file identity before returning. The source snapshot can be rechecked at execution/publication. The planner deliberately rejects visible full-range or center-sited sources until actual conversion is implemented. No pixel output, encoder, persisted export settings or file publication exists in 2D.1; the SDR still/streaming path is never an HDR input. Future renderers consume this plan, not GUI state. CLI emits a local JSON report that includes source paths; do not publish it unredacted.


## 2D.2 opaque HLG video execution

RenderVideo builds and preflights a fresh project plan, validates the explicit video-only subset, then owns one staged output and one encoder for the session. Each sampled edit span opens at most four independent source decoders. The strict export frame index rejects negative normalized timestamps and non-increasing decode-order timestamps, preserving the ordinal mapping; it is cached in a four-source in-memory LRU. FFmpeg decodes from source beginning and selects the contiguous ordinal interval needed by a span, without an approximate seek or a CFR fps filter. The Go sampler holds one frame per decoder and advances through raw frames until the exact indexed predecessor for each project CFR tick; repeated frames are reused, skipped source ordinals consumed. This is bounded memory but may decode a long prefix repeatedly. Exact source decoder ordinal equivalence for arbitrary real media is a qualification gate, not a proven invariant beyond tested fixtures.

Source color remains HLG throughout: explicit left-sited limited-range 4:2:0 to 10-bit 4:4:4 expansion before cardinal rotation, crop, edit transform, clipping and scaling. Each opaque rectangle is copied bottom-to-top in planar Y'CbCr; black is code 64 luma and 512 chroma. Chroma is downsampled once with explicit left siting at final encode; no linear-light blend is attempted. Intermediate 16-Mpixel and output 4096x2160-area limits, four decoders, four frame indexes, bounded error tails and pipe backpressure constrain resource use; no full timeline buffer or lossless intermediate files. FFmpeg CPU/thread flags are bounded but process RSS is not mathematically capped. Source identity checked before and after each decoded span and immediately before publication. Each child is canceled/closed/reaped on error. The staged file is verified for HLG contract, Main 10 profile, frame count and stream duration, then hard-linked to a new destination without overwrite. Full-range/center-sited normalization, alpha blending, audio, progress UI and physical acceptance remain later phases.


## 2D.3 scene-linear HDR opacity

The export decoder now uses explicit source metadata for range/chroma normalization, with zscale expanding 10-bit 4:2:0 into limited-range BT.2020 HLG 4:4:4 before rotation and geometry. Full/limited range and center/left chroma are admitted; other source formats still fail closed. The existing project transform validation for full-range assets remains independent. No changes to the selected-clip exporter or preview. The prior opaque-only composer is now a fast path; mixed spans evaluate source-over coverage top-to-bottom, stopping at opaque coverage. Convert normalized 10-bit Y'CbCr to BT.2020 nonconstant-luminance R'G'B', clamp individual nonlinear channels to [0,1] for mixed pixels, apply inverse HLG OETF, sum weighted scene-linear RGB, apply HLG OETF and quantize once into limited-range 10-bit Y'CbCr. Remaining coverage is black. No display OOTF or SDR tone mapping. Opaque pixels bypass the nonlinear roundtrip, so opaque excursions are not deliberately clipped by this compositor. Spatial geometry and chroma resampling remain in the HLG signal domain, not scene-linear resampling. Mixed pixels use exact scalar math, without an additional full-canvas float buffer; CPU performance is not yet qualified. Final 4:2:0 conversion and HEVC publication contract unchanged.


## 2D.4 offline audio and mux

Project export audio is independent of preview timing and waveOut. It consumes the same immutable plan and the exact span sample indices. An offline clip decoder uses normalized source timestamps (FFmpeg copyts/start_at_zero, aresample 48 kHz with async timestamp compensation and first_pts=0), finite apad/atrim sample intervals, and stereo signed 16-bit output. For clip timeline start T and source in S, first output sample is ceil(T*48000/1e6); source index is first output sample plus floor((S-T)*48000/1e6). Signed integer arithmetic avoids cumulative rounding drift. Decoders are retained across intervening spans until their exclusive clip sample end; on retirement, exact consumption, EOF, successful process exit and source identity are checked. No approximate source seek is used. Up to eight active audio processes; mixer operates in 2400-frame blocks with int32 accumulation, saturation once and silence for gaps/tail. Decoder and mux errors cancel and reap children. No raw PCM timeline file is created.

RenderProject and RenderVideo share one renderer. The first stage is verified video-only HLG. If audio is enabled, a second same-directory temporary MP4 receives stream-copied HEVC and AAC-LC encoding of the mixed PCM stream. The MP4 movie timescale of 240000 is divisible by every supported CFR and 48000. Final inspection verifies HLG metadata/profile, video frame count/duration, both streams' zero starts, stereo AAC at 48 kHz and exact audio duration_ts in 1/48000 units. AAC packet padding/priming can result in extra decoded tail samples outside the presentation interval; the contract is the MP4 presentation duration, not raw AAC decoder output length. Sources are rechecked before mux and publication; both staged files are cleaned on normal failure/cancel, and only the final verified artifact is hard-linked to an absent output. Peak disk usage includes both compressed video and final MP4. Physical A/V fidelity, timestamp compensation behavior on real VFR sources and long-trim performance remain qualification gates.


## 2D.5 native export orchestration

The toolbar Export menu preserves the existing selected-clip command and offers explicit project CFR selection. The UI validates the in-memory project snapshot and destination, confirms output contract/audio/duration, then freezes the current project fingerprint and configured tools. The job retires preview/still/thumbnail/waveform workers before running the shared exporter. The existing service.Jobs epoch owner still owns completion. Export cancellation uses a child context distinct from Jobs.Cancel (which discards completion); editing remains disabled until the worker has returned after process and temporary-file cleanup. The UI timer reads a mutex-protected latest progress snapshot rather than receiving unbounded per-frame messages. Progress is submitted frames or samples, not encoded output or an ETA; separate stages cover finalization, inspection and publication. Success is reported only after publication; a late cancel after a successful link does not turn success into cancellation. Close during export requests cancellation, does not destroy the editor. The CLI shares the observed render API and optional throttled progress output. Existing destination checks are repeated in the backend, and hard-link publication atomically refuses late collisions. No new project schema/preferences/dependencies. The job does not promise cleanup after forced termination or immediately interrupt uninterruptible filesystem calls.


## 2D.6 export qualification and hardening

Final project publication now additionally requires video stream time_base=1/240000 and duration_ts equal to the planned frame count times the exact per-frame tick step. A streaming FFprobe packet scan checks each presentation timestamp and packet duration, unique grid position and exact count; a bounded bitset handles B-frame packet reordering without sorting or decoding the entire output. The 24-hour/60 fps upper bound allocates at most 648000 bytes for this bitset. Both video encode and audio mux explicitly set video track and movie timescales. Source frame probing fails on FFprobe error diagnostics even when exit status is zero. No sampled visible rectangle means render validation fails; short individual unsampled spans still warn. The publication progress callback occurs before the final source identity check, narrowing the last mutable-source window. File identity remains size/mtime/path, not content hashing or a source lock. Normal cancellation/errors prevent publication; atomic hard-link publication still refuses destination races. The scan adds a sequential demux pass and is not a proof of pixel integrity. Full resource/physical qualification and known limitations are tracked in docs/export-qualification.md.


## 2E.1 shared audio gain and mixer

Project schema 6 introduces optional per-clip integer audio_gain_percent, with nil representing unity and range 0–400. LayerVersion remains 5 so existing opacity/canvas projects validate without migration. The gain setter is a validated immutable transaction and obeys track locks. Gain is legal on attached audio and independent audio clips, not on silent assets or detached video. Clone/split/copy/paste preserve ownership; detach transfers gain from video to independent clip and reattach transfers the independent clip's current gain back. Evaluator snapshots carry effective integer gain; zero removes an audio contribution but does not affect video or timeline length. Track mute/solo remains authoritative. Both composition.MixPCM and projectexport.mixAudio consume PCMMixer: bounded stereo frames, up to eight contributions, int64 sum in hundredths of signed PCM units, nearest rounding with half ties away from zero and one final signed-16-bit saturation. No intermediate clipping, gain normalization or per-track order dependence. Offline and preview retain separate decoder and timestamp adapters; shared sample arithmetic does not establish real decoder equivalence or AAC equality. Source audition remains separate (zero suppresses audio, positive non-unity labeled raw). Selected-clip export rejects positive non-unity gain. New gain UI and reversible mute are deferred to 2E.2. See docs/audio-processing.md.


## 2E.2 reversible clip mute and native audio controls

Schema 7 adds optional clip audio_muted; GainVersion remains 6. Clip mute suppresses effective gain without modifying stored gain. Detach and reattach move both fields; clipboard and split retain them. Track policy remains independent; clip mute does not hide video. The evaluator excludes zero-effective-gain audio and active-track audition advances over suppressed clips when there is a later enabled clip. The source mpv adapter applies an explicit linear lavfi volume filter only for non-unity audible gain; silent video uses aid=no. Source audition is separate from the shared PCM mix, so mpv/device output is not asserted bit-identical. Selected-clip export accepts silent clip/track policy, but rejects positive non-unity audible gain. The native inspector and context menu invoke validated immutable project edits through the existing commit/history path; no separate state owner or live slider draft. Changes stop preview through existing commit invalidation, and the next playback uses the edited snapshot. The existing transform inspector now captures all normal property controls before switching layouts. Source waveforms remain unprocessed. See docs/audio-processing.md for runtime acceptance.

## 2E.3 sample-level audio fades

Schema 8 stores independent integer-microsecond edge fade durations, bounded to 60 seconds each. MuteVersion remains 7. The effective duration of each ramp is the lesser of configured duration and current clip duration. The evaluator carries fade settings and clip interval by value. The shared PCM mixer evaluates the product of linear fade-in/fade-out factors at each output sample's timeline tick (floor of the 48 kHz sample time in microseconds, relative to the preview run start or export zero). Factors use a fixed-point scale of one million, multiplied with configured gain and signed PCM in the existing int64 sum. With eight streams at 400% gain the absolute accumulation bound is 104857600000000, well within int64. Final sample rounding and saturation still occur once. Constant gain has a fast path; no extra per-sample allocation or fade-specific decoder restart. Fade ramps do not add evaluation span boundaries. Mute/zero gain removes the contribution before decoding. Trimming preserves configured durations, re-anchored to new edges; splitting retains only the original outer edge fades, so a split inside a ramp does not preserve its exact waveform. Move/copy preserve configuration; detach/reattach transfer it along with gain/mute. No live draft or new history owner. Source audition remains raw with respect to fades, explicitly labeled; composite preview and whole-project export process them. Selected-clip export fails closed on audible fades. See docs/audio-processing.md.

## 2E.4 source-anchored audio automation

Schema 9 adds optional VolumeEnvelope with 1–128 strictly increasing source-time keyframes per clip, each a 0–100 integer percentage of configured gain. FadeVersion remains 8. Points must lie in the asset interval including its exclusive-end boundary as an interpolation anchor. Nil is unity. Clones, clip accessors, clipboard, split, detach/reattach, and evaluator output own independent point slices. Source anchors deliberately survive outside current trims; each split result gets its own complete curve, so an unchanged retained source interval evaluates identically. A replacement that shortens the asset beyond a stored keyframe fails validation. Linear interpolation holds first/last values outside the keyframe interval and uses bounded integer multiply/divide (bits.Mul64/Div64) to avoid long-time overflow. The shared mixer samples the envelope at the timeline-to-source mapped tick, multiplies it by edge fades and configured gain, and rounds/saturates only after summation. The envelope factor is bounded by unity, preserving the previous accumulator bound; no additional span boundaries or decoder restarts. The existing no-envelope/no-fade fast path remains. The native timeline graph uses the same factor function; waveform cache remains source-based. Alt-drag owns a bounded UI-only draft and frozen view; release validates current selection/fingerprint and commits once, Escape/capture loss cancels, competing commands and wheel scrolling are blocked during drag. No new history owner or live audio mutation. Direct source audition labels envelopes as composite-only; selected-clip export rejects audible envelopes. See docs/audio-processing.md.

## 2E.5 PCM metering

The shared mixer optionally accumulates AudioLevels at final rounding/saturation, preserving the existing output samples. Each channel records its maximum absolute saturated sample and counts rounded values outside signed-16-bit bounds, so legal full-scale samples and opposing contributions that cancel do not count as clipping. Fixed-size stats merge by adding frame/clipping counts and taking peak maxima. Composite PCM blocks carry their stats through the existing bounded queue; waveOut completion confirms stats only after normal buffer retirement and clock confirmation, not submission/reset. PreviewMetrics owns the latest completed buffer, its timestamp and cumulative run stats under the existing mutex. UI polling never blocks on decoding or queues. Bars blank after 250 ms without confirmed buffers; peak hold/count stay latched for the run. Offline mix emits cumulative stats only after successful PCM writes, including final silence padding. Progress observation retains stats through subsequent stages in the GUI job, and reports pre-AAC PCM only. No schema changes, persistent telemetry, limiter, additional decode pass or audio device changes.

## 2E.6 canonical PCM source mapping

Both preview and export use media.PCMSourceFrame and media.PCMDecodeArgs. Source sample index at run-relative frame n is floor((source in + run start - timeline start)*48000/1000000)+n. Signed floor and overflow checks avoid microsecond-rounding differences; clip boundaries are ceil-quantized on the run grid. Export run start is zero. For preview starts on the exact shared sample grid (integer microsecond starts divisible by 125), the source indices and automation timestamps agree with the corresponding export suffix. Other starts use a run-relative grid, with less than one sample period of grid phase difference; no claim of identical arbitrary off-grid seek PCM. FFmpeg normalizes container timestamps with copyts/start_at_zero, resamples from source origin, pads to the required sample end, trims exact sample interval, resets output PTS, and emits 48 kHz stereo s16le. Preview checks both timeline and source continuity, bounds the stream to clip end, verifies EOF/process success on normal completion, and reaps on cancellation or discontinuity. Offline retains its strict source identity and process lifecycle. Starting from source origin avoids seek-dependent AAC priming/resampler state but increases cold late-seek cost; no fast approximate fallback. Decoder process count remains capped at eight and raw buffers remain block-bounded. A future source-indexed/checkpointed PCM optimization must preserve the canonical sample contract. No schema or public dependency changes. Physical audio acceptance and actual GPU work are not implied by automated sample parity.

## 3A.1 acceleration boundary

The media package owns a bounded FFmpeg capability report, explicit stage identifiers (decode, geometry, composite, encode) and a fail-closed backend decision policy. The inventory parses known hardware methods, device types, HEVC decoders/encoders and relevant filters. Optional device initialization uses a tiny generated frame with no user source; success proves only that initialization returned successfully. Per-command timeout, total timeout and output cap bound diagnostics. The GUI runs the report through the existing job/epoch/fingerprint ownership and uses the configured engine snapshot. No startup scan, persistent cache, new dependency, preference/schema migration, or automatic backend change. CPU and auto both resolve to CPU; a forced unqualified backend is rejected. This is a policy/diagnostic boundary, not a replacement of existing decoder/compositor/encoder interfaces. Subsequent stages will integrate the selection at actual execution entrypoints only after qualification. See docs/gpu-architecture.md.
