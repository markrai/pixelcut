import contextlib
import importlib.util
import io
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("compare", Path(__file__).with_name("compare-refactor-benchmarks.py"))
compare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(compare)


class BenchmarkComparisonTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name) / "bench.txt"

    def write_samples(self, timings=(10, 20, 30, 40, 50), allocation=8):
        lines = [
            f"{name} 100 {timing} ns/op 32 B/op {allocation} allocs/op"
            for timing in timings
            for name in sorted(compare.EXPECTED)
        ]
        self.path.write_text("\n".join(lines), encoding="utf-8")

    def test_medians_and_zero_baseline(self):
        self.write_samples(allocation=0)
        baseline = compare.parse(self.path)
        self.write_samples((20, 40, 60, 80, 100))
        candidate = compare.parse(self.path)
        with contextlib.redirect_stdout(io.StringIO()) as output:
            compare.compare(baseline, candidate)
        report = output.getvalue()
        self.assertEqual(report.count("+100.0%"), 21)
        self.assertEqual(report.count("n/a (zero baseline)"), 21)
        self.assertIn("| large/clone | ns/op | 30 | 60 | +100.0% |", report)

    def test_missing_samples_rejected(self):
        self.write_samples((10, 20, 30, 40))
        with self.assertRaisesRegex(ValueError, "expected five samples"):
            compare.parse(self.path)

    def test_duplicate_samples_rejected(self):
        self.write_samples((10, 20, 30, 40, 50, 60))
        with self.assertRaisesRegex(ValueError, "expected five samples"):
            compare.parse(self.path)

    def test_invalid_metric_rejected(self):
        self.write_samples()
        self.path.write_text(self.path.read_text().replace("32 B/op", "nan B/op"), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "invalid metric"):
            compare.parse(self.path)

    def test_unexpected_workload_rejected(self):
        self.write_samples()
        self.path.write_text(self.path.read_text().replace("/clone-2", "/unknown-2"), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "unexpected benchmark row"):
            compare.parse(self.path)


if __name__ == "__main__":
    unittest.main()
