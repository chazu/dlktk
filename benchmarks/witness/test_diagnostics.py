"""Regression checks from the frozen pilot and a reversed attack."""
import json
import unittest

from corpus import HERE, read_manifest
from diagnostics import assess
from score import score


class WitnessDiagnostics(unittest.TestCase):
    def test_pilot(self):
        rows = list(map(json.loads, (HERE / "pilot.jsonl").read_text().splitlines()))
        report = score(read_manifest("pilot"), rows)
        totals = report["diagnostics"]
        self.assertEqual(totals["correct"], {"plain": 5, "dlktk": 7})
        self.assertEqual(totals["metrics"]["exact_distinct_graph"], {"sum": 8, "n": 10})
        self.assertEqual(totals["metrics"]["verdict_agreement"], {"sum": 8, "n": 9})
        self.assertEqual(totals["preserved_inexact_verdicts"], {"sum": 1, "n": 2})
        duplicate = next(r for r in report["cases"] if r["id"] == "0_2_3_6")
        self.assertTrue(duplicate["capture"]["exact"])
        self.assertEqual(duplicate["capture"]["duplicates"], 2)
        self.assertNotIn("verdict_agreement", duplicate["metrics"])

    def test_reversal_counts_both_differences(self):
        case = dict(root="A", edges=[["B","A"]], answer="no")
        row = dict(arm="dlktk", answer=None, submission=dict(root="B", attacks=[dict(attacker="A",target="B")]))
        graph, metrics = assess(case, row)
        self.assertEqual(graph["missing"], [("B","A")])
        self.assertEqual(graph["extra"], [("A","B")])
        self.assertEqual(metrics["attack_recall"]["score"], 0)
        self.assertEqual(metrics["root_correct"]["score"], 0)

    def test_unavailable_and_missing_answers(self):
        case = dict(root="A", edges=[["B","A"]], answer="no")
        row = dict(arm="dlktk", answer=None, error={"stage":"transport"})
        self.assertEqual(assess(case, row), (None, {}))
        row.update(tool_result={"grounded_label":"OUT"}, error={"stage":"final"})
        _, metrics = assess(case, row)
        self.assertEqual(metrics["verdict_agreement"]["score"], 0)
        self.assertEqual(metrics["final_accuracy"]["score"], 0)
        self.assertNotIn("exact_distinct_graph", metrics)


if __name__ == "__main__":
    unittest.main()
