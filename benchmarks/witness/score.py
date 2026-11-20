"""Score the saved paired sessions without inference or network access."""
import argparse
from collections import Counter
import json
from pathlib import Path

from corpus import SUITES, read_manifest
from diagnostics import assess, index_sessions


def score(cases, rows):
    lookup = index_sessions(cases, rows)
    counts = {arm: Counter(yes=0, no=0, invalid=0, transport=0) for arm in ("plain","dlktk")}
    pairs = Counter(both=0, plain_only=0, dlktk_only=0, neither=0, unavailable=0)
    details = []
    for c in cases:
        answers = {}
        missing = False
        measurements = {}
        for arm, tally in counts.items():
            r = lookup[c["id"], arm]
            measurements[arm] = assess(c, r)
            final = measurements[arm][1].get("final_accuracy")
            if final is None:
                tally["transport"] += 1
                missing = True
            elif r["answer"] not in ("yes", "no"):
                tally["invalid"] += 1
            elif final["score"] == 1:
                tally[c["answer"]] += 1
            answers[arm] = r["answer"]
        right = [arm for arm in answers if answers[arm] == c["answer"]]
        category = "unavailable" if missing else {0:"neither", 2:"both"}.get(len(right))
        if category is None:
            category = right[0] + "_only"
        pairs[category] += 1
        assisted = lookup[c["id"],"dlktk"]
        graph, metrics = measurements["dlktk"]
        exact = graph["exact"] if graph is not None and assisted.get("tool_result") else None
        details.append(dict(id=c["id"], family="chain" if len(c["shape"]) == 1 else "branching",
                            gold=c["answer"], **answers, pair=category, exact_graph=exact,
                            capture=graph, metrics=metrics, error=assisted.get("error")))
    resources = {}
    for arm in counts:
        runs = [r for r in rows if r["arm"] == arm]
        calls = [call for r in runs for call in r["calls"]]
        costs = [call["usage"].get("cost") for call in calls]
        resources[arm] = {
            "models": sorted({call["model"] for call in calls}),
            "calls": len(calls),
            "prompt_tokens": sum(call["usage"]["prompt_tokens"] for call in calls),
            "output_tokens": sum(call["usage"]["completion_tokens"] for call in calls),
            "cost_usd": round(sum(costs), 7) if all(c is not None for c in costs) else None,
            "session_seconds": sum(r["ms"] for r in runs) / 1000,
        }
    groups = {family: summarize([r for r in details if r["family"] == family])
              for family in ("chain", "branching")}
    return dict(correct_by_gold=counts, pairs=pairs, resources=resources, groups=groups,
                diagnostics=summarize(details), cases=details)


def summarize(rows):
    totals = {}
    for r in rows:
        for key, metric in r["metrics"].items():
            tally = totals.setdefault(key, {"sum": 0, "n": 0})
            tally["sum"] += metric["score"]
            tally["n"] += 1
    inexact = [r["metrics"]["tool_verdict_correct"]["score"] for r in rows
               if r["capture"] is not None and not r["capture"]["exact"] and "tool_verdict_correct" in r["metrics"]]
    graphs = [r["capture"] for r in rows if r["capture"] is not None]
    return dict(cases=len(rows), pairs=Counter(r["pair"] for r in rows),
                correct={arm: sum(r[arm] == r["gold"] for r in rows) for arm in ("plain","dlktk")},
                metrics=totals, missing=sum(len(g["missing"]) for g in graphs),
                extra=sum(len(g["extra"]) for g in graphs), duplicates=sum(g["duplicates"] for g in graphs),
                preserved_inexact_verdicts={"sum": sum(inexact), "n": len(inexact)})


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", type=Path)
    parser.add_argument("--suite", choices=SUITES, default="pilot")
    args = parser.parse_args()
    print(json.dumps(score(read_manifest(args.suite),
                           [json.loads(s) for s in args.results.read_text().splitlines()]),indent=2))
