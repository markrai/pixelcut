# PixelCut

A small, source-aware video editor for people who want to cut and crop footage without fighting color management.

PixelCut starts with a command-line media engine. Its first goal is to qualify the original HDR recording, preview, and export path before building the desktop timeline.

## Quick start

Requirements:

- Go 1.23 or newer
- FFmpeg and FFprobe on PATH (FFmpeg must include libx265)
- Optional: mpv with gpu-next support for preview (Windows installer locations are searched if PATH lookup fails)

Build:

```sh
go build -o pixelcut.exe ./cmd/pixelcut
```

On Linux or macOS, use `-o pixelcut` instead.

### Inspect an original recording

```powershell
.\pixelcut.exe inspect "C:\Videos\original.mp4"
```

Inspect reports codec, pixel format, color primaries, transfer, matrix, range, geometry, stream metadata and timing. It does not modify the recording.

### Preview

```powershell
.\pixelcut.exe preview "C:\Videos\original.mp4"
.\pixelcut.exe preview -hdr "C:\Videos\original.mp4"
```

The default requests an SDR Rec.709 preview. The HDR option requests native output on a compatible display and platform. Neither option guarantees a match to the phone's rendering. HDR preview requires physical display qualification.

### Remux without re-encoding

```powershell
.\pixelcut.exe remux original.mp4 copy.mp4
```

Copies audiovisual streams into a new MP4 without re-encoding. Cardinal display rotation is preserved. Recognized Pixel MetaHandle/mett timed metadata is excluded with a warning; unknown auxiliary streams are rejected. Global and stream metadata and chapters are stripped, including the observed GPS tags. This is not a byte-for-byte copy of the original file.

### Trim or crop

```powershell
.\pixelcut.exe trim -start 2.5 -duration 8 original.mp4 trimmed.mp4
.\pixelcut.exe crop -x 120 -y 0 -width 3600 -height 2160 original.mp4 cropped.mp4
```

The crop example assumes landscape 3840 x 2160 source footage. Crop coordinates refer to the displayed orientation. For a portrait recording displayed at 1080 x 1920, combine trim and crop in one encode: 

```powershell
.\pixelcut.exe crop -start 2 -duration 5 -x 40 -y 40 -width 1000 -height 1840 original.mp4 cropped-trimmed.mp4
```

 Crop coordinates and dimensions must be even for 4:2:0 chroma alignment. Start and duration are expressed in seconds. Trim re-encodes video and audio; crop re-encodes video but copies audio.

The renderer accepts explicitly signaled, square-pixel, 10-bit 4:2:0 BT.2020 HLG with supported chroma location and optional pure cardinal display rotation. Re-encoding normalizes rotation into the pixels. Unknown side data and auxiliary streams are rejected; recognized Pixel MetaHandle/mett streams are excluded with a warning. Multiple audio tracks are not yet supported by the renderer. Export writes to a temporary file, checks the output against the color contract, then publishes without overwriting an existing file. The destination filesystem must support hard links (e.g. NTFS).

Re-encoded output uses HEVC Main 10 through libx265 and CRF 16. It is not mathematically lossless. Global and stream metadata and chapters are intentionally omitted from exports. This is not a guarantee of comprehensive privacy sanitization. Per-frame dynamic HDR metadata is not yet qualified. Do not delete source recordings.

## Tests

```sh
go test -v ./...
go vet ./...
```

Integration tests generate small synthetic HLG-tagged videos, including a rotated fixture with a simulated Pixel metadata track. They exercise remux, trim, crop and combined edits, check output signaling and geometry, and compare decoded audiovisual frame hashes for the rotated remux. FFmpeg and FFprobe must be available for integration tests to run. This fixture is not a substitute for camera footage or calibrated HDR display testing.

## Roadmap

1. Qualify the integrated engine on actual Pixel footage on Windows. Direct FFmpeg experiments passed provisionally; Windows Media Player showed an abrupt contrast change in edited clips that was not observed in VLC.
2. Establish an SDR export transform and evaluate it against the original phone and reference playback.
3. Introduce a non-destructive project model, frame-accurate timeline, media bin and basic video/audio lanes.
4. Add native desktop preview, transitions and overlays only after the baseline is stable.
5. Add hardware-accelerated decode and encode with the same fidelity tests as the software path.

See [media architecture](docs/architecture.md) and [qualification procedure](docs/qualification.md).
