"""Score saved predictions without contacting either provider."""
import argparse
import hashlib
import json
from pathlib import Path
from statistics import median

from run import HERE, OPTIONS


def score(path, cases, labels, digest):
    rows = [json.loads(line) for line in path.read_text().splitlines()]
    if not rows:
        raise ValueError(f"{path}: no observations")
    pairs = {}
    for row in rows:
        key = row["id"], row["run"]
        if key in pairs or row["corpus_sha256"] != digest:
            raise ValueError(f"{path}: duplicate observation or different corpus")
        pairs[key] = row
    expected = {(case["id"], order) for case in cases for order in (1, 2)}
    if set(pairs) != expected:
        raise ValueError(f"{path}: missing or unexpected observation")
    held = [case["id"] for case in cases if case["split"] == "held-out"]
    weak = {cid for cid in held if labels[cid] != "addresses"}
    first = {cid: pairs[cid, 1] for cid in held if "choice" in pairs[cid, 1]}
    ranked = sorted(first, key=lambda cid: first[cid]["scores"]["addresses"])
    comparable = [cid for cid in first if "choice" in pairs[cid, 2]]
    wrong = [cid for cid in first if first[cid]["choice"] != labels[cid]]
    false_reassurance = [cid for cid in first if cid in weak and first[cid]["choice"] == "addresses"]
    flips = [cid for cid in comparable if first[cid]["choice"] != pairs[cid, 2]["choice"]]
    costs = [row["usage"]["cost"] for row in rows if "cost" in (row.get("usage") or {})]
    timings = [row["ms"] for row in rows if "choice" in row]
    return dict(
        provider=rows[0]["provider"], models=sorted({row["model"] for row in rows if row.get("model")}),
        held_out=len(held), first_pass_answers=len(first), correct=len(first)-len(wrong),
        weak_answered=len(weak.intersection(first)), false_reassurance=false_reassurance,
        weak_in_first_ten=len(weak.intersection(ranked[:10])), inspected=min(10,len(ranked)),
        comparable=len(comparable), order_flips=flips, wrong=wrong,
        baseline_false_reassurance=len(weak), baseline_weak_in_first_ten=len(weak.intersection(held[:10])),
        all_calls=len(rows), failures=sum("error" in row for row in rows),
        median_ms=median(timings) if timings else None, cost_usd=sum(costs) if costs else None,
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", nargs="+", type=Path)
    parser.add_argument("--labels", type=Path, help="Human-reviewed JSON object mapping every case ID to a label")
    args = parser.parse_args()
    corpus = (HERE / "cases.json").read_bytes()
    cases = json.loads(corpus)
    labels = {case["id"]: case["provisional_label"] for case in cases}
    if args.labels:
        reviewed = json.loads(args.labels.read_text())
        if reviewed.keys() != labels.keys() or not set(reviewed.values()).issubset(OPTIONS):
            parser.error("Supply one recognized label per case ID")
        labels = reviewed
    summary = [score(path, cases, labels, hashlib.sha256(corpus).hexdigest()) for path in args.results]
    print(json.dumps(dict(labels="reviewed" if args.labels else "provisional", results=summary), indent=2))


if __name__ == "__main__":
    main()
