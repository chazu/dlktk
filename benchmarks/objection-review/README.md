# Objection review pilot

Each candidate reply claims to answer an inherited objection on a specific hybrid. Predict whether its text addresses the full objection. An explicit admission that the problem still exists belongs in `persists`. A vague reply with too little evidence belongs in `unclear`.

## Corpus and review

The source discussion is `fivij-zatad` in `examples/make-prorgamming-fun-again/dialectic.ndjson`. Each case gives the original IDs for verbatim fields. Other fields are constructed for this pilot. Two recorded answers come from the restored hybrid's own address links. The constructed cases simulate submitting the reply with that link. The ownership cases include alternative hybrids answering the same parent objection.

The four ownership cases are development cases. The sixteen held-out cases cover gate throughput, contract drift, review volume, and practice time. All variants of each claim family stay in one split. All cases share a discussion domain. A future adoption trial needs other discussions.

Review the parent, objection, hybrid, and reply in each case. Record changes by case ID before interpreting model scores. The initial labels are author proposals. The runner sends only the four text fields. Human review is pending.

## Protocol fixed before inference

1. Send the same state and three options to both providers. Use a reproducible shuffle per case and reverse it on a second pass.
2. Use pass one for primary metrics. Report changed choices on pass two and retain failures. Use the declared winning choice for classification.
3. Count false reassurance when the model selects `addresses` on a case labeled `persists` or `unclear`. Report a count and denominator. Rank cases for review by ascending probability of `addresses`. Break ties by corpus order.
4. Compare with the structural baseline. Every submitted answer links to its objection, so the graph treats all as answered. With no semantic ranking, the baseline inspects the first ten in the seeded corpus order.
5. Consider a larger trial only with full coverage, zero false reassurances, more weak replies in the first ten than baseline, and at most one order flip on the held-out cases. A passing pilot qualifies further evaluation only. Defer adoption pending human labels and additional claim families.

## Run

Use Python 3.10 or later. Set `DECIDER_URL` to the System One service base URL and supply `OPENROUTER_API_KEY` for Jev. Run from this directory. Each provider writes one JSON line per attempt at a case and option order. Use a new output path per run.

```sh
python3 run.py decider decider.jsonl
python3 run.py jev jev.jsonl
python3 score.py decider.jsonl jev.jsonl
```

## November 19 pilot results

Both providers answered all 40 requests. These results use the provisional labels. The primary pass covers 16 held-out replies, including 11 labeled inadequate or unclear.

| Measure | Answer-link baseline | Decider 2B | Jev 1.14 |
|---|---:|---:|---:|
| Correct three-way assessment | 5/16 | 11/16 | 12/16 |
| Weak replies marked addressed | 11/11 | 1/11 | 0/11 |
| Weak replies among first ten inspected | 5/10 | 8/10 | 9/10 |
| Choices changed after reversing option order | n/a | 4/16 | 1/16 |
| Median request time across both passes | n/a | 177 ms | 218 ms |

The baseline comparison interprets every answer link as reassurance. Its ranking depends entirely on corpus order. Eleven of sixteen replies are weak under the draft labels, so random inspection would find 6.875 weak replies per ten on average.

Decider marked the participation leaderboard in practice-4 as an answer even though managers could still cancel every session. Its choice changed when the options reversed. Jev's only order flip was practice-3, between a persisting problem and insufficient information. Both models rejected volume-2. A human reviewer should consider whether capping the review queue solves the objection or moves the backlog upstream.

Jev met the numerical criteria for a larger trial on the draft labels. Decider missed the false-reassurance and order-stability criteria. Jev reported a total cost of USD 0.000789 for 40 requests. Decider returned token counts without a monetary cost.

Defer integration. All held-out replies are constructed, all cases share one discussion, and a human still needs to review the labels. Preserve these predictions for rescoring. To provide independent labels, examine the state fields before reading the predictions. Supply a JSON object mapping each case ID to a label with `score.py --labels reviewed.json decider.jsonl jev.jsonl`.
