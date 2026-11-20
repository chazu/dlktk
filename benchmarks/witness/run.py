"""Run the two fresh-session arms of the witness pilot."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

from corpus import SUITES, load_cases

MODEL = "openai/gpt-4.1-mini"
BUDGET = 2048
SYSTEM = ("Solve the witness puzzle using only the supplied testimony. Give one sentence "
          "identifying the decisive testimony. Finish on a separate line with Answer: yes or Answer: no.")
TOOL = {"type": "function", "function": {
    "name": "evaluate_testimony",
    "description": "Use dlktk to evaluate your reading of the testimony. Submit the main witness and the directed attacks you found.",
    "parameters": {"type": "object", "required": ["root", "attacks"], "additionalProperties": False,
                   "properties": {"root": {"type": "string", "description": "Name of the witness making the main claim"},
                                  "attacks": {"type": "array", "items": {
                                      "type": "object", "required": ["attacker", "target"], "additionalProperties": False,
                                      "properties": {"attacker": {"type": "string", "description": "Name of the witness making the accusation"},
                                                     "target": {"type": "string", "description": "Name of the witness accused"}}}}}}
}}


def graph(binary, arguments):
    root, attacks = arguments["root"], arguments["attacks"]
    def valid(name):
        return isinstance(name, str) and bool(name.strip()) and len(name) <= 100
    if not valid(root) or not isinstance(attacks, list) or len(attacks) > 50:
        raise ValueError("Invalid witness graph")
    edges = [(a["attacker"], a["target"]) for a in attacks]
    if any(not valid(name) for edge in edges for name in edge):
        raise ValueError("Invalid witness name")
    if root in dict(edges) or len(dict(edges)) != len(edges):
        raise ValueError("Each accusing witness must occur once and the main witness cannot accuse")
    with tempfile.TemporaryDirectory() as work:
        def move(*args):
            cmd = [binary, "--format", "json", "--store", str(Path(work) / "store"), *args]
            proc = subprocess.run(cmd, cwd=work, capture_output=True, text=True, timeout=20)
            if proc.returncode:
                raise RuntimeError(proc.stderr.strip())
            return json.loads(proc.stdout)
        disc = move("new", "Witness testimony")["id"]
        issue = move("-d", disc, "raise", "Should the main witness be believed?", "--card", "open")["id"]
        pos = move("-d", disc, "propose", issue, f"{root} asserts the main claim")["id"]
        nodes = {root: pos}
        pending = edges
        while pending:
            rest = []
            for attacker, target in pending:
                if target in nodes:
                    nodes[attacker] = move("-d", disc, "object", nodes[target], f"{attacker} accuses {target}")["id"]
                else:
                    rest.append((attacker, target))
            if len(rest) == len(pending):
                raise ValueError("Attack cycle or disconnected witness")
            pending = rest
        label = move("-d", disc, "show", pos)["label"]
        return {"root": root, "grounded_label": label}, nodes


def chat(payload):
    request = Request("https://openrouter.ai/api/v1/chat/completions", json.dumps(payload).encode(),
                      {"Content-Type": "application/json", "Authorization": "Bearer " + os.environ["OPENROUTER_API_KEY"]})
    with urlopen(request, timeout=90) as response:
        return json.load(response)


def session(prompt, arm, binary, send=chat):
    messages = [{"role": "system", "content": SYSTEM}, {"role": "user", "content": prompt}]
    row = {"arm": arm, "calls": [], "answer": None}
    remaining = BUDGET
    def call(use_tool=False):
        nonlocal remaining
        if remaining <= 0:
            raise ValueError("Output budget exhausted")
        payload = dict(model=MODEL, messages=messages, temperature=0, max_tokens=remaining)
        if use_tool:
            payload.update(tools=[TOOL], tool_choice={"type": "function", "function": {"name": "evaluate_testimony"}})
        result = send(payload)
        choice, usage = result["choices"][0], result["usage"]
        row["calls"].append(dict(model=result["model"], provider=result.get("provider"),
                                 finish=choice["finish_reason"], usage=usage))
        remaining -= usage["completion_tokens"]
        if remaining < 0:
            raise ValueError("Provider exceeded output budget")
        return choice["message"]
    start = time.monotonic()
    stage = "provider"
    try:
        if arm == "dlktk":
            message = call(True)
            stage = "tool"
            tc, = message["tool_calls"]
            if tc["function"]["name"] != "evaluate_testimony":
                raise ValueError("Unexpected tool name")
            args = json.loads(tc["function"]["arguments"])
            row["submission"] = args
            result, row["nodes"] = graph(binary, args)
            row["tool_result"] = result
            messages.extend([{"role": "assistant", "content": message.get("content"), "tool_calls": [tc]},
                             {"role": "tool", "tool_call_id": tc["id"], "content": json.dumps(result)}])
        stage = "provider"
        final = call()["content"]
        row["response"] = final
        stage = "final"
        m = re.search(r"(?:^|\n)Answer:\s*(yes|no)\s*[.!]?\s*\Z", final or "", re.I)
        if not m:
            raise ValueError("Missing final Answer line")
        row["answer"] = m[1].lower()
    except (HTTPError, URLError, TimeoutError) as err:
        row["error"] = {"stage": "transport", "detail": str(err)}
    except (KeyError, ValueError, TypeError, RuntimeError, subprocess.TimeoutExpired) as err:
        row["error"] = {"stage": stage, "detail": str(err)}
    row["ms"] = round(1000 * (time.monotonic() - start))
    return row


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--csv")
    parser.add_argument("--suite", choices=SUITES, default="pilot")
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    if not os.environ.get("OPENROUTER_API_KEY"):
        parser.error("Set OPENROUTER_API_KEY")
    binary = args.binary.resolve(strict=True)
    version = json.loads(subprocess.check_output([binary,"discover","--format","json"]))["version"]
    cases = load_cases(args.csv, args.suite)
    with args.output.open("x") as out:
        for i, case in enumerate(cases):
            arms = ("plain", "dlktk") if i % 2 == 0 else ("dlktk", "plain")
            for arm in arms:
                row = session(case["prompt"], arm, binary)
                row.update(id=case["id"], requested_model=MODEL, contract=version)
                print(json.dumps(row), file=out, flush=True)
                print(case["id"], arm, row.get("error", {}).get("stage", "complete"), flush=True)


if __name__ == "__main__":
    main()
