# 2C.R.9: Regression and performance qualification

Status: evidence collection started; neither performance nor native runtime acceptance is complete.

## Automated evidence

The standard CI must pass on the exact candidate (format, tests, vet, race,
Windows build and desktop tests, synthetic media tests and coverage). Record
the run URL and candidate SHA. The 2C.R.8 predecessor also requires its own
successful verification before accepting this phase.

The Refactor performance qualification workflow runs the unchanged R.1
project benchmark fixture at `eff81c55b65ffee528bd7ad636b3fb2937cbe642`
and the candidate in the same runner and worktree path, with the same Go
version, GOMAXPROCS, benchmark duration and CPU setting. The R.1 revision is
the first revision containing this workload; the earlier pre-refactor
reference `94f0402a59aa543beb0383e40151e145bea01ec6` is not itself a
benchmark-bearing revision. Five alternating runs reduce order bias. Raw
samples, environment and median changes are uploaded. No arbitrary
performance threshold is imposed. Review all three metrics, raw variation
and any meaningful regressions before accepting. Composition benchmarks
have no pre-extraction equivalent and are reported separately by normal CI.
These metadata-only workloads do not measure Win32 paint, process startup,
source I/O, decoding, encoding or HDR output.

## Native Windows acceptance

Use disposable copies of projects and media; keep original recordings and
existing recovery data safe. Use a separate Windows test account for
recovery, forced termination and cache tests. Record exact revision, OS,
CPU/GPU/RAM, display and HDR settings, Go, FFmpeg, FFprobe and mpv versions.
Run `qualify.bat` on an interactive desktop and retain its output; confirm
neither equivalence test skipped. Record each result as PASS, FAIL, or NOT RUN
with reproduction steps and evidence. Never count NOT RUN as PASS.

| ID | Scenario | Expected |
| --- | --- | --- |
| R9-01 | Open version 1 and version 2 projects; save/reopen copies | Existing edits, IDs, tracks, geometry and source references preserved |
| R9-02 | Multi-select, trim, ripple, split, copy/paste, transfer and undo/redo | Atomic edits, stable selection and correct saved dirty state; rejected overlap changes nothing |
| R9-03 | Marquee, cancel drag, capture loss, zoom, scroll, ruler seek | Paint and hit targets agree; canceled gestures leave committed edits unchanged |
| R9-04 | Start before first clip, in a gap, within clip, at final end | Start at next clip/source in-point or marker; no wrap at final end |
| R9-05 | Pause, resume, scrub across clips, step VFR frames, stop/restart rapidly | Correct project/source mapping; no stale frame/clock events or crash |
| R9-06 | Switch active tracks, use independent audio track | No unintended mixing or compositing; unsupported playback explicitly identified |
| R9-07 | Import, change or remove media while scanning, relink, cancel a job and start another | CHECKING/offline state resolves; stale jobs do not commit; sources remain untouched |
| R9-08 | Rapid edits and close; restart; force termination after checkpoint; corrupt a disposable recovery slot | Latest successful snapshot restores; last valid slot remains usable; no silent claim of durability |
| R9-09 | Clear cache while generating tiles, change tools, close during media work | No stale result or cache clear race; deadlines/warnings respected; no hung UI |
| R9-10 | Native crop/transform equivalence and real Pixel landscape/portrait HLG footage | Follow [media gates](qualification.md); verify geometry, VFR timing, color, A/V sync and HDR/SDR viewing separately |
| R9-11 | Large project interaction, preview startup, recovery and shutdown | Measure baseline and candidate on same machine/fixtures; investigate significant regressions |

For R9-11 use the same source files, clip counts, track layout, tool versions,
cache state and display on both revisions. Compare cold and warm preview
startup, seek-to-visible-frame latency, drag responsiveness, memory and
shutdown time. Take five measurements per repeatable operation and retain
raw values and medians, including failures/timeouts. If an operation is not
available in the baseline, mark it not comparable. Record actual UI stalls
rather than treating a successful build as responsiveness evidence.

Explicit open/save, preferences/recent persistence, startup recovery read,
single-source availability checks and final relink validation remain
synchronous after R.8. These are known limits, not evidence of an entirely
nonblocking UI. Continuous composition, independent audio playback and
whole-project export are outside this phase.

## Evidence record

- Candidate revision:
- R.8 CI URL:
- Candidate CI URL:
- Paired benchmark workflow URL and artifact:
- Performance review and regressions:
- Windows environment and media tools:
- Native equivalence log:
- R9-01 through R9-11 outcomes and evidence:
- Real Pixel media qualification and HDR display findings:
- Remaining blockers and accepted deviations:

Keep the roadmap gate open until all required evidence is reviewed. Any
regression fix should be independently tested; no new product features
during stabilization. Do not upload private recordings, absolute personal
paths, or recovery files to GitHub.
