# Implementation plan — the wicked-problems improvements

*Concrete plan for implementing the ten improvements in
[`wicked-problems.md`](wicked-problems.md). Written against the code as it
stands on this branch (contract 0.9.0), then revised after an adversarial
review pass (see "Review findings incorporated" at the end). Each item lists
the touched packages, data-model changes, surfaces, and tests.*

## Ground rules (apply to every item)

- **Layering holds.** Only `internal/store` touches pudl; `internal/af` stays
  pure; `internal/proto` is the only mutator above store; legality lives in
  `internal/ibis`; envelopes in `internal/render`; contract in
  `internal/discover`; CLI in `cmd/dlktk`; MCP mirror in `internal/mcpserv`.
- **Grounded semantics stay the referee.** Nothing below changes how
  `status`/`decide`/`check` compute labels (the one deliberate exception:
  `status --under <audience>`, which is explicitly a different, named lens).
- **Every new move validates on import** and appears in the audit trail. New
  relations/link-rels must be added to `store.validateRecord`,
  `knownRelation`, `Export`/`ExportHistory`, **and `store.History` +
  `summarize`** (the `log` read has a hardcoded relation list).
- **Every new move auto-records the roster binding**: call `ensureRoster`
  inside the move's transaction (the Q8 pattern every existing move follows).
- **Every new surface exists three times**: CLI verb, JSON envelope in
  `discover.Current()` (+ CUE), MCP tool in `mcpserv` — including arg-struct
  ripple from changed proto signatures (`Promotes`, `Under`, `From`,
  `ReviewBy` fields).
- **Contract bump**: one bump to `0.10.0` covering the whole batch.
- **Determinism**: all new list outputs sorted; no map-iteration ordering.

## Data-model summary (all items)

New fact relations (namespaced per design §3.7):

| relation | args | written by |
|---|---|---|
| `dlktk/reframe` | `{disc, old, new, basis, author}` | `reframe` |
| `dlktk/value` | `{disc, node, value, author}` | `promote` / `--promotes` |
| `dlktk/audience` | `{disc, name, ranking: [string], author}` | `audience` |

Extended relations:

- `dlktk/link.rel` gains `synthesizes` (position→position) and `raised_from`
  (issue→position|argument).
- `dlktk/node` gains optional `tag` (`"assumption"` only, for now).
- `dlktk/decision` gains optional `review_by` (unix seconds).

**pudl compatibility (was risk R1) — resolved.** pudl's `AddFact` performs no
CUE validation on args (verified in the pinned module: the write path checks
only relation non-empty/non-reserved and args non-empty), so extended shapes
and new relations write fine. Residual, out-of-repo work: pudl bundles its own
copy of `dlktk.cue` (bootstrap schema) with closed definitions — pudl-side
*validation tooling* will flag the new fields/relations until pudl's copy is
synced from `dlktk schema`. Schedule a pudl PR after this lands; until then
the mismatch affects only pudl-side validators, not dlktk operation.

---

## Item 1 — untested vs battle-tested

- `internal/render`: `PositionView` gains `untested bool` (label IN and
  `len(AttackedBy)==0`). `positionNote` says "justified — but untested (never
  attacked)". `AgendaView` gains an `untested` section
  (`{issue, text, position, position_text}`) listing IN-and-unattacked
  positions on *undecided* issues; `AgendaText` renders it with a "stress-test
  before deciding" header.
- **Scope note (intended behavior, do not "fix"):** on a `select_one` issue
  with ≥ 2 positions every rival is attacked by the mutual-attack rule, so
  `untested` fires only for a sole position or `open`-cardinality positions —
  exactly the "first plausible answer sails through unexamined" case.
- `internal/render.Moves`: when `readyToDecide` finds a unique IN position
  that is unattacked, emit an `object` suggestion ("stress-test the
  unchallenged position before deciding") *before* the `decide` suggestion.
- `internal/check`: new finding kind `untested_decision`, severity `warning`
  (fails under `--strict`): standing decision whose position has no attackers
  in the current graph.
- Tests: render (untested flagged; reinstated and select_one-rival not
  flagged), agenda, check (decide an unattacked position → warning;
  attacked-and-reinstated → none).

## Item 2 — divergent stalemate advice

- `internal/render.advise`: stalemate branch appends the generative exits:
  a preference; or `synthesize` a hybrid; or `reframe` if the deadlock signals
  a false dichotomy. Contested branch mentions synthesis.
- `internal/render.Moves`: on stalemate, add a `synthesize` suggestion (args:
  issue + the UNDEC rival ids) and a `reframe` suggestion. The synthesize
  `effect` string must be honest: **the hybrid joins the select_one rivalry
  (stalemate becomes N+1-way) until parents are conceded or a
  preference/audience elevates it** — otherwise agents told to trust `moves`
  will loop synthesizing.
- Tests: golden advice strings; moves-on-stalemate contains synthesize/reframe
  with the caveat in the effect.

## Item 3 — `reframe` + `raise --from`

- `internal/ibis`: `Reframe{Disc, Old, New, Basis, Author}` type; `Graph`
  gains `Reframes []Reframe`, helpers `ReframedTo(issue) (string, bool)` and
  the reframed-node scope (an issue's positions + their `reachableAF`
  closure). New rel `RaisedFrom Rel = "raised_from"`. Legality:
  `CanReframe(old, decided)` — old exists, is an issue, not already reframed,
  **and has no standing decision** (a decided issue must have its decision
  superseded first, or be left alone; silently burying a standing decision
  would be the Q4 defect in a new hat). `CanRaise` gains a `from` node
  (must exist, be position/argument; mutually exclusive with `--parent`).
- `internal/store`: `relReframe`; `AddReframe`; `Reframes(disc, w)`; `Graph`
  loads them; export/import/validate; history export; **`History` +
  `summarize`**.
- `internal/proto`: `Reframe(disc, old, text, basis, card)` — one transaction:
  `ensureRoster`, legality, create new issue node (+card), `AddReframe`.
  Basis mandatory (IllegalMove otherwise). `Raise` gains `from` →
  `raised_from` link.
- Reads — exclusion semantics (precise, because agenda is node-keyed):
  - `agenda`: drop the reframed issue from `ready`/`unpopulated` **and drop
    the reframed issue's node scope (its positions and everything
    `reachableAF` from them) from `undecided`** — otherwise the dead framing's
    UNDEC nodes keep the litigation alive.
  - `check`: **skip `stalemate` and `untested_decision` findings for reframed
    issues** (they are no longer the live question); decision-drift is
    unreachable by construction (reframing a decided issue is illegal).
  - all-issues `status`/`replay` sweep: reframed issues excluded; a
    single-issue `status <old>` still works and reports `reframed_to` with
    advice "reframed → <new>". `IssueStatus` gains `reframed_to,omitempty`.
  - `tree`: `raised_from` links render the sub-issue under the node that
    spawned it (extend `childrenOf`; new legend glyph) **and
    `respondsToSomething` (root detection) must also treat an outgoing
    `raised_from` link as "not a root"** — otherwise the issue renders twice.
    Reframe lineage shown on the issue line ("↻ reframed → i:new").
- CLI: `reframe <issue> <text> --basis <label> [--card …]`; `raise --from
  <node>`. MCP: `reframe` tool; `raiseArgs` gains `from`.
- Tests: proto legality (missing basis, double reframe, non-issue, **decided
  issue rejected**), agenda node-scope exclusion (an UNDEC argument under the
  old framing disappears), check stalemate-suppression, tree golden for
  `raise --from` (renders once, nested), export→import round-trip including a
  reframe, `log` shows the reframe.

## Item 4 — `whatif` and `crux`

- `internal/af`: nothing (Build/Grounded reused on synthetic graphs).
- New `internal/render/whatif.go`:
  - `Hypothetical` = one of `{Object target}`, `{Prefer winner loser}`,
    `{Without node}`. Applying: `Object` adds a synthetic argument node —
    **`Kind: ibis.Argument`** (else `IsAFNode` is false and `Build` silently
    drops the attack) — with ids `~h1`, `~h2`… (clearly non-proquint) +
    objects_to link; `Prefer` appends a preference (validated via `CanPrefer`
    on the copy); `Without` removes a node and its incident links (verified
    label-equivalent to a real concede: `Build`/`positionsFor` ignore links
    with absent endpoints).
  - `WhatIf(g, issue, hyps)` deep-copies the graph, applies hyps, `af.Build` +
    `Grounded` both sides, returns `WhatIfView{issue, hypotheticals: [string],
    flipped: [LabelChange], outcome: [PositionView], stalemate bool}`.
  - `Crux(g, fw, labels, issue)`: for each *argument* in the issue's
    `reachableAF` scope, recompute labels on a copy without that node; collect
    those whose removal changes any position's label →
    `CruxView{issue, cruxes: [{node, text, author, flips: [LabelChange]}],
    note}`, sorted by flip count then id. The `note` documents the known
    limitation: **single-node removal misses jointly load-bearing sets** (two
    redundant arguments, neither individually pivotal). O(args²·edges) — fine
    at dlktk scale.
- Both reads honor `--as-of`/`--valid-at` by riding the same `when()` path
  `loadFramework` uses (counterfactuals over past states compose with
  `replay`).
- CLI: `whatif <issue> [--object <t>]… [--prefer <w>:<l>]… [--without <n>]…`
  (repeatable `StringArrayVar` flags, ≥1 required; `:` is safe — proquints
  never contain it); `crux <issue>`. MCP: `whatif` (array args), `crux`.
- Tests: whatif flips match making the real move; whatif writes nothing;
  whatif under `--as-of`; crux on the INTRODUCTION example identifies the
  rebuttal as load-bearing; prefer-cycle hypothetical rejected; synthetic
  attack actually lands (regression for the argument-kind trap).

## Item 5 — `worlds` (preferred extensions)

- `internal/af`: `PreferredExtensions()` on a (sub-)framework:
  1. Grounded first; collect the UNDEC residue `U`.
  2. **All checks run over the `Defeat` relation** (grounded is computed over
     Defeat; using raw Attack would manufacture conflicts from
     preference-neutralized attacks).
  3. Decompose `U` into Defeat-connected components; enumerate per component
     with conflict-free pruning (extend partial sets, cut on first conflict),
     then cross-combine components. Guard `worldsMaxComponent = 20` per
     component; on breach, return the envelope-level `too_contested` note
     (not an error — no new exit code; the read succeeds with worlds omitted
     and the guard explained).
  4. Candidate `S ⊆ U` is admissible iff `IN∪S` is conflict-free and every
     `s∈S` is acceptable w.r.t. `IN∪S`; keep **inclusion-maximal** sets (not
     maximum-cardinality — preferred extensions can differ in size).
  Soundness (review-verified): every preferred extension contains the
  grounded extension and excludes grounded-OUT nodes, so residue-only
  enumeration is complete; acceptability is monotone in the defending set.
  Deterministic order (sort members; sort worlds lexicographically).
- `internal/render/worlds.go`: `WorldsView{issue, worlds: [{in: [NodeRef],
  distinguishing: [id]}], robust: [NodeRef], contingent: [NodeRef], hopeless:
  [NodeRef], too_contested: bool, note}` — robust = IN in every world;
  contingent = IN in ≥1 but not all; hopeless = position IN in none. Text
  renderer labels worlds A, B, C….
- CLI `worlds <issue>` — **issue-scoped only** (whole-discussion worlds are
  the cross-product of independent issues' residues: exponential and
  useless). Sub-framework = the issue's `reachableAF` node set with induced
  edges. MCP `worlds`.
- Tests: odd 3-cycle (1 world = grounded); mutual pair (2 worlds); **even
  4-cycle a→b→c→d→a (exactly two worlds {a,c}/{b,d} — exercises S-internal
  defense)**; **preference-neutralized attack inside the residue** (Defeat vs
  Attack regression); differing-size extensions (inclusion-maximality
  regression); component guard; render golden.

## Item 6 — values and audiences

- `internal/ibis`: `ValueTag{Disc, Node, Value, Author}`, `Audience{Disc,
  Name, Ranking []string, Author}`; `Graph` gains `Values map[node]string`,
  `Audiences map[name]Audience`. Legality:
  - `CanPromote(node, author)` — AF node; **author must own the node**
    (mirrors concede's ownership rule: a value changes the node's fate under
    every `--under` lens and is otherwise unretractable, so a stranger must
    not be able to stamp it); one value per (live) node — re-promote rejected.
    Changing a value = the owner concedes the node and restates it.
    **Dangling value facts (node absent/retracted) are ignored on graph
    load** — they are the expected residue of concede-and-restate.
  - `CanAudience(name, ranking)` — ranking ≥ 2 distinct values; name not
    currently declared. **Audience supersession exists** (see below); silent
    redefinition does not.
- `internal/store`: `relValue`, `relAudience`; add/read; `Graph` loads (values
  filtered to live nodes); export/import/validate/history-export;
  **`History` + `summarize`**; `SupersedeAudience(disc, name)` — invalidates
  (closes the vt interval of) the current audience fact(s) by name, the
  `SupersedeDecision` pattern.
- `internal/proto`: `Promote(disc, node, value)`; `DeclareAudience(disc,
  name, ranking, supersede bool, basis string)` — a re-declaration requires
  `--supersede --basis` (the Q4 pattern: retiring a ranking that every
  robustness verdict depends on must record why); both run in one transaction
  with `ensureRoster`. `Propose`/`Support`/`Object` gain an optional
  `promotes` value written in the same transaction.
- `internal/af`: `BuildUnder(g, audience)` — as `Build`, plus audience
  neutralization with **explicit cross-mechanism precedence**:
  - For an attack `a→b`: if a *pairwise* (transitively closed) preference
    exists on the pair `{a,b}` **in either direction**, the pairwise relation
    alone decides survival and the audience is ignored for that pair — a
    recorded dialectical move outranks the systematic lens.
  - Otherwise the attack is neutralized iff both nodes carry values and the
    audience ranks value(b) strictly above value(a).
  - **Why (review finding 1, blocker):** independent composition of the two
    filters is unsound — `prefer a b` plus an audience ranking value(b) above
    value(a) would neutralize *both* directions of a symmetric select_one
    rivalry, labelling both rivals IN (the Q2 collapse). Per-pair precedence
    restores antisymmetry: pairwise is antisymmetric by cycle rejection,
    audience-only is antisymmetric because a strict value order blocks at most
    one direction. `BuildUnder` also records audience-neutralized pairs
    (`AudienceBlocked map[[2]string]string`) for explanation.
- `internal/render`:
  - `status --under <a>`: same `IssueStatus` envelope plus `under` field.
  - `AudiencesView{audiences: [{name, ranking}], issues: [{issue, text,
    baseline: {position: label}, by_audience: {name: {position: label}},
    robust: [id], sensitive: [{position, verdicts: {audience: label}}]}]}`;
    robust = IN under baseline *and* every **currently-declared** audience
    (superseded audiences are vt-closed and excluded by the default read).
  - `explain --under` gains audience-blocked attack annotations.
- CLI: `promote <node> <value>`; `--promotes` on `propose`/`object`/`support`;
  `audience <name> <v1> <v2>…` (move; `--supersede --basis` to re-declare) /
  `audience` (list); `audiences` (the report); `--under <name>` on
  `status`/`explain`/`worlds`. MCP: `promote`, `audience`, `audiences` tools;
  **`proposeArgs`/`attachArgs` gain `promotes`, `statusArgs`/`explain`/
  `worlds` args gain `under`** (proto signature ripple).
- Tests: BuildUnder neutralization (attack fails against higher-ranked value;
  unvalued nodes unaffected; symmetric select_one rivals with ranked values
  resolve without any `prefer`); **the composed pairwise+audience case: with
  `prefer a b` recorded and an audience favoring b's value, exactly one rival
  is IN** (blocker regression); promote ownership rejected for non-owner;
  dangling value ignored after concede; audience re-declare without
  `--supersede` rejected, with it the report drops the old ranking;
  round-trip; `log` shows promote/audience.

## Item 7 — `synthesize`

- `internal/ibis`: rel `Synthesizes Rel = "synthesizes"`. Legality
  `CanSynthesize(issue, froms)`: issue exists+is issue; ≥2 froms; each from is
  a position responding to that issue; froms distinct.
- `internal/proto`: `Synthesize(disc, issue, text, froms, promotes)` — one
  transaction: `ensureRoster`, position node + `responds_to` + one
  `synthesizes` link per parent.
- `internal/render`: `show` lists synthesizes links for free (it iterates all
  links — verified). `tree`: synthesizes links are *not* child edges;
  the position's line gains a `⊕ from p:a+p:b` annotation. Moves-on-stalemate
  suggests it with the honest effect string (item 2).
- CLI `synthesize <issue> <text> --from <p> --from <p> […] [--promotes v]`;
  MCP `synthesize`.
- AF: no change (`synthesizes` ignored by `Build`; select_one rivalry applies
  to the hybrid like any position — including vs its parents, correctly).
- Tests: legality (1 parent rejected, parent from another issue rejected);
  labelling unchanged by lineage; tree annotation; round-trip.

## Item 8 — `assume`

- `internal/ibis`: `Node.Tag string` (`json:"tag,omitempty"`; only
  `"assumption"` accepted at validation). Helper `g.Assumptions()`.
- `internal/proto`: `Assume(disc, target, text)` = `attach` with
  `ibis.Supports` + tag (inside the shared transaction pattern).
- `internal/render`: `AgendaView` gains `assumptions []NodeRef` —
  **undischarged** = tag=assumption, no incoming `objects_to` *and* no
  incoming `supports` (never examined), label ≠ OUT. `NodeView`/tree show the
  tag (`[assumption]`).
- `internal/check`: warning `defeated_assumption`: a standing decision whose
  position has a supports-path (transitive over `supports` links into the
  position) from an assumption currently labelled OUT.
- CLI `assume <target> <text>`; MCP `assume`. Import validation accepts the
  tag; CUE schema updated.
- Tests: agenda lists undischarged only (objected-to assumption drops off);
  check warning fires only when the OUT assumption supports the *decided*
  position; labelling untouched by tags.

## Item 9 — playbooks

- `AGENTS.md` + `skills/dlktk-dialectic/SKILL.md`: add a "Diverge before you
  converge" section (quotas: no `prefer` until ≥2 positions from ≥2 authors;
  one generative move per persona per round; devil's-advocate rotation against
  untested IN positions), the new personas (Reframer, Analogist,
  First-Principles, stakeholder advocates + audience declaration), and the new
  verbs woven into the loop (`agenda` sections `untested`/`assumptions`;
  `whatif`/`crux`/`worlds` as the exploration reads; the stalemate decision
  tree: synthesize → reframe → prefer, **with the synthesize caveat that the
  hybrid must then be elevated or its parents conceded**).
- `examples/deliberate.sh`: extend to exercise a divergence phase (a second
  proposer, an assumption, a synthesis) so CI runs the mechanics.
- README: new verbs one-liner each; INTRODUCTION §7 pointer.

## Item 10 — `--review-by`

- `internal/ibis`: `Decision.ReviewBy int64` (`json:"review_by,omitempty"`).
- `internal/proto`: `Decide`/`Supersede` accept `reviewBy int64` (0 = none);
  validation: must be in the future at decision time.
- `internal/check`: finding `review_due`, severity `warning` (fails under
  `--strict`): standing decision with `review_by` < now. "Now" = `--as-of`
  when given, else wall clock (plumb `nowUnix` into `check.Run`; both call
  sites — main.go and mcpserv — updated). Text advice: "re-affirm or revise
  via supersede".
- CLI: `--review-by T` on `decide`/`supersede`; shown in status/show decided
  views (`DecidedView.ReviewBy`). MCP arg structs extended.
- Tests: future validation; check fires after horizon (fixed nowUnix), not
  before; supersede with new horizon clears it; round-trip.

## Sequencing

1. Shared data-model plumbing for 3/6/7/8/10 (ibis types, store relations,
   import/export/validate, History/summarize, CUE text).
2. Items **1, 2** (render/check only).
3. Items **4, 5** (pure af/render reads).
4. Items **3, 7, 8, 10** (new moves).
5. Item **6** (largest; builds on the plumbing).
6. Item **9** (docs last, so they document what actually shipped) + README +
   `discover` bump to 0.10.0 + MCP additions verified end-to-end.
7. Full test pass, `go vet`, `check --all --strict` against `examples/` —
   **budget for the new warnings (`untested_decision`, `review_due`,
   stalemate-scope changes) newly firing on the existing q-series examples;
   update the examples or the CI invocation accordingly.**
8. Out-of-repo follow-up: PR pudl to sync its bundled `dlktk.cue` bootstrap
   schema from `dlktk schema`.

## Out of scope (explicitly)

- Signing/attestation (Q6's open half), embedding-based similarity search,
  bipolar AF (supports still never feed the labelling), live cross-discussion
  edges (Q5 stands; the Analogist cites by value), preferred-semantics-based
  *deciding* (grounded stays the referee), joint (multi-node) crux search.

## Review findings incorporated

An adversarial review of the first draft produced 15 findings; all accepted:

1. **(blocker)** Item 6: independent pairwise+audience neutralization could
   label both select_one rivals IN → replaced with per-pair precedence
   (pairwise preference wins the pair; audience applies only to
   preference-free pairs) + a dedicated regression test.
2. Item 5: enumeration explicitly over `Defeat` (not `Attack`);
   inclusion-maximal (not max-cardinality); even-4-cycle and
   neutralized-attack-in-residue tests added.
3. Item 5: 2^24 guard was an order of magnitude too generous → per-component
   decomposition with pruning, guard 20/component, `too_contested` as an
   envelope note (no new exit code); `worlds` is issue-scoped only.
4. Item 6: `promote` now requires node ownership; dangling value facts
   ignored on load; the change-a-value path documented.
5. Item 6: audiences gained a supersession path (`--supersede --basis`,
   vt-close the prior fact) so retired rankings stop poisoning the
   robustness report.
6. Item 3: agenda exclusion widened to the reframed issue's node scope;
   check's stalemate warning suppressed for reframed issues; reframing a
   decided issue is illegal (supersede first).
7. Item 3: `raise --from` also updates tree root detection
   (`respondsToSomething`), not just `childrenOf`.
8. R1 resolved by inspection (pudl AddFact does not validate args); fallback
   deleted; pudl bootstrap-schema sync scheduled as follow-up.
9. New relations added to `store.History`/`summarize` so `log` sees them.
10. MCP arg-struct ripple from proto signature changes listed explicitly.
11. Synthesize-at-stalemate effect string made honest (hybrid joins the
    rivalry; must be elevated or parents conceded).
12. whatif/crux: synthetic nodes are `Kind: argument`; `--as-of` honored;
    crux's single-node limitation documented in the envelope.
13. `too_contested` returned as an envelope note instead of an unclassified
    typed error.
14. Item 1's select_one behavior documented as intended; step 7 budgets for
    new warnings on existing examples.
15. All new moves call `ensureRoster` inside their transactions; `audience`
    goes through proto (not the CLI-side shortcut the `roster` bind uses).
