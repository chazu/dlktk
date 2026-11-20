"""Verify the frozen witness cases against the authors' published CSV."""
import argparse
import ast
import csv
import hashlib
import io
import json
from pathlib import Path
import re
import random
from urllib.request import urlopen

HERE = Path(__file__).resolve().parent
REVISION = "a10dae138180584c9fb686ff11ff62d0decac6f1"
URL = (f"https://raw.githubusercontent.com/CorSteging/ParameterizedArgumentationBasedReasoningTasks/"
       f"{REVISION}/static_benchmarks/static_non_linear_5_15_shuffle_True.csv")
SHA256 = "fd8fd2586342ad0cf8f1355036155d6b26cd79b6cba5db5046a711633ff34358"
SHAPES = [(4,), (3,), (9,), (8,), (2, 2), (1, 2), (2, 5), (2, 6), (2, 4, 6), (2, 3, 6)]

EXTENSION_SHAPES = [
    (2,), (1,), (1, 4), (2, 4), (5,), (6,), (4, 4), (3, 4),
    (10,), (7,), (1, 8), (2, 8), (11,), (12,), (2, 2, 6), (2, 2, 5),
    (14,), (13,), (3, 4, 6), (4, 4, 6),
]
SUITES = {"pilot": "cases.json", "extension": "extension-cases.json"}


def variant(row):
    lines = row["prompt"].splitlines()
    testimony = lines[1:-2]
    names = sorted(line.removeprefix("Witness ").split(" says that ")[0] for line in testimony)
    rng = random.Random("dlktk-witness-extension-v1/" + row["id"])
    shuffled = names.copy()
    rng.shuffle(shuffled)
    renaming = dict(zip(names, shuffled))
    # Substitute only speaker and accused names. Preserve the main claim verbatim.
    def rename(line):
        return re.sub(r"(Witness |witness )([A-Za-z]+)(?= says that | is lying\.)",
                      lambda m: m[1] + renaming[m[2]], line)
    rng.shuffle(testimony)
    return "\n".join([lines[0], *map(rename, testimony), *lines[-2:]])


def read_manifest(suite):
    return json.loads((HERE / SUITES[suite]).read_text())


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


def select_cases(data, suite="pilot"):
    if hashlib.sha256(data).hexdigest() != SHA256:
        raise ValueError("Unexpected upstream CSV digest")
    rows = list(csv.DictReader(io.StringIO(data.decode())))
    cases = []
    shapes = SHAPES if suite == "pilot" else EXTENSION_SHAPES
    for shape in shapes:
        row = next(r for r in rows if ast.literal_eval(r["format"]) == shape)
        prompt = row["prompt"] if suite == "pilot" else variant(row)
        root, edges, lengths = extract(prompt)
        answer = "yes" if row["answer"] == "1" else "no"
        if (row["answer"] not in ("0", "1") or lengths != list(shape)
                or (answer == "yes") != all(n % 2 == 0 for n in lengths)):
            raise ValueError(f"Answer key disagrees with testimony for {row['id']}")
        cases.append(dict(id=row["id"], shape=lengths, answer=answer, root=root, edges=edges, prompt=prompt))
    if len({c["prompt"] for c in cases}) != len(shapes) or sum(c["answer"] == "yes" for c in cases) != len(shapes) // 2:
        raise ValueError("Expected distinct prompts with balanced verdicts")
    if suite == "extension":
        if set(shapes) & set(SHAPES):
            raise ValueError("Extension repeats a pilot shape")
        for branching in (True, False):
            group = [c for c in cases if (len(c["shape"]) > 1) == branching]
            if len(group) != 10 or sum(c["answer"] == "yes" for c in group) != 5:
                raise ValueError("Expected five yes and five no cases per shape family")
    return cases


def manifest(cases):
    return [{k: v for k, v in case.items() if k != "prompt"} for case in cases]


def load_cases(path=None, suite="pilot"):
    if path:
        data = Path(path).read_bytes()
    else:
        with urlopen(URL, timeout=60) as response:
            data = response.read()
    cases = select_cases(data, suite)
    if manifest(cases) != read_manifest(suite):
        raise ValueError("Cases differ from the frozen manifest")
    return cases


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--csv", help="Read a local copy instead of downloading the pinned CSV")
    parser.add_argument("--suite", choices=SUITES, default="pilot")
    args = parser.parse_args()
    for case in load_cases(args.csv, args.suite):
        print(case["id"], case["answer"], case["shape"])
