# Pixel footage qualification

Use an original, unmodified camera recording. Keep all source and generated media outside version control. Test both landscape and portrait, indoor and outdoor lighting, moving subjects, skin tones and bright highlights.

## Gate A: source characterization

Run `pixelcut inspect original.mp4` and save the report locally. Record the camera model, recording setting, resolution, frame rate, codec and reported color fields. Include any rotation, auxiliary streams and side data. Do not override missing metadata to force an export. Expand support only after understanding the actual signal.

## Gate B: remux and render

1. Remux the recording without re-encoding.
2. Trim a short segment and crop another segment using even pixel coordinates.
3. Inspect all outputs. Compare color signaling, dimensions, frame timing, rotation and audio synchronization.
4. Compare decoded video frame hashes for the remux using `ffmpeg -v error -i INPUT -map 0:v:0 -f framehash -`. Compare frame timing separately if container timestamp origins differ.
5. Review the cropped and trimmed outputs at matching source timestamps. Check for unexpected range or gamut changes, banding, loss of detail and audio drift.

## Gate C: display

Compare original and exports in the same application, on the same display, using the same Windows HDR setting. Then compare in the Pixel's player and CapCut as separate visual references. Record the display, player, display HDR state, brightness, and any player tone-mapping controls. A phone screenshot is not reliable evidence of the actual HDR signal.

Test both native HDR and SDR preview. Never conclude that a color tag match proves equivalent display rendering. If the preview and exported file disagree, isolate the decode, display and encoding paths before modifying the color policy.

## Acceptance

- No unexpected conversion to 8-bit SDR.
- Remux retains decoded video frames and expected timing.
- Trim and crop preserve the supported HDR signal and expected geometry.
- No unexpected visible contrast, saturation, highlight or range shifts when viewed in the same playback pipeline.
- Audio synchronization remains acceptable and source files remain unchanged.
- Unsupported media produces a useful error rather than a misleading export.

The synthetic automated tests are a regression gate. Real Pixel footage, HDR display behavior, and reference playback agreement are separate release gates.
