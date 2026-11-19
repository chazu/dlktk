"""Compare saved choices with the search and framing references."""
import json
import sys
from pathlib import Path

from trial import corpus, framing_cases, search_cases


def search_metrics(cases, results):
    totals = dict(paraphrase_hits=0, literal_hits=0, negatives_with_matches=0, wrong_suggestions_on_positives=0)
    for case, hits in zip(cases, results, strict=True):
        expected = set(case["expected"])
        if not expected:
            totals["negatives_with_matches"] += bool(hits)
        else:
            totals[case["kind"] + "_hits"] += bool(expected.intersection(hits))
            totals["wrong_suggestions_on_positives"] += len(set(hits) - expected)
    return totals


def top_three(row):
    if row["choice"] == "none":
        return []
    scores = row["scores"]
    return sorted((k for k in scores if scores[k] > scores["none"]), key=lambda k: -scores[k])[:3]


def evaluate(path, searches, frames):
    rows = [json.loads(line) for line in path.read_text().splitlines()]
    indexed = {r["id"]: r for r in rows}
    if len(rows) != len(indexed) or set(indexed) != {c["id"] for c in searches + frames}:
        raise ValueError(f"Incomplete or duplicate cases in {path}")
    errors = {r["id"]: r["error"] for r in rows if "error" in r}
    if errors:
        return dict(errors=errors)
    search = search_metrics(searches, [top_three(indexed[c["id"]]) for c in searches])
    frame = dict(pair_hits=0, exclusive_hits=0, unclear_hits=0, wrong_prompts=[], unnecessary_abstentions=[])
    for case in frames:
        actual = indexed[case["id"]]["choice"]
        if actual not in case["expected"]:
            field = "unnecessary_abstentions" if actual == "unclear" else "wrong_prompts"
            frame[field].append(case["id"])
        elif actual == "unclear":
            frame["unclear_hits"] += 1
        elif actual == "exclusive":
            frame["exclusive_hits"] += 1
        else:
            frame["pair_hits"] += 1
    return dict(search=search, framing=frame)


if __name__ == "__main__":
    searches, frames = list(search_cases()), list(framing_cases())
    print("Corpus nodes:", len(corpus()))
    print("Substring:", search_metrics(searches, [c["literal"] for c in searches]))
    print("Candidate shortlist:", search_metrics(searches, [c["shortlist"][:3] for c in searches]))
    print("Paraphrase targets in shortlist:", sum(bool(set(c["expected"]) & set(c["shortlist"])) for c in searches if c["kind"] == "paraphrase"))
    for arg in sys.argv[1:]:
        print(arg, json.dumps(evaluate(Path(arg), searches, frames), indent=2))
