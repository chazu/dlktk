"""Deterministic witness measurements shared by the paired scorer and DeepEval."""
from collections import Counter


def index_sessions(cases, rows):
    lookup = {(r["id"], r["arm"]): r for r in rows}
    expected = {(c["id"], arm) for c in cases for arm in ("plain", "dlktk")}
    if set(lookup) != expected or len(lookup) != len(rows):
        raise ValueError("Expected exactly one session per case and arm")
    return lookup


def capture(case, submission):
    """Return distinct-edge differences even if the tool rejected repeated edges."""
    if not isinstance(submission, dict) or not isinstance(submission.get("attacks"), list):
        return None
    counts = Counter()
    for attack in submission["attacks"]:
        if not isinstance(attack, dict):
            return None
        edge = attack.get("attacker"), attack.get("target")
        if not all(isinstance(name, str) for name in edge):
            return None
        counts[edge] += 1
    gold = set(map(tuple, case["edges"]))
    missing = sorted(gold - counts.keys())
    extra = sorted(counts.keys() - gold)
    repeated = [{"edge": e, "repetitions": n-1} for e, n in sorted(counts.items()) if n > 1]
    return dict(root=submission.get("root"), missing=missing, extra=extra, repeated=repeated,
                expected=len(gold), distinct=len(counts), duplicates=sum(counts.values()) - len(counts),
                exact=submission.get("root") == case["root"] and not (missing or extra))


def edge_list(edges):
    return ", ".join(f"{a} -> {b}" for a, b in edges) or "none"


def assess(case, row):
    """Return only applicable metrics. Transport failures have no final accuracy."""
    metrics = {}
    def record(key, value, reason):
        metrics[key] = {"score": float(value), "reason": reason}

    answer = row.get("answer")
    if row.get("error", {}).get("stage") != "transport":
        record("final_accuracy", answer == case["answer"], f"final={answer}, gold={case['answer']}")
    graph = capture(case, row.get("submission")) if row["arm"] == "dlktk" else None
    if graph is not None:
        missed, extra = graph["missing"], graph["extra"]
        root_reason = f"root={graph['root']}, expected={case['root']}"
        record("root_correct", graph["root"] == case["root"], root_reason)
        record("exact_distinct_graph", graph["exact"],
               f"{root_reason}. Missing: {edge_list(missed)}. Extra: {edge_list(extra)}.")
        record("attack_recall", (graph["expected"] - len(missed)) / graph["expected"],
               f"{graph['expected'] - len(missed)}/{graph['expected']} gold attacks recovered")
        record("no_missing_edges", not missed, f"{len(missed)} missing: {edge_list(missed)}")
        record("no_extra_edges", not extra, f"{len(extra)} extra: {edge_list(extra)}")
        record("no_repeated_edges", not graph["duplicates"],
               f"{graph['duplicates']} repetitions: {edge_list(r['edge'] for r in graph['repeated'])}")
    label = (row.get("tool_result") or {}).get("grounded_label")
    if label is not None:
        if label not in ("IN", "OUT"):
            raise ValueError(f"Unexpected label on acyclic witness puzzle: {label}")
        verdict = "yes" if label == "IN" else "no"
        record("tool_verdict_correct", verdict == case["answer"], f"tool={label}, gold={case['answer']}")
        record("verdict_agreement", answer == verdict, f"tool={label}, final={answer}")
    return graph, metrics
