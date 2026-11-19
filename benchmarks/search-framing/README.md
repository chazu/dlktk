# Duplicate search and issue framing

## Protocol fixed before inference

Use the seven top-level design discussions, the final programming-practices discussion, and the lock example. The lock node IDs and texts come from a fresh run of the reference harness. Omit earlier versions of the same discussion to avoid counting copies of a claim. No benchmark queries or labels enter the corpus.

For search, compare whole-query case-insensitive substring matching, a local candidate ranking, and a typed model choice from at most eight candidates plus no match. Use a deterministic term-overlap ranking weighted by inverse document frequency. Include an exact substring before other candidates. Report candidate recall separately. Return up to three nodes whose probability exceeds no match, provided the model selected a node. Compare top-three recall for paraphrases, retention of literal hits, and nonempty results for eight negative queries. Count incorrect suggestions on positive queries, too. Ship only if the trial gains at least three paraphrase hits over substring, retains both literal hits, and returns a result on at most one negative. Require an advantage over local candidates alone in recall or false matches.

For framing, give the model an issue and two or three candidate answers. It can select a specific compatible pair, say all pairs conflict, or request more information. Score any demonstrably compatible pair as correct. Reference cases include documented project decisions and constructed examples with explicit or missing constraints. Require every reference with missing constraints to abstain and every exclusive reference to avoid a false composition prompt. Require at least three correct compatible-pair selections. Count wrong prompts and unnecessary abstentions separately.

Run Jev 1.14 and Decider 2B on identical prompts. Freeze the options and labels before inspecting predictions. These are small diagnostic trials from one project. Record the outcome separately for each feature.

## Results

Both providers completed all 31 requests. The combined corpus has 102 nodes across nine discussions. The local shortlist included the expected node for nine of ten paraphrases.

| Search method | Paraphrases found in first three | Literal hits | Negative queries returning hits | Extra suggestions on positive queries |
|---|---:|---:|---:|---:|
| Ordinary substring | 0/10 | 2/2 | 0/8 | 0 |
| First three local candidates | 7/10 | 2/2 | 8/8 | 26 |
| Decider 2B | 8/10 | 1/2 | 4/8 | 9 |
| Jev 1.14 | 9/10 | 2/2 | 0/8 | 1 |
| Live Go CLI with Jev | 9/10 | 2/2 | 0/8 | 2 |

Ship opt-in Jev search. The unfiltered candidate list always proposes nodes, even for unrelated queries. Jev rejected the reversed starvation claim and found the original writer-starvation objection. Neither model recovered the patronizing-quiz paraphrase because the correct node missed the shortlist. On the second live run, Jev also suggested a position advocating live edges alongside criticisms of them. Treat returned nodes as leads for inspection. The queries were authored against this corpus and establish no general accuracy rate.

| Framing method | Correct compatible pair | Correct exclusive | Correct abstention on missing information | Incorrect definite prompts | Unnecessary abstentions |
|---|---:|---:|---:|---:|---:|
| Decider 2B | 3/4 | 2/3 | 2/4 | 3 | 1 |
| Jev 1.14 | 3/4 | 2/3 | 2/4 | 2 | 2 |

Defer the framing preview. Both models called the unspecified merge policy exclusive. Decider recommended combining policies whose contents were unknown. Jev called single ownership and two maintainers incompatible without definitions for either role. The supplied project playbook gives direct reference judgments for the practices and lock examples. Recorded graph decisions and constructed references cover the other cases. Treat these as pilot references, not a second independent human annotation set. Ordinary `raise` continues to accept the author's cardinality offline.

## Reproduce

From this directory, run `python3 score.py decider.jsonl jev.jsonl` for the frozen trial. To collect a new sample, use `python3 trial.py jev new-jev.jsonl` with `OPENROUTER_API_KEY`, or `python3 trial.py decider new-decider.jsonl` with `DECIDER_URL`. The trial sends only the state and choice criteria. The two providers received the same candidate IDs in the same order. Scores and model IDs are in the result files. The Jev trial cost USD 0.000795. Decider reported no monetary cost.

For the CLI replication, import the seven top-level snapshots, the current programming-practices snapshot and the reference harness discussion into a temporary store. Run both `search --all` and `search --all --semantic` for each query in `search.tsv`. A fresh harness run generates new lock-node IDs. Compare those hits by their original text. The saved replication used the IDs in `lock-nodes.jsonl`.
