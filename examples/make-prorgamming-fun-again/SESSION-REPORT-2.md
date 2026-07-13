# Session report — rerun under contract 0.11.0 (arc two, item 1)

Run 2026-07-12 against dlktk contract **0.11.0** (rival-aware tested-ness),
same prompt ([`issue.md`](issue.md)), same seven-persona cast, from scratch.
Discussion `kudag-kuroj` in the local store (`.pudl/`), exported to
[`dialectic.ndjson`](dialectic.ndjson); the 0.10.0 run's export is preserved
as [`dialectic-0.10.0.ndjson`](dialectic-0.10.0.ndjson). The purpose
of the rerun: see how the hardened tested-ness semantics change the
convergence endgame that the first run exposed as the weakest phase.

## Headline

**The session closed with `check --strict` exit 0 — zero findings.** The
0.10.0 run closed over a live `untested_decision` warning; this run's every
decision (root, sub-issue, governance) survived a substantive cross-author
objection before being decided, because the tool now demands exactly that at
exactly the right moments. The convergence-integrity loop item 1 was built to
close is closed at its linchpin.

## What the deliberation produced (fresh content, not a replay)

1. **Divergence:** five rival positions from five authors (on-demand
   comprehension, partition-by-criticality, architecture-as-executable-
   contract, inverted review/prediction-quizzing, aviation hand-flown legs),
   three premises recorded as assumptions.
2. **Devil's-advocate round:** all five defeated by cross-author objections;
   all five reinstated by defenses — three of which independently converged
   on *risk-targeting* (sample by criticality, target the churn, scope to the
   core), the seed of the eventual synthesis.
3. **A different reframing cascade than last time:** the bus-factor objection
   to partition-by-criticality spawned, via `raise --from`, *"whose
   comprehension is the unit — the individual's or the team's?"* (the 0.10.0
   run's cascade went to theory-location instead). Synthesized answer:
   **paired theory-holdership** — two current holders per load-bearing
   module, rotating one seat at a time through hand-implemented handoffs.
   Decided with `--review-by`; robust across both declared audiences.
4. **Grand synthesis:** **risk-priced comprehension** — human-authored
   contract layer; paired holdership in the core; interrogation sampled by
   criticality × churn feeding spaced retrieval; full-speed generation
   everywhere else. Five parents. Stress-tested by the Shipper's
   *sum-of-costs* objection — the exact attack the 0.10.0 bundle deserved
   and never received — and defended (costs are scoped to disjoint slices,
   bounded by core size). `crux` reports that defense (`lupug-kakog`) as the
   single argument the whole outcome hinges on: if the taxes do overlap in
   practice, the decision unravels — which is what the review horizon is for.
5. **Audiences:** nothing at the root is robust — under `velocity-first` the
   cost-objection's defense is value-blocked, the hybrid flips OUT and
   do-nothing flips IN. Same central finding as 0.10.0, reached through a
   different causal path (argument-level value tags rather than
   position-level ones).
6. **Governance, resolved dialectically this time:** "whose ranking governs?"
   got two rival positions and was settled by argument alone (charter grounds
   IN after its circularity objection was rebutted; per-team relativism OUT
   under the principal-agent objection) — no preference needed, no override,
   and the decision was tested before it was made.

## What the 0.11.0 mechanics changed, observed live

| Moment | 0.10.0 behavior (from F2) | 0.11.0 behavior (this run) |
|---|---|---|
| Mid-divergence, 5 fresh rivals | `untested` agenda section useless on multi-position issues (rival edges = "attacked") | agenda `untested` **empty** (no flooding); `status` marks each position untested individually |
| Stalemate advice | offered `prefer` co-equally | "…untested (rival edges only, no substantive objection) — **object first, then prefer**"; annotation **disappears** once every rival is genuinely tested |
| Hybrid ready to decide, unexamined | no cue at all — "the tool gave no cue" | agenda names the hybrid in `untested`; `moves` puts the stress-test `object` **before** `decide` |
| Elevating an untested hybrid by preference | two subsumption prefers typed with zero pushback | every `prefer hybrid rival` suggestion annotated "hybrid is untested; object first, then prefer" — and only the hybrid, not the five tested rivals |
| Session close | `untested_decision` fired and was ignored | `check --strict` exit 0, nothing to ignore |

The mechanism held against the shapes it was designed for: the sub-issue
hybrid and the grand hybrid were both caught at the decide-adjacent moment,
both received real cross-author objections *because the tool asked*, and both
entered their decisions tested.

## Findings

- **N1 — the excusal `prefer` is suggested unannotated in the OUT branch.**
  When a position is OUT under a live objection, `moves` offers
  `prefer <position> <objection>` ("block its attack") with no untested
  annotation — but taking it is precisely the excusal move that re-arms
  untested-ness under condition 2. Item 1's ordering annotation covers the
  UNDEC-tie branch only; it should cover the reinstatement branch too.
- **N2 — F4 recurred, verbatim.** The installed skill copy
  (`~/.claude/skills/dlktk-dialectic`) is still the pre-0.10.0 playbook —
  no divergence quotas, no synthesize/reframe/assume vocabulary, no untested
  section. This run followed the repo skill manually after noticing; an
  agent that didn't notice would have run the convergent playbook against
  the 0.11.0 engine. Item #10's version handshake is the fix and is not yet
  built (the skill declares no minimum contract version to compare).
- **N3 — F1 recurred: `roster` still one row per move.** 50 rows for 7
  distinct bindings this session. Item #10.
- **N4 — F5 recurred: the select_one trap is still armed.** Five largely
  complementary process changes were seated as mutual rivals by the default
  cardinality; the endgame was once again forced through a bundle synthesis.
  Item #6 (open-issue closure mechanics, then cardinality guidance) remains
  the missing road.
- **N5 — the unimplemented synthesis discipline is visible as absence.**
  `synthesize` accepted five parents with no prompt for what the hybrid
  *drops* (item #4 — this hybrid does drop things: blanket gating, the flat
  weekly mandate, universal do-nothing; recorded only in prose). No surface
  lists parents' undefeated objections as inherited questions (item #2), and
  the five subsumption preferences drew no `self_elevated_synthesis` note
  (item #3). This run compensated by playbook discipline; the graph cannot
  yet check it.
- **N6 — `--review-by` rejects the format `check` prints.** `2026-12-15` is
  refused (wants RFC3339/Unix) while `review_due` findings render horizons
  as bare dates. Trivial, but an agent copying a date from check output back
  into a move will trip on it.
- **N7 — stalemate advice enumerates every untested id.** With five untested
  rivals the advice named all five; accurate but verbose. The agenda
  correctly names one node; the advice could similarly name the strongest.

## Verdict against issue.md's bar

Divergence held (nine positions across three issues, two syntheses, a
provenance-tracked deeper question, a computed value map), and the closing
answer is again a recombination no single persona proposed. The difference
is the endgame: in 0.10.0 the tool watched an unexamined bundle walk through
`decide`; in 0.11.0 every convergent move this session made was either
preceded by a demanded stress-test or annotated until one happened. Item 1
does what the arc-two diagnosis said it would — it is the linchpin — and the
residue it leaves (N1–N5) is, item for item, the rest of the arc-two batch.
