"""Exercise agent isolation, the dlktk adapter, and scoring without a provider."""
import argparse
import json
from pathlib import Path

from corpus import HERE, load_cases
from run import BUDGET, graph, session
from score import score


def check(binary, csv_path):
    cases = load_cases(csv_path)
    for case in cases:
        args = {"root": case["root"], "attacks": [{"attacker": a, "target": b} for a, b in reversed(case["edges"])]}
        result, nodes = graph(binary, args)
        assert result["grounded_label"] == {"yes": "IN", "no": "OUT"}[case["answer"]]
        assert len(nodes) == sum(case["shape"]) + 1

    case = cases[0]
    args = {"root": case["root"], "attacks": [{"attacker": a, "target": b} for a, b in case["edges"]]}
    def reply(payload):
        messages = payload["messages"]
        assert messages[1] == {"role": "user", "content": case["prompt"]}
        assert payload["max_tokens"] == BUDGET - (12 if len(messages) == 4 else 0)
        if "tools" in payload:
            assert len(messages) == 2
            msg = {"tool_calls": [{"id":"test", "type":"function",
                                   "function":{"name":"evaluate_testimony","arguments":json.dumps(args)}}]}
        else:
            if len(messages) == 4:
                assert json.loads(messages[-1]["content"])["grounded_label"] == "IN"
            else:
                assert len(messages) == 2
            msg = {"content": "The objections are defeated.\nAnswer: yes"}
        return {"choices":[{"message":msg,"finish_reason":"stop"}], "model":"test",
                "usage":{"prompt_tokens":50,"completion_tokens":12}}
    plain = session(case["prompt"], "plain", binary, reply)
    tool = session(case["prompt"], "dlktk", binary, reply)
    for row in (plain, tool):
        row["id"] = case["id"]
        assert row["answer"] == "yes", row
    summary = score([case], [plain,tool])
    assert summary["pairs"]["both"] == 1 and summary["cases"][0]["exact_graph"]
    try:
        score([case], [plain, plain])
    except ValueError:
        pass
    else:
        raise AssertionError("Duplicate sessions passed scoring")
    args["attacks"] = [{"attacker":"A","target":"B"},{"attacker":"B","target":"A"}]
    bad = session(case["prompt"], "dlktk", binary, reply)
    assert bad["error"]["stage"] == "tool" and bad["answer"] is None
    print("Ten oracle graphs, both session paths, budget, invalid capture, and duplicate scoring passed.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--csv")
    args = parser.parse_args()
    check(args.binary.resolve(strict=True), args.csv)
