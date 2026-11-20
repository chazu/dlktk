"""Present saved witness measurements as local DeepEval test cases (optional)."""
# /// script
# requires-python = ">=3.10"
# dependencies = ["deepeval==4.3.4"]
# ///
import argparse
from collections import defaultdict
import json
import os
from pathlib import Path
from tempfile import TemporaryDirectory

from corpus import SUITES, read_manifest
from diagnostics import assess, index_sessions


def evaluate_rows(cases, rows):
    from deepeval import evaluate
    from deepeval.config.settings import get_settings
    from deepeval.evaluate.configs import AsyncConfig, CacheConfig, DisplayConfig
    from deepeval.metrics import BaseMetric
    from deepeval.test_case import LLMTestCase

    get_settings().CONFIDENT_API_KEY = None  # Keep saved testimony in this process.

    class WitnessMetric(BaseMetric):
        threshold = 1.0

        def __init__(self, key):
            self.key = key

        @property
        def __name__(self):
            return self.key

        def measure(self, test_case, *args, **kwargs):
            metric = test_case.metadata["measurements"][self.key]
            self.score, self.reason = metric["score"], metric["reason"]
            self.success = self.score >= self.threshold
            return self.score

        async def a_measure(self, test_case, *args, **kwargs):
            return self.measure(test_case)

        def is_successful(self):
            return self.success

    sessions = index_sessions(cases, rows)
    groups = defaultdict(list)
    observations = {}
    for c in cases:
        for arm in ("plain", "dlktk"):
            r = sessions[c["id"], arm]
            _, measurements = assess(c, r)
            name = f"{arm}/{c['id']}"
            observations[name] = {"metrics": {}, "error": r.get("error")}
            if measurements:
                groups[tuple(measurements)].append(LLMTestCase(
                    name=name, input=f"Witness case {c['id']}", expected_output=c["answer"],
                    actual_output=r["answer"] or "no final answer", metadata={"measurements": measurements}))
    for keys, tests in groups.items():
        results = evaluate(tests, [WitnessMetric(k) for k in keys],
                           async_config=AsyncConfig(run_async=False),
                           cache_config=CacheConfig(write_cache=False, use_cache=False),
                           display_config=DisplayConfig(print_results=False, show_indicator=False, inspect_after_run=False))
        for test in results.test_results:
            observations[test.name]["metrics"] = {m.name: {"score": m.score, "reason": m.reason}
                                                  for m in test.metrics_data}
    return observations


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", type=Path)
    parser.add_argument("--suite", choices=SUITES, default="pilot")
    parser.add_argument("--output", type=Path, required=True, help="New per-session metric report")
    args = parser.parse_args()
    cases = read_manifest(args.suite)
    rows = list(map(json.loads, args.results.read_text().splitlines()))
    os.environ["DEEPEVAL_TELEMETRY_OPT_OUT"] = "YES"
    os.environ["DEEPEVAL_DISABLE_DOTENV"] = "1"
    os.environ.pop("CONFIDENT_API_KEY", None)
    cwd = Path.cwd()
    with TemporaryDirectory() as work:
        os.chdir(work)
        try:
            report = evaluate_rows(cases, rows)
        finally:
            os.chdir(cwd)
    with args.output.open("x") as out:
        json.dump(report, out, indent=2)
        out.write("\n")
    print(f"Saved {len(report)} sessions to {args.output}")


if __name__ == "__main__":
    main()
