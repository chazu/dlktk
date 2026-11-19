"""Collect typed choices for the objection-review pilot."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import random
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

HERE = Path(__file__).resolve().parent
OPTIONS = {
    "addresses": "The response describes how the hybrid handles every material part of this objection.",
    "persists": "The described mechanism leaves a material part of this objection in place.",
    "unclear": "The supplied information does not establish whether the hybrid handles this objection.",
}
INSTRUCTIONS = "Assess this reply to the inherited objection for this particular hybrid. Use only the supplied facts."


def query(url, headers, payload):
    body = json.dumps(payload, ensure_ascii=False).encode()
    for attempt in range(4):
        try:
            with urlopen(Request(url, body, headers), timeout=30) as response:
                result = json.load(response)
            break
        except HTTPError as error:
            if error.code not in (409, 429, 529) or attempt == 3:
                raise
            time.sleep(2 ** attempt)
    answer = result["answers"]["review"]
    scores = answer["probabilities"]
    options = payload["questions"]["review"]["criteria"]
    if answer["choice"] not in options or set(scores) != set(options):
        raise ValueError("Unexpected options in response")
    if not all(math.isfinite(p) and 0 <= p <= 1 for p in scores.values()):
        raise ValueError("Invalid probabilities")
    return dict(choice=answer["choice"], scores=scores, model=result.get("model"), usage=result.get("usage"))


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
    corpus = (HERE / "cases.json").read_bytes()
    cases = json.loads(corpus)
    with args.output.open("x") as out:
        for case in cases:
            order = list(OPTIONS)
            random.Random(case["id"]).shuffle(order)
            for run, keys in enumerate([order, order[::-1]], 1):
                question = dict(type="choice", instructions=INSTRUCTIONS, criteria={k: OPTIONS[k] for k in keys})
                payload = dict(model=model, state=case["state"], questions={"review": question})
                row = dict(id=case["id"], provider=args.provider, requested_model=model, order=keys,
                           corpus_sha256=hashlib.sha256(corpus).hexdigest(), run=run)
                start = time.monotonic()
                try:
                    row.update(query(url, headers, payload))
                except (HTTPError, URLError, TimeoutError, ValueError, KeyError) as error:
                    row["error"] = str(error)
                row["ms"] = round(1000 * (time.monotonic() - start))
                print(json.dumps(row), file=out, flush=True)
                if args.provider == "decider":
                    time.sleep(1)


if __name__ == "__main__":
    main()
