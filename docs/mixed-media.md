# Mixed-media support (3A.7)

PR #54 merged at e7b04d7172815ba4150173b6c43eec5dd5ddbec3; CI 46099251467 and performance 46099251455 passed. Transition physical preview and UI evidence remains pending.

## 3A.7a import classification

Keep the existing strict HLG renderer and original FromReport constructor. FromImportReport is the explicit broader media-bin admission boundary. It uses Stream.ClassifySourceColor before creating an asset. Source bytes remain unchanged. HLG assets retain their old serialized representation and import behavior. Classified SDR/PQ assets may be saved, relinked, deduplicated and removed, but **cannot be placed on any timeline track** in this slice. Project validation also rejects a hand-authored clip using a classified bin-only asset. Unused bin assets do not affect HLG export. No conversion or source relabeling takes place.

Supported initial descriptors:

- Existing square-pixel 10-bit 4:2:0 BT.2020 HLG contract unchanged.
- H.264/HEVC planar yuv420p or yuv420p10le SDR with BT.709 primaries, matrix and transfer, tv/pc range, left/center chroma and square pixels.
- Explicit HEVC yuv420p10le BT.2020 nonconstant-luminance PQ (smpte2084) with explicit range, chroma and square pixels. Classification only; no PQ render claim.

For 8-bit planar H.264/HEVC whose longer coded side is at least 1280 and shorter side at least 720, missing/unknown/unspecified tags may be interpreted as BT.709 primaries/transfer/matrix, limited range, left chroma and square pixels. A missing SAR of 0:1 is also treated as unspecified. This is a provisional HD SDR convention, not proof of the source's intended colors. Never override a conflicting explicit tag, infer HDR, infer missing 10-bit color, or infer missing SD color. Fully tagged smaller SDR is accepted. Inference is reported in the media-bin label and persisted as flags (primaries 1, transfer 2, matrix 4, range 8, chroma 16, SAR 32); before rendering is enabled, provide a documented override/confirmation policy. No guesses about the user's specific screen recording without inspecting it. Reject unsupported codec/pixel format, odd dimensions, conflicting metadata, non-square geometry, unsupported display matrix/side data, multiple ordinary video or audio streams and unknown auxiliary streams for classified sources. Existing HLG restrictions remain intact.

Schema 11 adds optional color_family, color_defaults, codec, pixel_format, chroma fields on classified assets; existing range records effective range, and family fixes the effective primaries/transfer/matrix. Descriptor is comparable by value so clipboard, relink and dedup remain deterministic. Earlier versions 1-10 remain readable without migration on load; v10 transitions still valid. Importing a classified asset upgrades the project. New projects and latest-version feature edits use v11; older releases reject v11. Cached metadata namespace is revised to retain only rotation and the recognized Pixel metadata handler tags required for safety; old stripped reports cannot bypass rotation checks.

## Next

3A.7b: re-probe and verify descriptor against source identity; design explicit SDR preview transform and cache key, permit timeline placement only when supported consumers are ready, retain HLG behavior, gate unsupported export. 3A.7c: explicit SDR reference-white mapping into the HLG scene-linear compositor and qualification against known patches, mixed overlays, transition and audio contracts. 3A.7d: native BT.709 SDR output and documented HDR-to-SDR tone mapping. PQ render activation separately gated. Do not make GPU or color policy defaults based on inference alone.

Physical check: after merge run git pull and run.bat; import original screen recording and inspect media-bin classification. If rejected, collect .\pixelcut.exe inspect "actual-recording.mp4". It should not be placeable yet. Existing HLG footage should still place, preview and export normally. CI tests tagged/untagged classification, conflict rejection, schema v10 compatibility, v11 save/load, import-only guards, relink and real FFmpeg H.264 import. Physical mixed-media preview and export not claimed.
