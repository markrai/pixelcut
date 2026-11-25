# Media architecture

## Design principle

Source color is part of the media contract, not an aesthetic preset. Untagged or contradictory media must not be silently converted. Original files are read-only inputs.

## Initial architecture

- `cmd/pixelcut`: command parsing and process cancellation.
- `internal/media`: FFprobe inspection, HLG input validation, edit validation, FFmpeg argument construction, output verification and preview process invocation.
- External tools: FFmpeg and FFprobe perform inspection and processing; mpv provides an independent display experiment. They are not bundled.

The core is a dependency-free Go module. External processes are invoked with argument arrays rather than shell commands. The future GUI can call the media package or a stable command protocol. Keep the GUI independent from encoder implementation details.

## Color contract

| Field | Initial supported render value |
| --- | --- |
| Pixel format | yuv420p10le |
| Primaries | bt2020 |
| Transfer | arib-std-b67 (HLG) |
| Matrix | bt2020nc |
| Range | tv or pc, explicitly signaled |
| Chroma location | left or center |
| Sample aspect ratio | 1:1 |

Both FFmpeg output options and x265 VUI options explicitly carry the color signal. After encoding, FFprobe verifies codec, color fields and geometry. The process fails closed if those fields change. Tag validation is necessary but insufficient for visual fidelity: it does not independently verify every sample, metadata layer or display transformation.

The crop path operates on 10-bit YUV samples without an intentional transfer or gamut conversion. Trim uses output seeking and preserves variable frame cadence where possible. Exact timing, A/V synchronization and variable frame rate behavior require additional qualification. A fast keyframe-aware trim path is deferred until its boundary behavior is tested.

Remux preserves compressed streams without re-encoding. The current verification checks the video contract and stream count; it does not exhaustively compare all container or auxiliary metadata. The integration fixture also compares decoded video frame hashes and timestamps.

## Future timeline

Represent assets separately from clips. Each clip references an asset, source in/out timestamps, timeline position, track, crop rectangle and audio gain. Store timestamps as integers in an explicit timebase, never floating-point timeline positions. Projects store references to original media and edits; exports are derived assets.

Before a multitrack compositor is implemented, define the working color space, transfer handling for overlays and mixing, HDR/SDR policy, and deterministic tests. Do not mix gamma-encoded HDR and SDR layers without a defined transform.

## Non-goals for the initial milestone

Color grading, effects, GPU encoding, dynamic HDR, SDR export, embedded preview and a graphical timeline. The initial CPU encode path is a reference implementation, not a performance target.

## Distribution

Choose a project license before publishing releases. FFmpeg, x265 and mpv have their own licensing obligations; review them before distributing bundled binaries or linking their libraries. The current project expects separately installed tools.
