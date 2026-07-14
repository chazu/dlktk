# Session report — rerun under contract 0.17.1 (arc two, full batch)

Run 2026-07-13 against dlktk contract **0.17.1** — the first run with the
*entire* arc-two batch available (items 1–10), plus the 0.17.1 schema
completion that lets a value-map decision round-trip through pudl's CUE.
Same prompt ([`issue.md`](issue.md)), same seven-persona cast, from scratch.
Discussion `fivij-zatad` in the local store (`.pudl/`), exported to
[`dialectic.ndjson`](dialectic.ndjson). The two prior exports are preserved:
[`dialectic-0.10.0.ndjson`](dialectic-0.10.0.ndjson) (run 1) and
[`dialectic-0.11.0.ndjson`](dialectic-0.11.0.ndjson) (run 2, item 1 only).
Run 2's live store and report are snapshotted under `.backups/run2-0.11.0/`.

## Headline

**The false-rivalry bundle that forced both prior runs is gone, and
`check --strict` closed at exit 0 with zero findings.** Run 2 closed cleanly
too — but only because its *one* hardened mechanism (item 1, tested-ness) was
carrying the whole endgame while the synthesis discipline sat "visible as
absence" (its findings N4/N5). This run engaged the whole
convergence-integrity loop: the root question was raised `--card open`, so the
four surviving practices **compose as four standing decisions** instead of
being concatenated into one unexamined super-synthesis; the single genuine
either/or produced a two-parent synthesis that **recorded what it drops,
inherited both parents' objections, and discharged them on the record**; and
the one value-driven sub-question **closed as a map, not a winner**, with its
governance question raised from it. Nothing was decided untested; nothing was
self-elevated; no bundle walked through `decide`.

## What the deliberation produced (fresh content, not a replay)

1. **Root as an open issue (item 6 — the missing road from run 2's N4).**
   Five positions from five authors on `fivol-mipin` `[open]`: comprehension
   gate (`fivup-kupur`), human-authored executable architecture contracts
   (`fizap-rotal`), hand-flown legs (`fizim-golaf`), churn-scoped deep review
   (`fizol-sifib`), incident-tied spaced retrieval (`fizuj-zilug`). Because
   these **compose**, they were seated as an open issue and adopted as four
   independent standing decisions — no artificial contest, no bundle.
2. **The graph discriminated.** Hand-flown legs was killed by the Adopter's
   sustainability objection ("the first ritual cut under deadline pressure —
   who protects the calendar, and what ships later?") and **left OUT and
   undecided**. An open issue is not a rubber stamp for every proposal; only
   the four that survived a cross-author objection were adopted.
3. **The one genuine either/or → a disciplined synthesis (items 2,3,4,8).**
   The "contracts rot — who maintains them?" objection raised, via
   `raise --from`, a select_one sub-issue: *where does a module's load-bearing
   theory live — in one owner or the whole team?* (`fofaf-mugij`). Both
   partial answers carried a live objection (bus-factor vs doesn't-scale), so
   they were recombined into **paired theory-holdership** (`fogan-bukul`) —
   two holders per load-bearing module, rotating one seat at a time through
   hand-implemented handoffs. It **records its drops** (single-owner
   accountability; read-everything breadth), **inherited both parents'
   objections** as questions, and **discharged each with `--answers`** by a
   *different* author than the synthesis author. The Adopter stress-tested it
   with a cost model (`fogug-ropuk`); the Architect reinstated it; a *third*
   author (Maintainer) decided it. `crux` names the reinstatement
   (`fohad-zagof`, "the second seat replaces ad-hoc review, not adds to it")
   as the single load-bearing argument.
4. **The value-driven sub-question → a value-map (item 7).** *How strict
   should the gate be — blocking or advisory?* (`fojas-pipit`) split cleanly
   on values: under `craft-first` the blocking gate is IN and advisory OUT;
   under `velocity-first` the verdict inverts. Nothing robust. Closed with
   `decide --map --review-by 2027-06-01` — the audience-conditional map *is*
   the deliverable — and the governance question raised from it.
5. **Governance, resolved dialectically (item 7 + item 1).** *Whose ranking
   governs — an org-wide charter or per-team choice?* (`fokos-tifir`).
   Per-team relativism fell to the principal-agent objection; the charter
   position was itself attacked ("a charter ossifies") and reinstated ("a
   charter with a review horizon is revised, not ossified — the same
   living-constraint discipline dlktk itself uses") before being decided.

## What the full arc-two batch changed, observed live

| Run-2 residue | 0.11.0 behavior (run 2) | 0.17.1 behavior (this run) |
|---|---|---|
| N4 — select_one trap | five complementary changes seated as mutual rivals; endgame forced through a bundle synthesis | root raised `--card open`; **four composing standing decisions**, no bundle |
| N5 — synthesis discipline "visible as absence" | `synthesize` took 5 parents with no drops prompt; no inherited-questions surface; subsumption prefers drew no note | 2-parent synthesis **records drops**, **lists both inherited questions**, discharges them via `--answers`; prefers over parents draw **no** `self_elevated_synthesis` |
| Adopter persona (doc only) | no cost objection in the graph | Adopter objects the synthesis **with a cost model**, `--promotes adoptability`; reinstated, not rubber-stamped |
| Value conflict at the root | converged to a single winner despite "nothing is robust" | the value-split sub-issue **closed as a map**; governance question raised → `mapped_pending_governance` cleared |
| N3 — roster one row per move | 50 rows for 7 bindings | `roster` reports **7 deduped bindings** (item 10) |
| Every decision | item 1 alone carried tested-ness | every decided position (4 root + synthesis + charter) faced a substantive cross-author objection in the defeat relation; `single_author_convergence` clean (decider ≠ synthesis author) |

## The 0.17.1 schema fix, dogfooded

The value-map decision writes `kind:"map"` into the `dlktk/decision` fact —
a field pudl's closed CUE def rejected until 0.17.1 added it. This run's
export (`dialectic.ndjson`, 101 facts) contains that fact plus 2 `addresses`
links, 2 `synthesizes` links, recorded `drops`, and 2 audiences. It
**re-imports into a clean store and re-checks at `--strict` exit 0** — the
round-trip the schema fix was for.

## Findings

- **The residues that remain are honest deferrals, not defects.** The gate
  strictness map still points at the (now-decided) governance issue: a human
  applies the charter's ranking to resolve the map at its review horizon
  (2027-06-01). That is the value-map working as designed — the map is a
  standing question with a clock, not a dodge.
- **F4/N2 (stale installed skill) is now structurally addressed** but unproven
  here: the repo skill declares `MIN_CONTRACT = 0.17.0` and performs the
  `discover` version handshake as its mandatory first move (item 10). This run
  followed the repo skill; the handshake would now halt an agent whose engine
  predated the playbook, which is the failure both prior runs hit blind.
- **No `reframe` or `assume` this run.** Provenance cascades used
  `raise --from` (three of them: theory-location, gate-strictness,
  governance); both other moves remain available and were simply not the
  shortest honest path this time.

## Verdict against issue.md's bar

Divergence held and then some — **nine positions across four issues**, one
genuine synthesis, one value-map, three provenance-tracked `raise --from`
cascades, and a computed audience map — and the closing answer is again a
recombination no single persona proposed: a **composed portfolio** (gate +
contracts + churn-scoped review + incident-tied retrieval) resting on a
**paired-holdership** core, with gate *strictness* left as an explicitly
value-governed decision. The difference from run 2 is no longer just the
linchpin: run 2 proved item 1 stops an unexamined winner; run 3 shows the
*whole* convergence-integrity loop doing what the arc-two diagnosis promised —
compose instead of bundle (6), drop and discharge instead of concatenate
(2,3,4), price adoption (5), map instead of force a winner (7), and check that
the scrutiny actually left the author's own hands (8) — with the referee
(grounded semantics) untouched throughout.
