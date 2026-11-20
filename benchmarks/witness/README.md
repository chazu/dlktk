# Witness reasoning pilot

The pilot uses ten shuffled puzzles from Steging, Renooij, and Verheij. The manifest contains the published row IDs and answer keys, plus the attack edges extracted from each puzzle. Keep this file outside agent sessions. [Design](DESIGN.md) · [Published dataset](https://github.com/CorSteging/ParameterizedArgumentationBasedReasoningTasks/tree/a10dae138180584c9fb686ff11ff62d0decac6f1/static_benchmarks)

From this directory, run `python3 corpus.py` to download and verify the pinned CSV. To work offline, pass `--csv PATH` with a copy of `static_non_linear_5_15_shuffle_True.csv`. Both paths verify the same SHA-256 digest. The check reconstructs each chain from the shuffled prose and checks its length and the supplied verdict.

## Run

Build dlktk from the checkout with `GOWORK=off go build -o dlktk ./cmd/dlktk`. From this directory, exercise the adapter with `python3 check.py --binary ../../dlktk`. This runs all ten reference graphs through the CLI and tests the two agent paths with simulated replies.

For live inference, set `OPENROUTER_API_KEY` and run `python3 run.py --binary ../../dlktk new-run.jsonl`. Use a new output filename. Each line records one session, its actual model, usage, final answer, and any error. Assisted sessions also record the submitted edges, generated node IDs, and the label returned to the agent. The adapter receives only the model's submission. It does not infer missing edges or correct witness names.

Run `python3 score.py new-run.jsonl` for the paired outcomes and exact-graph checks. Scoring uses the local frozen manifest and makes no network requests.

## First paired run

The same GPT-4.1 mini model answered **7/10 with dlktk and 5/10 unaided**. Three puzzles were correct in both arms. Four were correct only with dlktk, two only unaided, and one in neither arm. There were no transport failures. One rejected tool submission counts against the assisted arm. [Saved sessions](pilot.jsonl)

| Measure | Plain | With dlktk |
|---|---:|---:|
| Correct on the five yes cases | 3 | 4 |
| Correct on the five no cases | 2 | 3 |
| Model calls | 10 | 19 |
| Input tokens | 1,756 | 5,163 |
| Output tokens | 3,303 | 1,162 |
| Provider-reported cost (USD) | 0.0059872 | 0.0039244 |
| Total session time (seconds) | 36.073 | 30.833 |

Both arms used OpenRouter with provider OpenAI and returned model ID `openai/gpt-4.1-mini`. The local binary reported contract 0.20.0. The assisted agent submitted seven exact graphs, two incomplete graphs, and one graph with repeated attacks. The adapter rejected the last submission before a verdict. The model followed the returned label in eight of nine final answers.

### Where the arms differed

| Puzzle | Correct arm | Evidence in the saved session |
|---|---|---|
| `0_9` | dlktk | The plain response called a single chain a cycle and accepted the main claim. The assisted graph preserved all nine attacks and returned OUT. |
| `0_8` | dlktk | The plain response inferred a cycle among five witnesses. The assisted graph was exact and returned IN. Its final prose still incorrectly described a mutual accusation. |
| `0_2_5` | dlktk | The assisted graph retained only the two direct attackers. Omitting five attacks happened to preserve OUT. The plain answer credited the defense against one attacker. |
| `0_2_6` | dlktk | The plain response described mutual accusations absent from the testimony. The assisted graph captured both chains and returned IN. |
| `0_2_4_6` | plain | The assisted agent submitted only the three direct attackers. Omitting all nine downstream attacks changed IN to OUT. |
| `0_2_3_6` | plain | The assisted submission contained every required attack, but listed two attacks twice. The adapter rejected the repeated attackers under the frozen protocol. |

On `0_1_2`, both arms answered yes. The assisted agent recorded the exact graph. dlktk returned OUT because one direct objection survived. The final response discussed the defeated objection and contradicted the tool result.

These ten public puzzles show a concrete benefit on long chains and two distinct failure points: capture and final reporting. One model, one attempt per arm, a forced graph call, and one strict duplicate policy limit the conclusion. A correct verdict alone can conceal an incomplete graph. Keep this pilot as a diagnostic. If extending it, use fresh cases and continue scoring both capture and verdict. To reproduce this report offline, run `python3 score.py pilot.jsonl`.

## Paired diagnostics and optional DeepEval report

`score.py` reports paired outcomes, per-puzzle edge differences, and totals for chains and branching graphs. In `diagnostics.metrics`, each entry has a sum and the number of applicable cases. Divide the attack-recall sum by its count for mean per-puzzle recall. A reversed edge counts as one missing and one extra attack. Duplicate edges affect the repetition count and the runner's acceptance check. Graph comparison uses the root and the set of distinct directed attacks. The older `exact_graph` field applies only to accepted graphs.

The optional adapter exposes the same measurements through DeepEval 4.3.4. From this directory:

```sh
python3 score.py pilot.jsonl
uv run --python 3.12 --script deepeval_report.py pilot.jsonl --output /tmp/witness-pilot-metrics.json
```

The output path must be new. Each entry identifies the puzzle and arm and contains scores and diagnostic reasons. Metrics without an applicable observation are omitted. A rejected submission can still have a measurable capture. Only sessions with a tool label contribute to verdict agreement. An invalid final answer counts as incorrect. Transport failures remain separate. The adapter disables telemetry and hosted reporting, and uses saved verdicts and graphs. The first package installation requires network access. The paired scorer has no DeepEval dependency.

For the twenty fresh pairs, use `--suite extension` on the corpus verifier, runner, scorer, or adapter. See the [extension protocol](EXTENSION.md).
