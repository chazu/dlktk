# Implementation plan — the wicked-problems improvements

*Concrete plan for implementing the ten improvements in
[`wicked-problems.md`](wicked-problems.md). Written against the code as it
stands on this branch (contract 0.9.0). Each item lists the touched packages,
data-model changes, surfaces, and tests.*

## Ground rules (apply to every item)

- **Layering holds.** Only `internal/store` touches pudl; `internal/af` stays
  pure; `internal/proto` is the only mutator above store; legality lives in
  `internal/ibis`; envelopes in `internal/render`; contract in
  `internal/discover`; CLI in `cmd/dlktk`; MCP mirror in `internal/mcpserv`.
- **Grounded semantics stay the referee.** Nothing below changes how
  `status`/`decide`/`check` compute labels (the one deliberate exception:
  `status --under <audience>`, which is explicitly a different, named lens).
- **Every new move validates on import.** New relations/link-rels must be added
  to `store.validateRecord`, `knownRelation`, `Export`/`ExportHistory`, and the
  `discover.CUESchema` text (kept in lockstep with pudl's copy, see risk R1).
- **Every new surface exists three times**: CLI verb, JSON envelope in
  `discover.Current()` (+ CUE), MCP tool in `mcpserv`.
- **Contract bump**: one bump to `0.10.0` covering the whole batch, with the
  version-history comment listing the additions.
- **Determinism**: all new list outputs sorted; no map-iteration ordering.
- **Tests**: each item names its minimum tests; shared fixtures via small
  helper graphs in-package (the pattern `af_test.go`/`proto_test.go` already
  use).

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

**Risk R1 (pudl schema lockstep).** pudl ≥ v0.1.3 ships the `pudl/dlktk` CUE
package as a bootstrap schema, and CUE definitions are closed: if pudl
*enforces* args unification on `AddFact`, new fields (`tag`, `review_by`) and
new `rel` enum values would be rejected at write time. **First implementation
step is an empirical probe** (unit test writing a `dlktk/node` fact with an
extra field against a temp store). Fallback if enforcement bites: keep existing
relations byte-compatible and move the extensions to side relations
(`dlktk/tag {node, tag}`, `dlktk/review {disc, issue, review_by}`, and a
dedicated `dlktk/synthesis {disc, position, from: [string]}` instead of new
link rels). The plan below assumes the probe passes; the fallback changes only
the store layer, not the surfaces.

---

## Item 1 — untested vs battle-tested

- `internal/render`: `PositionView` gains `untested bool` (label IN and
  `len(AttackedBy)==0`). `positionNote` says "justified — but untested (never
  attacked)". `AgendaView` gains `untested []IssueRef`-like section
  (`{issue, text, position, position_text}`) listing IN-and-unattacked
  positions on *undecided* issues; `AgendaText` renders it with a "stress-test
  before deciding" header.
- `internal/render.Moves`: when `readyToDecide` finds a unique IN position that
  is unattacked, emit an `object` suggestion ("stress-test the unchallenged
  position before deciding") *before* the `decide` suggestion.
- `internal/check`: new finding kind `untested_decision`, severity `warning`
  (fails under `--strict`): standing decision whose position has no attackers
  in the current graph.
- Tests: render test (untested flagged, reinstated not flagged), agenda test,
  check test (decide an unattacked position → warning; attacked-and-reinstated
  → none).

## Item 2 — divergent stalemate advice

- `internal/render.advise`: stalemate branch appends the generative exits:
  "…a preference resolves this; or synthesize a hybrid of the rivals
  (`synthesize`), or reframe the question (`reframe`) if the deadlock signals a
  false dichotomy". Contested branch similarly mentions synthesis.
- `internal/render.Moves`: on stalemate, add a `synthesize` suggestion (args:
  issue + the UNDEC rival ids) and a `reframe` suggestion.
- Tests: golden advice strings; moves-on-stalemate contains synthesize/reframe.

## Item 3 — `reframe` + `raise --from`

- `internal/ibis`: `Reframe{Disc, Old, New, Basis, Author}` type; `Graph`
  gains `Reframes []Reframe` and helper `ReframedIssues() map[old]new`.
  New rel `RaisedFrom Rel = "raised_from"`. Legality: `CanReframe(old)` (old
  exists, is an issue, not itself already reframed); `CanRaise` extended for a
  `from` node (must exist, be position/argument — mutually exclusive with
  `--parent`).
- `internal/store`: `relReframe`; `AddReframe`; `Reframes(disc, w)`; `Graph`
  loads them; export/import/validate; history export.
- `internal/proto`: `Reframe(disc, old, text, basis, card)` — one transaction:
  legality, create new issue node (+card), `AddReframe`. Basis mandatory
  (IllegalMove otherwise). `Raise` gains `from` arg → `raised_from` link.
- Reads: reframed issues are dropped from `agenda` (all sections) and from the
  all-issues `status`/`replay` sweep (single-issue reads still work and mark
  the status `reframed_to`). `IssueStatus` gains `reframed_to,omitempty`;
  advice: "reframed → <new>". `tree`: `raised_from` links make the sub-issue
  render under the node that spawned it (extend `childrenOf`; the legend gains
  a glyph); reframe lineage shown on the issue line ("↻ reframed → i:new").
- CLI: `reframe <issue> <text> --basis <label> [--card …]`; `raise --from
  <node>`. MCP: `reframe` tool; `raise` gains `from`.
- Tests: proto (reframe legality: missing basis, double reframe, non-issue),
  agenda exclusion, tree lineage render, export→import round-trip including a
  reframe.

## Item 4 — `whatif` and `crux`

- `internal/af`: nothing (Build/Grounded reused on synthetic graphs).
- New `internal/render/whatif.go`:
  - `Hypothetical` = one of `{Object target}`, `{Prefer winner loser}`,
    `{Without node}`. Applying: `Object` adds a synthetic argument node
    (`~h1`, `~h2`… ids, clearly non-proquint) + objects_to link; `Prefer`
    appends a preference (validated against cycles — reuse `CanPrefer` on the
    copy); `Without` removes a node and its incident links (simulating a
    concede).
  - `WhatIf(g, issue, hyps)` deep-copies the graph, applies hyps, `af.Build` +
    `Grounded` both sides, returns `WhatIfView{issue, hypotheticals: [string],
    flipped: [LabelChange], outcome: [PositionView], stalemate bool}` (reuses
    the existing `LabelChange`).
  - `Crux(g, fw, labels, issue)`: for each *argument* in the issue's
    `reachableAF` scope, recompute labels on a copy without that node; collect
    those whose removal changes any position's label →
    `CruxView{issue, cruxes: [{node, text, author, flips: [LabelChange]}]}`,
    sorted by number of flips then id. O(args²·edges) — fine at dlktk scale.
- CLI: `whatif <issue> [--object <t>]… [--prefer <w>:<l>]… [--without <n>]…`
  (repeatable flags, at least one required); `crux <issue>`. Both pure reads.
  MCP: `whatif` (array args), `crux`.
- Tests: whatif flips match making the real move; whatif writes nothing
  (store untouched); crux on the INTRODUCTION example identifies the rebuttal
  as load-bearing; prefer-cycle hypothetical rejected.

## Item 5 — `worlds` (preferred extensions)

- `internal/af`: `PreferredExtensions()` — grounded first; collect the UNDEC
  residue `U`; if `len(U) > worldsMaxResidue` (24) return a typed
  `TooContestedError`. Enumerate subsets of `U` (attack-relation restricted):
  candidate `S` is admissible iff `IN∪S` is conflict-free and every element of
  `S` is acceptable w.r.t. `IN∪S`; keep maximal ones. Preferred extensions all
  contain the grounded IN set, so this enumeration is complete. Deterministic
  order (sort members, sort worlds lexicographically). Unit-tested against
  hand-computed examples: odd cycle (1 world = grounded), mutual pair (2
  worlds), the INTRODUCTION graph at its stalemate stage.
- `internal/render/worlds.go`: `WorldsView{issue?, worlds: [{in: [NodeRef],
  distinguishing: [id]}], robust: [NodeRef], contingent: [NodeRef], hopeless:
  [NodeRef], note}` — robust = IN in every world; contingent = IN in ≥1;
  hopeless = position IN in none. `distinguishing` = members not shared by all
  worlds (what you'd have to accept to live in this world). Text renderer
  labels worlds A, B, C….
- CLI `worlds [issue]`; scope to the issue's `reachableAF` sub-framework when
  an issue is given (build a sub-framework from the scoped nodes + edges),
  whole discussion otherwise. MCP `worlds`.
- Tests: af enumeration cases above; residue guard; render golden.

## Item 6 — values and audiences

- `internal/ibis`: `ValueTag{Disc, Node, Value, Author}`, `Audience{Disc,
  Name, Ranking []string, Author}`; `Graph` gains `Values map[node]string`,
  `Audiences map[name]Audience`. Legality: `CanPromote(node)` (AF node; one
  value per node — re-promote rejected, immutability ethos; to change,
  concede/restate); `CanAudience(name, ranking)` (name new, ranking ≥2
  distinct values).
- `internal/store`: `relValue`, `relAudience`; add/read; `Graph` loads;
  export/import/validate/history.
- `internal/af`: `BuildUnder(g, audienceName)` — same as `Build`, plus: an
  attack `a→b` is additionally neutralized when both nodes carry values and
  the audience ranks value(b) strictly above value(a). Implementation: after
  the pairwise-preference pass, filter `Defeat` again; record
  audience-neutralized pairs in a new `AudienceBlocked map[[2]string]string`
  (pair → "value(b)≻value(a)") for explanation. Pairwise `prefer` still
  applies (explicit dialectical moves outrank the systematic lens is *not*
  assumed — both filters apply independently; an attack survives only if
  neither blocks it).
- `internal/render`:
  - `status --under <a>`: same `IssueStatus` envelope plus `under` field.
  - `AudiencesView{audiences: [{name, ranking}], issues: [{issue, text,
    baseline: {position: label}, by_audience: {name: {position: label}},
    robust: [id], sensitive: [{position, verdicts: {audience: label}}]}]}`;
    robust = IN under baseline *and* every audience.
  - `explain` gains audience-blocked attack annotations when `--under` is set.
- CLI: `promote <node> <value>`; `--promotes` flag on
  `propose`/`object`/`support` (sugar: promote inside the same transaction —
  proto methods gain an optional value arg); `audience <name> <v1> <v2>…`
  (move) / `audience` (list); `audiences` (the report read); `--under <name>`
  flag on `status`/`explain`/`worlds`. MCP: `promote`, `audience`,
  `audiences`; `under` param on status/explain/worlds.
- Tests: BuildUnder neutralization (VAF textbook case: attack fails against
  higher-ranked value; unvalued nodes unaffected; symmetric select_one rivals
  with ranked values resolve without any `prefer`); audiences report robust vs
  sensitive; re-declare audience rejected; round-trip.

## Item 7 — `synthesize`

- `internal/ibis`: rel `Synthesizes Rel = "synthesizes"`. Legality
  `CanSynthesize(issue, froms)`: issue exists+is issue; ≥2 froms; each from is
  a position responding to that issue; froms distinct.
- `internal/proto`: `Synthesize(disc, issue, text, froms)` — one transaction:
  position node + `responds_to` issue + one `synthesizes` link per parent.
- `internal/render`: `show` lists synthesizes links both directions (already
  generic if `show` iterates all links — verify; extend rel names). `tree`:
  synthesizes links are *not* child edges (avoids double-parenting; positions
  already render under their issue); instead the position's line gains a
  `⊕ from p:a+p:b` annotation. Moves-on-stalemate suggests it (item 2).
- CLI `synthesize <issue> <text> --from <p> --from <p> […]`; MCP `synthesize`.
- AF: no change (`synthesizes` ignored by `Build`; select_one rivalry applies
  to the hybrid like any position — including vs its parents, correctly).
- Tests: legality (1 parent rejected, parent from another issue rejected);
  labelling unchanged by lineage; tree annotation; round-trip.

## Item 8 — `assume`

- `internal/ibis`: `Node.Tag string` (`json:"tag,omitempty"`; only
  `"assumption"` accepted at validation). Helper `g.Assumptions()`.
- `internal/proto`: `Assume(disc, target, text)` = `attach` with
  `ibis.Supports` + tag. Ownership/concede unchanged (it's a node).
- `internal/render`: `AgendaView` gains `assumptions []NodeRef` —
  **undischarged** = tag=assumption, no incoming `objects_to` *and* no
  incoming `supports` (never examined), and label ≠ OUT. `NodeView`/tree show
  the tag (`◦ assumption` glyph / `[assumption]`).
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
  `whatif`/`crux`/`worlds` as the exploration reads; stalemate → synthesize/
  reframe/prefer decision tree).
- `examples/deliberate.sh`: extend the scripted deliberation to exercise a
  divergence phase (a second proposer, an assumption, a synthesis) so CI runs
  the mechanics.
- README: new verbs one-liner each; INTRODUCTION §7 pointer.

## Item 10 — `--review-by`

- `internal/ibis`: `Decision.ReviewBy int64` (`json:"review_by,omitempty"`).
- `internal/proto`: `Decide`/`Supersede` accept `reviewBy int64` (0 = none);
  validation: must be in the future at decision time.
- `internal/check`: finding `review_due`, severity `warning` (visible always,
  fails under `--strict`): standing decision with `review_by` < now. "Now" =
  `--as-of` when given, else wall clock (plumb a `nowUnix` into `check.Run` for
  testability). Text advice: "re-affirm or revise via supersede".
- CLI: `--review-by T` on `decide`/`supersede` (parseTime); shown in
  status/show decided views (`DecidedView.ReviewBy`). MCP args extended.
- Tests: future validation; check fires after horizon (fixed nowUnix), not
  before; supersede with new horizon clears it; round-trip.

## Sequencing

1. **Probe R1**, then the data-model plumbing shared by 3/6/7/8/10 (store
   relations, ibis types, import/export/validate, CUE).
2. Items **1, 2** (render/check only) — immediate value, no schema risk.
3. Items **4, 5** (pure af/render reads).
4. Items **3, 7, 8, 10** (new moves).
5. Item **6** (largest; builds on the plumbing).
6. Item **9** (docs last, so they document what actually shipped) + README +
   `discover` bump to 0.10.0 + MCP additions verified end-to-end.
7. Full test pass, `go vet`, `check --all --strict` against `examples/`,
   golden-output sanity run of the INTRODUCTION walkthrough.

## Out of scope (explicitly)

- Signing/attestation (Q6's open half), embedding-based similarity search,
  bipolar AF (supports still never feed the labelling), live cross-discussion
  edges (Q5 stands; the Analogist cites by value), preferred-semantics-based
  *deciding* (grounded stays the referee).
