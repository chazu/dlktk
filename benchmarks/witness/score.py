"""Score the saved paired sessions without inference or network access."""
import argparse
from collections import Counter
import json
from pathlib import Path

from corpus import HERE


def score(cases, rows):
    lookup = {(r["id"], r["arm"]): r for r in rows}
    expected = {(c["id"], arm) for c in cases for arm in ("plain","dlktk")}
    if set(lookup) != expected or len(lookup) != len(rows):
        raise ValueError("Expected exactly one session per case and arm")
    counts = {arm: Counter(yes=0, no=0, invalid=0, transport=0) for arm in ("plain","dlktk")}
    pairs = Counter(both=0, plain_only=0, dlktk_only=0, neither=0, unavailable=0)
    details = []
    for c in cases:
        answers = {}
        missing = False
        for arm, tally in counts.items():
            r = lookup[c["id"], arm]
            if r.get("error", {}).get("stage") == "transport":
                tally["transport"] += 1
                missing = True
            elif r["answer"] not in ("yes", "no"):
                tally["invalid"] += 1
            elif r["answer"] == c["answer"]:
                tally[c["answer"]] += 1
            answers[arm] = r["answer"]
        right = [arm for arm in answers if answers[arm] == c["answer"]]
        category = "unavailable" if missing else {0:"neither", 2:"both"}.get(len(right))
        if category is None:
            category = right[0] + "_only"
        pairs[category] += 1
        assisted = lookup[c["id"],"dlktk"]
        submission = assisted.get("submission")
        exact = None
        if assisted.get("tool_result"):
            edges = sorted([a["attacker"], a["target"]] for a in submission["attacks"])
            exact = submission["root"] == c["root"] and edges == c["edges"]
        details.append(dict(id=c["id"], gold=c["answer"], **answers, pair=category, exact_graph=exact))
    return dict(correct_by_gold=counts, pairs=pairs, cases=details)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", type=Path)
    args = parser.parse_args()
    print(json.dumps(score(json.loads((HERE / "cases.json").read_text()),
                           [json.loads(s) for s in args.results.read_text().splitlines()]),indent=2))
