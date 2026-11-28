# Video transitions (3B.1)

## First transition: cross-dissolve

An explicit transition belongs to the incoming video clip and identifies its immediately preceding clip on the same track by stable ID. The two ordinary timeline intervals must touch at a cut; same-track overlap is still invalid. The dissolve uses the first part of the incoming clip, beginning exactly at the cut and ending after the requested duration. The outgoing video samples continue from its existing exclusive source Out through an unused source tail of the same duration. Both sources must have the required footage. Do not repeat the final frame, invent a handle, move the cut or shorten the project. Duration is an integer number of microseconds, positive, at most ten seconds, at most incoming clip duration, and at most the outgoing source handle. Consecutive transitions can touch but not overlap on the same track.

At timeline tick t in [cut, cut+duration), source times are outgoing.Out+(t-cut) and incoming.In+(t-cut). Incoming linear weight is floor((t-cut)*1000000/duration), with outgoing weight the complement. At the cut the outgoing weight is unity; at the exclusive end the transition is inactive and ordinary incoming video is used. Sample on actual output frame ticks in a later rendering slice, not rounded whole-percent steps. Audio continues to use the original clip intervals and existing audio fades/envelopes, with no automatic crossfade or source extension.

## Schema and editing

Project schema 10 adds optional transition_in on the incoming clip: kind (cross-dissolve), from_clip_id, duration_us. Version 9 remains supported with its original audio envelope contract; older projects retain their version on load/save without edits. Existing feature edits that upgrade to the latest version now use version 10, as does New. Old releases reject schema 10. No UI creation yet. Setter validates clone and honors locked tracks, exact adjacency, source handles and supported kind; nil removes the transition. Clipboard copying requires both endpoints, and paste remaps both IDs. Splitting a participant is rejected until transition removal. Group move/transfer is valid only when adjacency/handle validation remains satisfied. Deleting the incoming clip removes its owned transition; deleting only the outgoing clip fails if the incoming transition would be orphaned. Relink/trim that invalidates a handle fails validation. Undo/redo uses existing transactions.

## Rendering boundary

CompileTransitions snapshots immutable per-track windows, respects video visibility/solo, and binary-searches active windows without allocating per-frame schedules. It provides source intervals and exact fixed-point weights for future shared evaluation. Ordinary composition Compile still rejects projects containing transitions until preview integration is implemented. CompileForExport explicitly admits transitions; whole-project export supports the bounded subset below rather than silently exporting a hard cut. Selected-clip export also rejects a participating clip. Preview remains a deliberate temporary gate: do not hand-create schema 10 transition projects for daily editing yet.

3B.1b must integrate transition windows into shared evaluation and export spans, decode the outgoing handle with original source timestamps, and mix both track contributions in scene-linear BT.2020 HLG before composition with other tracks. Geometry/coverage and static opacity need explicit semantics; a dissolve is not equivalent to fading one translucent layer over another. Preserve ordinary opaque fast paths and bounded decoder/frame memory, source verification, cancellation and publication. Begin with bounded supported geometry/opacity cases; reject unimplemented cases explicitly. No duplicate CPU/GPU pipelines; both encoders consume identical normalized HDR frames. 3B.1c must implement matching preview semantics (SDR output remains approximate) including continuous/seek behavior. 3B.1d exposes GUI controls only after preview/export support, with native physical qualification. Crossfade audio, additional wipes, centered transitions and timeline shortening are outside the initial contract.

PR #49 merged and CI 46089908035 / performance 46089908038 passed. GPU resource and public-path physical evidence remain outstanding; no default hardware policy change.

## 3B.1b HDR export

PR #50 merged at 1a7de9383866b82a0357cc5e9af0f4b21d51e3fd; CI 46090963388 and performance 46090963422 passed.

Export explicitly opts into the immutable composition evaluator. Transition ends split edit spans; existing audio contributions retain original clip intervals and sample mapping. LayerPlan records the outgoing handle with planned geometry separately from ordinary incoming layer. Span retains the original window even when unrelated edits split it, so weights never restart. Preflight includes the outgoing asset; each decoder uses its own source index, existing HLG normalization and geometry. The decoder pair occupies two of the existing four simultaneous decoder slots; one transition at a time, both endpoints must be 100% opaque and cover the full composition canvas after geometry. Other ordinary layers may coexist within that budget. Partial coverage, nonunity endpoint opacity and simultaneous transitions fail before rendering. Hidden/solo-excluded transitions do not contribute. No extra full-frame intermediate buffer.

The shared HDR compositor substitutes a track-local opaque contribution at the incoming layer's stack position, mixing scene-linear outgoing and incoming RGB with the exact fixed-point weight sampled at the CFR frame tick. Translucent overlays above it participate in the same linear accumulation and final quantization, with no intermediate HLG rounding. Opaque uncovered endpoints retain exact source codes; mixed pixels use the existing limited-range/clamping contract. Non-transition spans use the existing opaque and mixed paths. x265 and NVENC share the resulting 4:4:4 stream. Source identity, cancellation, audio mux, output verification and no-overwrite publication unchanged. This slice makes no performance claim for dissolves.

Ordinary CI tests model/evaluator span boundaries, unsupported geometry, simultaneous transitions and decoder budget, analytical linear-light black/white midpoint (luma 828, not code-space 502), layered blend, exact endpoint, cancellation and real CPU FFmpeg HLG export with audio hash equality to a hard cut. Physical opt-in exercises all four public backend combinations on the same short synthetic fixture and requires exact pre-encode CPU/CUDA hashes. No actual GPU qualification yet.

```cmd
git pull
set PIXELCUT_QUALIFY_TRANSITION_GPU=1
go test -count=1 -v -timeout 4m ./internal/projectexport -run "^TestRenderHLGTransition$"
set PIXELCUT_QUALIFY_TRANSITION_GPU=
```

3B.1c remains preview integration, including source handle seeking, boundary sampling and explicit SDR approximation; GUI controls remain gated until preview support. GPU resource and GUI physical acceptance from 3A remain outstanding.

### 3B.1b physical fixture correction

The initial TITAN opt-in transition run failed before encoding during CUDA pre-encode frame reads (EOF). The generated source was 64x48, below the 144x144 HEVC minimum documented for Ada NVDEC ([NVIDIA Video Decoder capabilities](https://docs.nvidia.com/video-technologies/video-codec-sdk/13.1/nvdec-video-decoder-api-prog-guide/index.html#video-decoder-capabilities)). This is a likely fixture cause, not yet confirmed by a successful physical rerun. Use 192x144 for both CPU and physical GPU transition runs, matching the existing qualified CUDA project fixture; retain identical timing, sample checks and exact CPU/CUDA hash gate. Derive decoded frame byte count from dimensions. Export decoder short-read errors now abort/reap the process and retain bounded FFmpeg stderr plus clip, span, output frame and source ordinal/tick context. No automatic fallback or weakened checks. Physical transition qualification remains pending.

## 3B.1c composite preview

PR #52 merged and CI 46095107530 / performance 46095107531 passed. TITAN rerun of TestRenderHLGTransition passed in 3.95s, all four CPU/CUDA and x265/NVENC combinations, exact pre-encode parity, output/audio/cancellation gates. This supersedes the earlier pending physical export note.

Compile now admits transitions for both export and preview. LayerPlan.ActiveTransition is the shared coverage/decoder-budget gate; DecodeLayers includes the outgoing handle for source identity checks and retention. The original window controls both frame sampling and source ticks, including seeks and off-grid boundaries. Ordinary audio evaluation and source audition remain separate; participating video source audition explicitly reports that the dissolve is composite-only. The still worker and live sequence both use RenderStill. At the incoming track position, mix the two tone-mapped BGRA frames using BT.709 inverse/forward transfer approximations and fixed-point linear values, preserving exact endpoints. This is an SDR approximation after per-source tone mapping, not the tone map of the HDR export composite. Ordinary opacity and overlay behavior remain unchanged. No extra intermediate image beyond the two source frames and output; bounded transfer tables. Retain outgoing clip ID through the dissolve and retire at its end; existing stream decoder restarts only for discontinuous tick/geometry and existing cancellation handles process lifetime. Source identity checks include both contributors, including still worker publication. Existing 10fps cap and audio-master clock remain. GUI creation still deferred.

Unit tests cover midpoint, seek, off-grid boundaries, source change, audio mapping and unsupported geometry. Real HLG test checks aligned still/stream pixels, two decoder reuse/retirement, and optional exact CPU/CUDA SDR parity. Physical preview acceptance pending.

```cmd
git pull
set PIXELCUT_QUALIFY_TRANSITION_PREVIEW_GPU=1
go test -count=1 -v -timeout 3m ./internal/composition -run "^TestTransitionPreviewRealMedia$"
set PIXELCUT_QUALIFY_TRANSITION_PREVIEW_GPU=
```

Next 3B.1d: native controls and physical UI acceptance. Mixed-media 3A.7 follows before keyframes; no SDR import change here.

## 3B.1d native transition controls

PR #53 merged at 0a97bba741ebdfb7c7111a856928e404e57d8ccf; CI 46098445924 and performance 46098445883 passed. Physical preview report has not been supplied.

Right-click one incoming video clip on an unlocked enabled track. Add cross-dissolve opens a native duration menu with presets 0.1, 0.25, 0.5, 1, 2, 3, 5, 10 seconds filtered by available source tail and incoming duration; include the exact maximum and existing duration when valid. A clip with an existing transition offers duration change and removal. Removal remains available when its media is offline or the track is hidden, subject to track locking. Creation requires both sources available, exact adjacency and a supported composition plan (Build at 30 CFR; not a physical source probe). No automatic shortening, extension, source substitution or audio edits. Geometry/opacity and concurrent transition limits are checked before committing a new duration. Subsequent edits can still invalidate rendering coverage; preview/export fail closed until corrected. Each change uses the normal project transaction, persistence and undo/redo. An unchanged duration is a no-op. Marker is drawn within the incoming clip using the existing clipped timeline span; no extra drag handle or separate selection model. Jump to dissolve midpoint and show composite still after adding/resizing; removal shows cut. Regular Play defaults to composite when a transition project has no existing preview session. Explicit source preview remains available from timeline context menu; it does not render dissolves. No other preview transport semantics changed.

### Physical UI acceptance

1. After merge, run git pull and run.bat. Import two supported HLG sources (or place one source twice). Trim the first clip's source Out to leave at least 0.5s unused footage; place the second adjacent with no gap. Keep both opaque and filling the canvas.
2. Right-click the incoming clip, choose Add cross-dissolve then 0.5 s. Confirm violet band spans the first 0.5s of incoming clip, preview shows midpoint, project duration and audio unchanged. Press Space to play composite; scrub across both boundaries. On a freshly reopened project, Play should start composite playback. Explicit source preview remains an audition.
3. Change duration via context menu; verify marker width, midpoint preview and undo/redo. Remove transition and undo. Save/reopen and verify persistence. Test locked track, gap, no source tail, incompatible opacity/geometry and offline removal.
4. Export whole project in CPU mode, then NVENC if available. Confirm correct transition and audio cut; run the opt-in preview qualification command from 3B.1c separately. Record physical evidence rather than declaring it passed from CI.

Initial scope still one active fully opaque full-canvas pair, maximum four total decoders, SDR composite preview at 10 FPS. No audio crossfade, centered dissolve, drag-to-resize or SDR import support yet. Next 3A.7a mixed-media source classification and import contract before keyframes.
