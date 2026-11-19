"""Evaluate the two reading aids on frozen discussion snapshots."""
import argparse
import csv
import itertools
import json
import math
import os
import re
import sys
import time
from collections import Counter
from pathlib import Path
from urllib.error import URLError

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE.parent / "objection-review"))
from run import query

SEARCH = ("Select the existing node that most nearly states the same claim as the query. "
          "Respect who does what, polarity, cause and scope. Related topics and contrary claims "
          "are not duplicates. Choose none if no candidate makes the claim.")
FRAME = ("Which two candidate policies could actually be adopted together under the stated constraints? "
         "Select a compatible pair if established. Choose exclusive if every pair conflicts. "
         "Choose unclear when scope, definitions or resources needed to answer are missing.")
STOP = set("a an and are as at be by can for from if in is it of on or should that the to when with".split())


def terms(text):
    return set(re.findall(r"[a-z0-9]+", text.lower())) - STOP


def corpus():
    docs = {n["id"]: n["text"] for n in map(json.loads, (HERE / "lock-nodes.jsonl").read_text().splitlines())}
    examples = ROOT / "examples"
    for path in [*sorted(examples.glob("*.ndjson")), examples / "make-prorgamming-fun-again/dialectic.ndjson"]:
        for row in map(json.loads, path.read_text().splitlines()):
            if row["relation"] == "dlktk/node":
                docs[row["args"]["id"]] = row["args"]["text"]
    return docs


def search_cases():
    docs = corpus()
    words = {key: terms(text) for key, text in docs.items()}
    df = Counter(term for tokens in words.values() for term in tokens)
    with (HERE / "search.tsv").open() as source:
        for index, row in enumerate(csv.DictReader(source, delimiter="\t"), 1):
            q = row["query"]
            literal = sorted(key for key, text in docs.items() if q.lower() in text.lower())
            tokens = terms(q)
            def rank(key):
                overlap = tokens & words[key]
                weight = sum(math.log((len(docs) + 1) / df[t]) for t in sorted(overlap))
                return (key not in literal, -weight, key)
            shortlist = sorted(docs, key=rank)[:8]
            criteria = {key: docs[key] for key in shortlist}
            criteria["none"] = "No candidate makes the same claim."
            expected = [] if row["expected"] == "-" else row["expected"].split(",")
            assert set(expected) <= docs.keys()
            yield dict(task="search", id=f"s{index:02}", kind=row["kind"], expected=expected,
                       literal=literal[:3], shortlist=shortlist, state=q, criteria=criteria, instructions=SEARCH)


def framing_cases():
    for case in json.loads((HERE / "framing.json").read_text()):
        opts = dict(zip("ABC", case["options"]))
        choices = {a+b: f"Adopt {a} and {b} together." for a, b in itertools.combinations(opts, 2)}
        choices.update(exclusive="All pairs conflict.", unclear="Insufficient information to determine compatibility.")
        yield dict(task="framing", id=case["id"], expected=case["expect"],
                   state=dict(issue=case["issue"], proposals=opts), criteria=choices, instructions=FRAME)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("provider", choices=["decider", "jev"])
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    headers = {"Content-Type": "application/json"}
    if args.provider == "jev":
        url = "https://openrouter.ai/api/alpha/decisions"
        headers["Authorization"] = "Bearer " + os.environ["OPENROUTER_API_KEY"]
        model = "typesafe/jev-1.14"
    else:
        url = os.environ["DECIDER_URL"].rstrip("/") + "/v1/systemone"
        model = "decider-2b-v11-Q4_K_M"
    with args.output.open("x") as out:
        for case in itertools.chain(search_cases(), framing_cases()):
            question = dict(type="choice", criteria=case["criteria"], instructions=case["instructions"])
            payload = dict(model=model, state=case["state"], questions={"review": question})
            row = dict(task=case["task"], id=case["id"], requested_model=model)
            start = time.monotonic()
            try:
                row.update(query(url, headers, payload))
            except (URLError, TimeoutError, ValueError, KeyError) as error:
                row["error"] = str(error)
            row["ms"] = round(1000 * (time.monotonic() - start))
            print(json.dumps(row), file=out, flush=True)
            if args.provider == "decider":
                time.sleep(1)


if __name__ == "__main__":
    main()
