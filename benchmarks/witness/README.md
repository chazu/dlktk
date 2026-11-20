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

## Twenty-pair extension results

GPT-4.1 mini solved **15/19 with dlktk and 14/19 unaided** on puzzles where both arms returned answers. Eleven were correct in both arms, four only with dlktk, three only unaided, and one in neither. The plain session for `0_1_8` timed out. Its assisted session returned the correct answer. We retained all 40 attempts and made no retry. [Saved extension sessions](extension.jsonl)

| Measure | Single chains | Branching graphs |
|---|---:|---:|
| Plain accuracy on available pairs | 7/10 | 7/9 |
| Assisted accuracy on the same pairs | 9/10 | 6/9 |
| Exact assisted captures, including the unpaired session | 8/10 | 5/10 |
| Mean fraction of gold attacks captured per puzzle | 82.4% | 65.8% |
| Missing / extra / repeated directed attacks | 17 / 0 / 0 | 31 / 0 / 0 |
| Inexact captures that preserved the gold verdict | 1/2 | 2/5 |
| Final answers agreeing with the returned label | 8/10 | 10/10 |

The chain result needs qualification. On `0_11` the agent captured all eleven attacks and used the correct OUT verdict. On `0_13` it captured just the direct attacker. Dropping twelve downstream attacks happened to preserve OUT. Both cases counted as assisted wins.

All three plain-only wins were branching puzzles with missing attacks in the assisted submission. On `0_4_4_6`, eleven omitted attacks changed the verdict to OUT. The agent followed every branching verdict it received. Branch capture is the first failure to investigate in this sample.

The two chain disagreements with the tool had opposite effects. On `0_5`, a correct graph yielded OUT and the agent answered yes. On `0_6`, an incomplete graph yielded OUT and the agent's yes answer matched the key. Track tool agreement and final accuracy independently.

Across all usable sessions, plain accuracy was 10/10 on yes and 4/9 on no. Assisted accuracy was 7/10 on yes and 9/10 on no. Each arm received the same 2,048-output-token allowance per puzzle. Plain sessions used 3,658 reported input tokens and 8,846 output tokens over 19 completed calls. Assisted sessions used 11,900 input tokens and 2,500 output tokens over 40 calls. Reported costs were USD 0.015617 and USD 0.008760, respectively. Elapsed session totals were 127.553 and 62.536 seconds. The plain total includes the timeout, which returned no usage or billing data.

Rescore with `python3 score.py --suite extension extension.jsonl`. Add `--suite extension` to the DeepEval command above and supply `extension.jsonl` as input. Both scorers produced identical applicable per-case scores on all saved pilot and extension sessions. This is one run of one model on a single template family. The extra model turn and one unavailable pair limit the comparison.
