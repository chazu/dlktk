# Session report — wicked-problems features exercise

Run 2026-07-11 against dlktk contract **0.10.0**, using the prompt in
[`issue.md`](issue.md). Discussion `sulog-galak` in the local store
(`.pudl/`), exported to [`dialectic.ndjson`](dialectic.ndjson). Seven
authors under seven personas (Maintainer, Shipper, Advocate-Mastery,
Advocate-Learning, First-Principles, Analogist, Reframer), following the
diverge-then-converge playbook in `skills/dlktk-dialectic`.

## What the deliberation produced

The bar in issue.md was "multiple non-trivial insights, thorough exploration,
divergent approaches." That happened. The arc:

1. **Divergence:** six rival positions from six authors (comprehension gates,
   inverted division of labor, pedagogical loop, Naur theory ownership,
   driver/navigator rotation, do-nothing + on-demand comprehension), each
   tagged with the value it promotes; four load-bearing premises recorded as
   assumptions.
2. **Adjudication:** a devil's-advocate round defeated four positions; two
   were reinstated by objections-to-objections (the prediction-gate and
   situated-practice defenses — which `crux` later confirmed as the two
   pivotal arguments).
3. **The reframing cascade:** the ivory-tower objection to theory ownership
   ("theory is built BY implementing") spawned, via `raise --from`, the deeper
   question *"where must a program's theory live so it survives agent-scale
   churn?"* — the single most generative move of the session, and it is
   visible as provenance in `tree`.
4. **Synthesis on the sub-issue:** all three theory-locations (head,
   executable spec, design record) were individually defeated by
   complementary objections; `synthesize` produced the surviving answer —
   **an executable specification bound to its decision record** ("the WHAT
   stays enforced, the WHY stays attached"), stress-tested and decided with a
   review horizon.
5. **Grand synthesis on the root:** a four-parent hybrid — the
   **theory-anchored agentic loop** (executable conceptual core + falsifiable
   prediction gates + situated learning artifacts + scheduled katas on
   load-bearing modules only; generation unrestricted elsewhere) — elevated
   over the two surviving rivals by subsumption preferences and decided with
   `--review-by 2026-12-15`.
6. **The value finding:** `audiences` showed **no position is robust across
   value rankings** — under a velocity-first audience the do-nothing position
   flips to IN and everything else (including both decisions) goes OUT. Per
   the playbook, that conflict was named as its own issue and decided on
   scope grounds: the problem statement presupposes mastery/longevity matter,
   so velocity constrains the *how*, not the *whether*.

Non-trivial insights, as demanded: the theory-location reframing; the
spec-bound-to-rationale synthesis; falsifiable-prediction gates as the
anti-rubber-stamp form of review; the audience-sensitivity result (the value
conflict *is* the real decision); and the confinement of hand-implementation
to the load-bearing core rather than everywhere.

## Feature-by-feature verdict

| # | Feature | Verdict |
|---|---------|---------|
| 1 | untested surfacing | **Works, with a gap** (see findings) — fired for the lone sub-issue position; `check --strict` caught the untested decision, exit 5 |
| 2 | divergent stalemate advice | **Works** — the 6-way stalemate advice offered synthesis and reframe before preference; it steered this session to synthesize twice instead of prefer-first |
| 3 | reframe / raise --from | **Works** — `raise --from` drove the session's best move; `reframe` verified separately (lineage `↻ reframed →`, agenda exclusion, cardinality change) |
| 4 | whatif / crux | **Works** — `whatif --object` previewed defending theory-ownership without polluting the record; `crux` correctly identified the two reinstatement arguments as pivotal |
| 5 | worlds | **Works** — the 2-way contest yielded exactly two coherent worlds with correct distinguishing nodes; robust/contingent/hopeless partition matched intuition |
| 6 | values / audiences | **Works** — produced the session's most important finding; `status --under` consistent with `audiences` |
| 7 | synthesize | **Works** — lineage recorded and rendered (`⊕ from …`); hybrid joins the rivalry as documented; accepted 4 parents including OUT ones |
| 8 | assume | **Works** — agenda lists undischarged assumptions; an attacked assumption leaves the list; gave First-Principles a genuine worklist |
| 9 | playbook | **Works** — the quotas and persona roles in the repo skill are what actually caused the divergence (see finding F4) |
| 10 | --review-by | **Works** — accepted on both decisions; not yet due, so no `review_due` finding (correct) |

## Findings (friction / bugs / gaps)

- **F1 — `roster` does not deduplicate bindings.** It emits one row per move
  (36 rows for 7 distinct author↔role pairs). The audit is unreadable at any
  real deliberation size. Likely a missing DISTINCT/dedup in the roster read.
- **F2 — "untested" is blind to select_one rivalry.** The grand hybrid
  `votov-mamug` was decided having never received a single substantive
  objection — yet it never appeared in the `untested` agenda section and
  `check --strict` raised no `untested_decision`, because its select_one
  rivals count as attackers. On a multi-position issue the epistemic intent
  of improvement #1 ("IN by silence is unexamined") is defeated: every
  position is trivially "attacked" by its rivals. Suggestion: exclude
  rival-edges when computing tested-ness, so only genuine objections count as
  a stress test. (The sub-issue hybrid got attacked only because the playbook
  told the personas to rotate a devil's advocate — the tool gave no cue.)
- **F3 — `show` on a reframed-to issue reports `links: null`.** The
  `reframes` lineage renders in `tree` (on the old issue) but is invisible
  from the new issue's `show`. Postmortem tooling reading `show` misses the
  framing history.
- **F4 — the installed skill was stale.** `~/.claude/skills/dlktk-dialectic`
  predates the wicked-problems playbook; the repo's `skills/dlktk-dialectic`
  is the one with divergence quotas, the three-exit stalemate rule, and the
  generative personas. An agent loading the installed copy would have run a
  purely convergent session — exactly the failure mode issue.md worries
  about. The features are fine; the *deployment* of the playbook is the risk.
  Re-sync the installed skill (or install from the repo as part of `make`).
- **F5 — select_one is a trap for wicked problems.** The default cardinality
  cast six largely *complementary* process changes as mutual rivals; the
  session recovered via synthesis, but `--card open` existed for exactly this
  and nothing in the skill suggests when to choose it. One sentence in the
  playbook ("if candidate answers could compose, raise with `--card open` —
  or expect the endgame to be a synthesis") would prevent false rivalry from
  the start.

## Answer to issue.md's question

The system did what the wicked-problems batch promises: the deliberation was
genuinely divergent (7 positions across 3 issues, 2 syntheses, 1 provenance-
tracked deeper question, a computed value-conflict map), and the closing
answer is a recombination no single persona proposed. The features that did
the steering were the stalemate advice (#2), `raise --from` (#3), `synthesize`
(#7), and `audiences` (#6). The two things that would most improve the next
run are F2 (rival-blind untested detection — the one place the tool still
lets an unexamined winner through) and F4/F5 (playbook deployment and
cardinality guidance — clarity-to-agents issues, exactly the kind issue.md
asked to surface).
