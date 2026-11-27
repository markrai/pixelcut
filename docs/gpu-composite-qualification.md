# 3A.4 HDR composition qualification

## Boundary and non-goals

3A.4a adds a test-only oracle matrix and candidate comparator next to the existing projectexport compositeHLG implementation. No new compositor, production dispatch, dependency, project schema or GPU code. Existing geometry is CPU-only; experimental CUDA decode remains independent. Do not conflate SDR preview compositing with HDR export. The input is normalized planar yuv444p10le, limited-range BT.2020 nonconstant-luminance HLG, integer rectangles and percent opacity, bottom-to-top ordering, up to four layers. The output is the same planar canvas before encoder chroma subsampling. Decode, scaling, tone mapping, encoding and audio are not measured by this harness.

## Qualification contract

- All pixels uncovered or covered first by an opaque layer must be exact, including source excursion codes. The fixture comparator derives the expected code from the topmost nonzero covering layer independently of the production compositor. Uncovered black is (64,512,512).
- Mixed pixels must remain legal limited-range 10-bit and differ by at most one code in each channel against compositeHLG (provisional candidate screen, not a physical HDR display claim). No tolerance for opaque or black pixels. Alpha ordering and single final quantization follow docs/project-export.md.
- The deterministic nine-case matrix covers gaps, odd rectangle placement, excursion bypass, 50% white analytic anchors, overlapping half-white layers, zero opacity, opaque over translucent, disjoint layers and four-layer gradients with 1%/99% opacity. Repeated CPU evaluation must be exact; the comparator's one-code mixed allowance is for future candidates, not CPU repeatability.
- Comparator rejects wrong frame lengths, exact-path deviations, out-of-range mixed samples and more than one-code mixed deviations. Future GPU tests must also verify actual transfer representation, device identity, cancellation/reaping, no reuse of stale output and resource ceilings.

## CPU baseline

```cmd
go test -count=1 -v ./internal/projectexport -run "^TestHLGQualification"
go test -run "^$" -bench "^BenchmarkHLGQualificationCPU$" -benchmem -benchtime=3x ./internal/projectexport
```

The benchmark compares two full-canvas opaque layers vs 100% bottom + 50% top at 640x360, 1920x1080 and 3840x2160, using deterministic 10-bit patterns. Input allocation and black initialization are outside timing; full opaque bottom ensures complete destination coverage on every iteration. Output byte rate is a normalization metric, not measured memory bandwidth. This is composition-only; no FFmpeg process startup, GPU transfers or encoder overhead. Record machine, Go version, repetitions and allocation measurements. Large 4K cases are opt-in benchmarks, not required ordinary tests. Candidate GPU timings must include transfers and setup separately and report comparable end-to-end cost.

3A.4b will choose and qualify a GPU candidate at this exact boundary. Do not enable a GPU compositor until exact-path and mixed-pixel checks, independent physical timing and integration gates pass. 3A.4c and 3A.4d remain pending.
