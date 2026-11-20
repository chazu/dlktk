# Witness reasoning pilot

The pilot uses ten shuffled puzzles from Steging, Renooij, and Verheij. The manifest contains the published row IDs and answer keys, plus the attack edges extracted from each puzzle. Keep this file outside agent sessions. [Design](DESIGN.md) · [Published dataset](https://github.com/CorSteging/ParameterizedArgumentationBasedReasoningTasks/tree/a10dae138180584c9fb686ff11ff62d0decac6f1/static_benchmarks)

From this directory, run `python3 corpus.py` to download and verify the pinned CSV. To work offline, pass `--csv PATH` with a copy of `static_non_linear_5_15_shuffle_True.csv`. Both paths verify the same SHA-256 digest. The check reconstructs each chain from the shuffled prose and checks its length and the supplied verdict.

## Run

Build dlktk from the checkout with `GOWORK=off go build -o dlktk ./cmd/dlktk`. From this directory, exercise the adapter with `python3 check.py --binary ../../dlktk`. This runs all ten reference graphs through the CLI and tests the two agent paths with simulated replies.

For live inference, set `OPENROUTER_API_KEY` and run `python3 run.py --binary ../../dlktk new-run.jsonl`. Use a new output filename. Each line records one session, its actual model, usage, final answer, and any error. Assisted sessions also record the submitted edges, generated node IDs, and the label returned to the agent. The adapter receives only the model's submission. It does not infer missing edges or correct witness names.

Run `python3 score.py new-run.jsonl` for the paired outcomes and exact-graph checks. Scoring uses the local frozen manifest and makes no network requests.
