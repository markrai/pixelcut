# Refactoring baseline

Reference revision: `94f0402a59aa543beb0383e40151e145bea01ec6`.
[Pre-refactoring CI](https://github.com/markrai/pixelcut/actions/runs/45683622460)
passed. Static source review measured 878 lines in main_windows.go, 457 in
timeline_windows.go, 58 editor field declarations and approximately 64% of
production Go bytes in the Windows GUI. Those observations describe the
unformatted reference revision, not a target to optimize for line count.

## Reproducible workload

The project package generates deterministic, metadata-only fixtures. No real
media, private paths or giant checked-in JSON files are required.

| Fixture | Total tracks | Assets | Clips |
| --- | ---: | ---: | ---: |
| Small | 2 | 4 | 12 |
| Medium | 6 | 30 | 240 |
| Large | 12 | 100 | 2400 |

Each track has ordered, nonoverlapping one-second intervals with one-second
gaps, and tracks overlap in project time. Video clips include representative
crop and transform properties; audio clips use the same source asset metadata.
All fixtures use valid version-2 project state and simulated absolute paths.

Run from repository root:

```sh
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./internal/...
go test -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go test ./internal/project -run '^$' -bench '^BenchmarkProjectBaseline$' -benchmem -benchtime=200ms -count=3
```

Windows GUI coverage is produced separately on the Windows CI runner.
CI uploads raw coverage profiles, per-function totals, package test output and
benchmark output as artifacts. A short CI benchmark is a reference observation,
not a stable performance threshold. Re-run on the same hardware and Go/tool
versions for meaningful before/after comparisons. Record CPU/OS/Go version,
fixture size, ns/op, B/op and allocs/op. Run with a longer benchtime and repeat
when investigating a regression.

Benchmarks measure validation, cloning, single clip snapping, group movement,
splitting, JSON encoding and JSON decoding/validation. They exclude actual
decode, UI painting, OS dialog latency, disk sync and native preview startup.
These are separate runtime measurements for 2C.R.9.

## Qualification evidence

- Baseline automated build/test: passed at the reference revision.
- Formatting, race, coverage and benchmark measurements for the new baseline:
  produced by the 2C.R.1 CI run. Do not assume results until that run finishes.
- Real Pixel footage, native embedded preview, physical HDR output and
  large-project UI latency: not measured in this phase.
- Native preview equivalence is opt-in via qualify.bat, not a regular CI gate.
  Follow [qualification.md](qualification.md) before marking it qualified.

There is no arbitrary percentage coverage threshold in this baseline. Future
slices must add focused regression tests and record significant coverage changes.
