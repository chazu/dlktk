"""Verify the frozen witness cases against the authors' published CSV."""
import argparse
import ast
import csv
import hashlib
import io
import json
from pathlib import Path
import re
from urllib.request import urlopen

HERE = Path(__file__).resolve().parent
REVISION = "a10dae138180584c9fb686ff11ff62d0decac6f1"
URL = (f"https://raw.githubusercontent.com/CorSteging/ParameterizedArgumentationBasedReasoningTasks/"
       f"{REVISION}/static_benchmarks/static_non_linear_5_15_shuffle_True.csv")
SHA256 = "fd8fd2586342ad0cf8f1355036155d6b26cd79b6cba5db5046a711633ff34358"
SHAPES = [(4,), (3,), (9,), (8,), (2, 2), (1, 2), (2, 5), (2, 6), (2, 4, 6), (2, 3, 6)]


def extract(prompt):
    roots, edges, witnesses = [], [], set()
    for line in prompt.splitlines():
        if not line.startswith("Witness "):
            continue
        who, statement = line.removeprefix("Witness ").split(" says that ", 1)
        if who in witnesses:
            raise ValueError(f"Duplicate testimony from {who}")
        witnesses.add(who)
        attack = re.fullmatch(r"witness (.+) is lying\.", statement)
        if attack:
            edges.append([who, attack[1]])
        else:
            roots.append(who)
    if len(roots) != 1:
        raise ValueError("Expected one main claim")
    root, = roots
    parents = dict(edges)
    paths = []
    for leaf in witnesses - set(parents.values()):
        chain = set()
        node = leaf
        while node != root:
            if node in chain or node not in parents:
                raise ValueError("Testimony does not lead to the main claim")
            chain.add(node)
            node = parents[node]
        paths.append(chain)
    if sum(map(len, paths)) != len(edges) or set.union(set(), *paths) != witnesses - {root}:
        raise ValueError("Expected disjoint chains attacking the main claim")
    return root, sorted(edges), sorted(map(len, paths))


def select_cases(data):
    if hashlib.sha256(data).hexdigest() != SHA256:
        raise ValueError("Unexpected upstream CSV digest")
    rows = list(csv.DictReader(io.StringIO(data.decode())))
    cases = []
    for shape in SHAPES:
        row = next(r for r in rows if ast.literal_eval(r["format"]) == shape)
        root, edges, lengths = extract(row["prompt"])
        answer = "yes" if row["answer"] == "1" else "no"
        if (row["answer"] not in ("0", "1") or lengths != list(shape)
                or (answer == "yes") != all(n % 2 == 0 for n in lengths)):
            raise ValueError(f"Answer key disagrees with testimony for {row['id']}")
        cases.append(dict(id=row["id"], shape=lengths, answer=answer, root=root, edges=edges, prompt=row["prompt"]))
    if len({c["prompt"] for c in cases}) != 10 or sum(c["answer"] == "yes" for c in cases) != 5:
        raise ValueError("Expected ten distinct prompts with balanced verdicts")
    return cases


def manifest(cases):
    return [{k: v for k, v in case.items() if k != "prompt"} for case in cases]


def load_cases(path=None):
    if path:
        data = Path(path).read_bytes()
    else:
        with urlopen(URL, timeout=60) as response:
            data = response.read()
    cases = select_cases(data)
    if manifest(cases) != json.loads((HERE / "cases.json").read_text()):
        raise ValueError("Cases differ from the frozen manifest")
    return cases


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--csv", help="Read a local copy instead of downloading the pinned CSV")
    args = parser.parse_args()
    for case in load_cases(args.csv):
        print(case["id"], case["answer"], case["shape"])
