# Objection review pilot

Each candidate reply claims to answer an inherited objection on a specific hybrid. Predict whether its text addresses the full objection. An explicit admission that the problem still exists belongs in `persists`. A vague reply with too little evidence belongs in `unclear`.

## Corpus and review

The source discussion is `fivij-zatad` in `examples/make-prorgamming-fun-again/dialectic.ndjson`. Each case gives the original IDs for verbatim fields. Other fields are constructed for this pilot. Two recorded answers come from the restored hybrid's own address links. The constructed cases simulate submitting the reply with that link. The ownership cases include alternative hybrids answering the same parent objection.

The four ownership cases are development cases. The sixteen held-out cases cover gate throughput, contract drift, review volume, and practice time. All variants of each claim family stay in one split. All cases share a discussion domain. A future adoption trial needs other discussions.

Review the parent, objection, hybrid, and reply in each case. Record changes by case ID before interpreting model scores. The initial labels are author proposals. The runner sends only the four text fields. The maintainer reviewed all 20 cases. See `reviewed.json` for labels and `review-notes.json` for case-specific judgments.

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
python3 score.py --labels reviewed.json decider.jsonl jev.jsonl
```

## November 19 reviewed results

Both models returned all 40 predictions each. Rescoring used the maintainer's labels and the original predictions. The primary pass has 16 held-out cases. The reviewer judged 12 replies inadequate or unclear.

| Measure | Answer-link baseline | Decider 2B | Jev 1.14 |
|---|---:|---:|---:|
| Correct three-way assessment | 4/16 | 7/16 | 10/16 |
| Inadequate or unclear replies marked addressed | 12/12 | 2/12 | 1/12 |
| Inadequate or unclear replies among first ten inspected | 7/10 | 9/10 | 9/10 |
| Choices changed after reversing option order | n/a | 4/16 | 1/16 |
| Median request time across both passes | n/a | 177 ms | 218 ms |

The structural baseline treats every answer link as reassurance and inspects in seeded corpus order. Uniform random inspection would find 7.5 weak replies per ten on average. Jev reported USD 0.000789 for its 40 requests. Decider reported token counts without a monetary cost.

Defer integration of the objection reviewer. Both providers missed the predeclared requirement of zero false reassurances. Decider also exceeded the limit of one order flip. Both improved the inspection order in this sample. All held-out replies were constructed from a single discussion.

The human review changed six provisional labels. On practice-1, both models accepted a protected half-day with a specific scheduling tradeoff. The reviewer judged that commitment infeasible in a real software shop. On contract-4, the reviewer accepted the required owner check while questioning its cost. These cases show why one label can be hard to assign. A future evaluation could ask separately whether a reply answers the stated objection and what evidence supports its feasibility. Keep the current labels as reviewed.

## Review in a browser

Run `python3 review.py` from this directory. Open `http://127.0.0.1:8741/`, or use `--port` to select another local port. The page resumes at the first unanswered case. Choose a judgment, add an optional reason, and select **Save and continue**. The server writes to `reviewed.json` and `review-notes.json`. Select a case in the sidebar to revise a saved judgment. The page shows the proposal, objection, hybrid, and reply. Model predictions and provisional labels stay hidden. With all cases reviewed, rerun the scoring command above to evaluate the saved predictions against your edits.
