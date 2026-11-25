import math
import statistics
import sys
from pathlib import Path

SIZES = ("small", "medium", "large")
OPERATIONS = ("validate", "clone", "snap", "group_move", "split", "encode", "decode_validate")
METRICS = ("ns/op", "B/op", "allocs/op")
EXPECTED = {
    f"BenchmarkProjectBaseline/{size}/{operation}-2"
    for size in SIZES
    for operation in OPERATIONS
}


def parse(path):
    samples = {name: {metric: [] for metric in METRICS} for name in EXPECTED}
    for line in Path(path).read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if not fields or not fields[0].startswith("BenchmarkProjectBaseline/"):
            continue
        name = fields[0]
        if name not in samples or len(fields) != 8 or int(fields[1]) <= 0:
            raise ValueError(f"{path}: unexpected benchmark row: {line}")
        values = dict(zip(fields[3::2], map(float, fields[2::2])))
        if set(values) != set(METRICS) or any(not math.isfinite(v) or v < 0 for v in values.values()):
            raise ValueError(f"{path}: invalid metric row: {line}")
        if values["ns/op"] <= 0:
            raise ValueError(f"{path}: invalid timing: {line}")
        for metric in METRICS:
            samples[name][metric].append(values[metric])
    for name, metrics in samples.items():
        if any(len(values) != 5 for values in metrics.values()):
            raise ValueError(f"{path}: expected five samples for {name}")
    return samples


def compare(baseline, candidate):
    print("## Paired project benchmark medians")
    print()
    print("Five alternating runs per revision; positive change means an increase. Not a performance pass/fail threshold.")
    print()
    print("| Workload | Metric | Baseline median | Candidate median | Change |")
    print("| --- | --- | ---: | ---: | ---: |")
    for name in sorted(EXPECTED):
        for metric in METRICS:
            old = statistics.median(baseline[name][metric])
            new = statistics.median(candidate[name][metric])
            delta = f"{(new / old - 1) * 100:+.1f}%" if old else ("0%" if not new else "n/a (zero baseline)")
            workload = name.removeprefix("BenchmarkProjectBaseline/").removesuffix("-2")
            print(f"| {workload} | {metric} | {old:g} | {new:g} | {delta} |")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: compare-refactor-benchmarks.py BASELINE CANDIDATE")
    compare(parse(sys.argv[1]), parse(sys.argv[2]))
