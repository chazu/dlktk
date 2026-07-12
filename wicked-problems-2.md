# dlktk and wicked problems, arc two — hardening convergence

*A follow-up to [`wicked-problems.md`](wicked-problems.md). That batch
(contract 0.10.0) built the divergence machinery and it works: the first
full exercise ([`examples/make-prorgamming-fun-again`](examples/make-prorgamming-fun-again/SESSION-REPORT.md))
produced genuinely divergent exploration — seven personae, three issues, a
provenance-tracked reframing cascade, and a computed value-conflict map.
This document is about what the exercise exposed at the other end: the
**convergence endgame is now the weakest phase**. The final synthesis was a
four-part bundle that reached `decide` without ever receiving a substantive
objection, and the tool not only failed to flag that — it structurally
couldn't. The items below fix the mechanics that let an unexamined winner
through, and the playbook gaps that invited it.*

> **Status: draft, revised** per the adversarial review recorded at the end
> (two independent reviewers, 28 findings consolidated to 19; all accepted).

## The diagnosis

The exercise's decided root position — the "theory-anchored agentic loop" —
was a concatenation of four parents, elevated by two subsumption preferences,
and decided without one direct objection. Post-hoc it is easy to name the
attacks it deserved (its cost is the *sum* of its parents' costs; the
economic-erosion argument that killed one parent applies with more force to
the bundle). Nobody made them, and every mechanism that should have prompted
someone was blind:

- The `untested` agenda section and `check --strict`'s `untested_decision`
  never fired, because select_one **rivals count as attackers** — on any
  multi-position issue, every position is trivially "attacked" and the
  epistemic intent of 0.10.0's item #1 is defeated.
- `synthesize` creates an **opaque node that inherits none of its parents'
  objections**, so bundling N positions sheds N sets of recorded criticism.
  The engine rewards concatenation.
- The **subsumption preference** (`prefer hybrid parent --basis subsumption`)
  is the natural companion move, and it is a dodge: if the hybrid truly
  contains a parent, the parent's unanswered objections apply to it — burying
  the parent with a preference buries them too.
- Nothing in the graph or playbook represents **adoption cost**, so a bundle
  whose parts are individually defensible and jointly unadoptable is
  invisible.
- The session **closed over a live warning**: `untested_decision` fired on
  the governance decision and the orchestrator finished anyway, unremarked.
  Warnings that obligate nothing are noise in training data for every future
  agent.
- Three smaller findings from the same session: `roster` emits one row per
  move instead of per binding; `show` on a reframed-to issue omits the
  `reframes` lineage; and the *installed* copy of the dialectic skill was
  stale, which would have silently reverted an agent to the pre-0.10.0
  convergent playbook.

A design constraint runs through everything below, sharpened by the review:
**the adversary is the orchestrator itself.** A single mind plays all the
personae, types every `--author` flag, and authors both the synthesis and its
scrutiny. Mechanisms that can be satisfied by one mind talking to itself —
a strawman self-objection, a token exclusion, a ritual cost objection
immediately rebutted — are not safety nets; they are laundering steps. Every
trigger condition in this batch is therefore stated in terms the graph can
check (authorship, defeat status, discharge links), not in terms of prose
compliance.

Items are grouped and ordered by leverage-per-effort: first the
labelling-adjacent fixes, then synthesis discipline, then process/playbook,
then repairs. The labelling itself — grounded semantics as sole referee — is
untouched by every item.

---

## 1. Tested-ness must ignore rival attacks — and resist self-dealing

**What.** Everywhere the system asks "was this position examined?" — the
agenda's `untested` surfacing, the `moves` stress-test suggestion, `check
--strict`'s `untested_decision` — a position counts as **tested** only if it
has at least one *substantive* attack: an `objects_to` edge that

1. is **authored by someone other than the position's author**, and
2. **participates in the defeat relation** — i.e. it was live against the
   position, and if it no longer defeats, it was beaten by a counter-argument
   rather than neutralized by a preference.

Select_one rival edges never count. Tested-ness is computed from the raw
links plus the defeat relation (`g.Links` + `fw.Defeat`), not from the
merged attack set, which has no provenance.

**Why.** This is the root cause of the exercise's failure: the one feature
built to catch unexamined winners is structurally blind on multi-position
issues. The two extra conditions come from the review, which found the naive
definition ("any `objects_to`") gameable in both directions available to a
single orchestrating mind: a weak **self-objection** clears the bar (condition
1 blocks it), and `prefer <position> <objection>` **neutralizes the attack
while the edge still counts** — the position gets credit for surviving a test
it was excused from (condition 2 blocks it). One residue is accepted and
documented: a rival's author writing an objection that merely restates the
rivalry passes the proxy. The proxy is authorship + defeat participation, not
content; content is the personae's job.

**Presentation (review-driven).** The broad definition applies to `check
--strict` in full. The **agenda**, however, surfaces untested positions only
when they are *decide-adjacent* — the issue is `ready`, or the node is the
target of a suggested `prefer`/`decide` — and names the single most relevant
node rather than listing every fresh position mid-divergence. During the
divergence phase every position is briefly untested by design; flagging all
of them trains agents to skim the section, which is how the exercise's hybrid
slipped through. Relatedly, when a node is both contested and untested,
`moves` and the stalemate advice must not offer `prefer` as a co-equal exit:
the preference suggestion is annotated ("this position is untested — object
first, then prefer"), so the advice states one ordering instead of two
options.

**Migration.** Redefining tested-ness flips shipped stores: the exercise's
own decided hybrid fails the new check, and that is the honest outcome — the
example's decision should be superseded on-record as part of landing this
item (dogfooding the mechanism), and the implementation step must budget for
newly-firing warnings on all existing examples, as the 0.10.0 plan did.

## 2. Syntheses surface their parents' unanswered objections — with a discharge condition

**What.** When a position has `synthesizes` lineage, `show`, `why`, and
`moves` on it list each parent's **undefeated objections** as *inherited
questions*, computed over the **transitive closure** of `synthesizes` links
(so chaining H1 = A+B, H2 = H1+C cannot launder A's and B's critics).
Inherited questions are dischargeable, and discharge is machine-readable: a
new inert link relation **`addresses`** (hybrid-side argument → parent
objection), written via an `--answers <objection-id>` flag on
`object`/`support`. An inherited question is *open* until some node
addresses it — either by re-aiming it (an `object` against the hybrid that
`--answers` the parent objection: "this still applies") or by dismissing it
(a `support` on the hybrid that `--answers` it: "here is why the hybrid
escapes it"). `addresses` links are evaluator-inert, consistent with the
§3.5 rule that only `objects_to` feeds the labelling.

**Why.** A hybrid should not enter the arena cleaner than its content — but
the first draft's version ("dismiss with a support") had no linkage between
answer and question, so the review found it degenerates into either a
permanent nag (nothing ever discharges) or a free pass (any support clears
everything), with N×M perfunctory "doesn't apply" nodes as collateral. The
small schema addition is the price of the mechanism meaning anything: with
`addresses` links, "which parent criticisms has this synthesis actually
answered?" is a computable question, which items #3 and #5 then build on.
Auto-creating attack edges remains wrong — some parent objections genuinely
don't transfer, and that is often the point of the synthesis.

**Presentation.** One composite prompt, not three: `moves` on a synthesis
emits a single "stress-test this hybrid" suggestion that lists the open
inherited questions as the candidate attacks and names the Adopter's cost
question (item #5) among them. The review was blunt that an untested-entry
plus an inherited-questions list plus a persona obligation, uncoordinated,
is nag fatigue — and all three get skimmed.

**Boundary.** `reframe` severs lineage by design (positions do not carry
over). A hybrid re-proposed under a new framing therefore inherits nothing —
item #1's untested check is the only guard on that path, which is one more
reason its hardening is the linchpin of the batch.

## 3. Flag the subsumption-preference dodge — keyed to the triage

**What.** When `prefer <winner> <loser>` is stated and the winner has
(transitive) `synthesizes` lineage to the loser, the move result carries a
warning — and `check --strict` reports **`self_elevated_synthesis`** —
unless the loser's inherited undefeated objections have all been
**addressed** on the winner (item #2's links), with at least one address
authored by someone other than the winner's author. The move remains legal.
The check finding is a *current-graph* property: an addressing node conceded
after the preference re-arms it. When `untested_decision` (item #1) already
fires on the same node, `self_elevated_synthesis` is suppressed — one defect,
one finding.

**Why.** The first draft's escape hatch ("any one substantive objection
clears the warning") was a one-token bar, and the review showed item #5 as
then drafted would have *mechanized* clearing it: a ritual Adopter objection,
promptly rebutted by the synthesis author, and every warning goes quiet —
the rubber stamp with extra steps. Keying the escape to the discharge of the
*specific loser's* open questions makes the warning say what it means: you
may bury a parent under your hybrid when the parent's critics have been
answered on the hybrid, not before.

**Timing (review-driven).** The intervention moves upstream of the mistake:
`moves` withholds or annotates the `prefer` suggestion while the winner's
inherited questions are open ("address the open questions on <hybrid> first,
then prefer"), because a warning delivered *after* an append-only move is
advice about the past. The move-result warning remains as a backstop and
embeds the concrete remedial command.

## 4. A synthesis must say what it drops — checkably

**What.** `synthesize` gains a repeatable **`--drops "<text>"`** flag (zero
or more, intended one per parent) recording what the hybrid *excludes*, as
node metadata rendered by `show`/`tree`. The move result warns when a
synthesis with ≥3 parents records no drops; `check --strict` reports
**`bundle_synthesis`** for a decided synthesis with ≥3 parents and no
recorded drops. The skill states the rule in one line: *a synthesis that
drops nothing is a bundle*; the `synthesize` effect string in `moves`
carries the same sentence.

**Why.** The first draft made this prose-only, and the review called it
correctly: unenforceable, satisfiable by a token clause, "held against" the
author by the same mind that wrote it. A flag is machine-checkable and
cheap; it also gives item #5's Adopter its trigger data (what was retained)
and gives reviewers of the exported dialectic a structured answer to "what
did the losing positions lose?" The threshold (≥3 parents) keeps honest
two-parent recombinations — like the exercise's sub-issue synthesis, which
genuinely fused two halves — friction-free.

## 5. An adoption-cost persona in the playbook — conditional, operational

**What.** Documentation only. Add an **Adopter** persona: argues from what a
real team will actually sustain. Operationally (the tool constrains this —
`promote` requires node ownership, so the Adopter cannot tag others' nodes):
the Adopter tags *its own objections* with `--promotes adoptability` and may
declare an `adoptability`-first `audience`, feeding the robustness report.
Its obligation is **conditional and substantive**: it must object to a
synthesis when the synthesis retains ≥2 parent mechanisms (per the `--drops`
record, item #4), and the objection must state a cost model — who does the
added work, when, and what they stop doing — not a slogan. Per item #8, the
Adopter's turn should run under a different author than the synthesis
author, and where the harness allows, as a separate agent process.

**Why.** IBIS tracks logical defeat; nothing represents feasibility, so the
argument pool under-produces the one objection bundles deserve most. The
review pushed back in both directions on the first draft: an *unconditional*
obligation to attack every synthesis is either a reflexive veto (legitimate
hybrids born OUT under template objections) or Kabuki (ritual objection,
ritual rebuttal, warnings cleared — see item #3). The conditional trigger
scopes the obligation to the shape that earns it — retained multi-mechanism
bundles — and the cost-model requirement plus author separation make the
exchange harder to fake by one mind on autopilot.

## 6. Cardinality guidance — after specifying the open-issue lifecycle

**What.** Two parts, strictly ordered. **First, mechanics:** specify and
implement closure for `open`-cardinality issues — `decide` may be recorded
**per position** on an open issue (multiple standing decisions, each
drift-checked independently by the existing `DecisionDrift` logic; `agenda`'s
`ready` lists each undecided-but-justified position; `supersede` targets the
per-position decision). **Then, guidance:** the skill and AGENTS.md gain the
rule of thumb — *if candidate answers could compose (process tweaks,
practices, guidelines), raise with `--card open`; use select_one only when
answers genuinely exclude one another; expect a select_one issue between
partial answers to end in a synthesis* — plus a short worked open-issue
example, and the stalemate advice mentions the same hint when it suggests
synthesis.

**Why.** The exercise's root issue seated six largely complementary process
changes as mutual rivals because select_one is the default and nothing
suggested otherwise; the false rivalry manufactured the bundle. But the
review caught the first draft shipping the signpost without the road: today
`Decision` holds exactly one position and a second `decide` is rejected, so
an open issue with four surviving practices *has no closure story* — the
guidance would have routed agents into a dead end whose only exit is
re-bundling everything into one synthesis at close time, recreating the
exact failure at a different moment. Mechanics first, then the sentence.

## 7. Closure without a verdict: record the map as the outcome — with drift semantics

**What.** For issues whose contest is genuinely value-driven, allow honest
closure without a winner: **`decide <issue> --map --basis <label>
--review-by <T>`** records a decision whose object is the issue's
audience-conditional map. Guardrails, all from the review:

- **Legality:** `--map` is an illegal move (exit 2) unless the issue is
  *actually audience-sensitive right now* — ≥2 currently-declared audiences
  and ≥1 position whose verdict differs across them (or from baseline). No
  audiences, no map. `--review-by` is **mandatory** for map decisions.
- **Storage:** the decision fact records only `{kind: map, basis,
  review_by}` — verdicts are **not snapshotted**. The decision-time map is
  derived bitemporally (evaluate as of the decision's transaction time),
  which the store already supports; a snapshot would just be a second copy
  that can silently disagree with the first.
- **Drift:** `check` recomputes the current map and reports **`map_drift`**
  when it differs from the decision-time map — a position's per-audience
  verdict flipped, an audience was superseded, or the issue now has a clear
  robust winner. Re-affirmation or conversion to a conventional decision
  goes through `supersede` (map→position and position→map supersession link
  by issue, with the superseded decision's kind recorded).
- **Standing residue:** a mapped issue carries a non-fatal check note,
  **`mapped_pending_governance`**, until a governance issue ("whose ranking
  should govern?") raised from it exists — mirroring the exercise's own good
  pattern, where naming the value conflict as an issue was the honest move.

`check` treats a mapped issue as closed for stalemate purposes.

**Why.** The exercise's most defensible output was the audiences finding —
*nothing is robust; the real decision is whose values govern* — and the
session still converged to a single winner, partly because the loop
terminates in `decide` and the agenda nags `ready` toward it. Rittel's
"solutions are better/worse, not true/false" implies the map *is* sometimes
the deliverable. But both reviewers converged on the same two failure modes
in the first draft: with no precondition, `--map` is a **universal stalemate
silencer**, strictly cheaper than arguing (map everything, decide nothing,
CI stays green — the no-stopping-rule pathology in new clothes); and with
frozen verdicts and no drift check, the one new decision kind would be
exempt from the living-constraint mechanism that is dlktk's strongest
property. The guardrails price the move honestly: a map decision is
available exactly when there is a real map, it expires on a clock, it is
re-checked like every other decision, and it points at the governance
question it defers.

## 8. Genuine adversarial separation — honest about what is checkable

**What.** Documentation (skill + AGENTS.md) plus one check finding. The
skill's *default procedure* for the synthesis stress-test becomes: **run the
devil's-advocate turn as a separate agent process whose only context is the
exported graph** (`dlktk export` output or read access to the store — no
shared conversational state with the synthesis author); synthesis author,
devil's advocate, and decider use three distinct `--author` identities.
The docs state plainly that roster separation proves **attribution, not
independence** — `--author` is self-assigned, and the exercise itself had
seven author strings and one mind. The one graph-checkable signal is added
to `check --strict`: **`single_author_convergence`** — a decided synthesis
where every substantive objection against it shares an author with the
synthesis, or where the decider's author equals the synthesis author.

**Why.** The first draft claimed "the roster makes the separation auditable
after the fact," and the review refuted it with the draft's own evidence:
a roster check on distinct author strings is satisfied by precisely the
failure case it targets. The honest version keeps the process-isolation
recommendation (every current harness can spawn a context-isolated
subagent), drops the false assurance, and adds the finding that catches the
echo chamber *regardless of how many author strings it wore* — because it
tests the shape of the scrutiny, not the names on it.

## 9. Warnings must obligate something

**What.** Playbook rule with a check to back it: a move-result warning or a
strict finding on an issue is a **work item** — the persona loop must either
resolve it before the next convergent move (`prefer`/`decide`/`supersede`
on that issue) or record the override rationale in that move's `--basis`.
`check --strict` gains **`unacknowledged_warning`**: a decided issue with a
finding that predates its decision and a decision basis that does not
acknowledge it. Additionally — making good on 0.10.0 item #9's unfulfilled
"mechanically checkable" claim — the process-rule family becomes findings
computed from author/timestamp data already on every node:
**`premature_preference`** (a `prefer` recorded before the issue had ≥2
positions from ≥2 authors), plus `bundle_synthesis` (item #4) and
`single_author_convergence` (item #8).

**Why.** The exercise set the precedent this item exists to break:
`untested_decision` fired and the session closed over it without comment.
Every signal items #1–#8 add inherits that precedent unless a warning
carries an obligation — and the review's sharpest omission finding was that
*both* batches keep writing prose rules ("no prefer until…", "each persona
must…") that nothing checks, while the data to check them (authorship,
timestamps, link provenance) is already on every node. Findings are cheap;
hopes are not.

## 10. Repairs from the exercise

**What.** Three small fixes found during the session:

- **`roster` dedup:** report distinct (author, role) bindings, not one row
  per move (36 rows for 7 bindings in the exercise).
- **`show` lineage completeness:** every node view must render its lineage
  links in **both directions** — `reframes` from the new issue's side (today
  it is visible only on the old issue in `tree`), `raised_from`,
  `synthesizes`, and the new `addresses` (item #2).
- **Skill/contract version handshake:** the stale-installed-skill failure
  (F4) recurs unless something *consumes* a version marker. The skill's
  setup step gains a mandatory first move: run `dlktk discover`, compare the
  contract version against the minimum the skill declares in its own body,
  and stop with a visible warning on mismatch — the agent that loads the
  skill is the checker, which works on machines `make` never touched. A
  `make install-skill` target syncs the repo skill to `~/.claude/skills` for
  convenience, but the handshake, not the Makefile, is the guarantee.

**Why/Utility.** All three are audit-surface integrity: the roster is the
attribution audit, `show` is the lineage audit, and the skill is the
behavior contract. Each was misleading or missing in the first real
exercise; the review added the observation that a version line nobody reads
is F4 with extra steps, hence the handshake.

---

## How the items fit together

The 0.10.0 batch built three loops (novelty, leverage, honesty). This batch
closes the loop those three feed into — the **convergence-integrity loop**:
a synthesis inherits its parents' open questions transitively and discharges
them on the record (#2), states what it drops (#4), cannot be self-elevated
while those questions are open (#3), has a designated predator when it
retains too much (#5), and cannot be decided untested (#1). When the honest
outcome is a value map rather than a winner, that is recordable — with
preconditions, an expiry, and drift-checking (#7). Items #6 and #8 prevent
the two failure shapes from forming at all — false rivalry from cardinality,
and self-judging orchestrators — #9 makes the process rules cost something
to break, and #10 keeps the audit surfaces truthful.

The through-line, post-review: every mechanism assumes the orchestrator is
the adversary, and every rule is either computed from the graph or paired
with a finding that is. Prose discipline is for personae; the referee only
trusts edges.

---

## Review findings incorporated

Two independent adversarial reviews of the first draft (a semantics/purity
lens and an agent-behavior lens) produced 28 findings, consolidated to 19;
all accepted:

1. **(blocker)** Item 7: map decisions had no drift semantics — frozen
   verdicts exempt from the living-constraint mechanism → verdicts are not
   snapshotted; the decision-time map is derived bitemporally; new
   `map_drift` finding; supersession carries the superseded decision's kind.
2. **(blocker)** Item 7: `--map` with no precondition is a universal
   stalemate silencer, legal even with zero audiences → illegal unless ≥2
   declared audiences with differing verdicts; `--review-by` mandatory;
   `mapped_pending_governance` until a governance issue exists.
3. **(blocker)** Item 1: "any `objects_to`" is gameable by strawman
   self-objection and by preference-neutralized objections (the edge counts,
   the test was excused) → substantive = different author **and**
   participates in defeat (beaten only by counter-argument, not preference);
   computed from raw links + defeat relation, since the merged attack set
   has no provenance. Residual rival-author-restates-rivalry case accepted
   and documented.
4. Item 3: the one-objection escape bar was a token, and item 5's
   obligation as drafted would have mechanized clearing it (ritual
   objection, ritual rebuttal) → escape keyed to discharge of the *specific
   loser's* inherited questions with ≥1 non-self-authored address; the check
   finding re-arms if an addressing node is later conceded.
5. Item 2: "dismiss with a support" had no linkage — permanent nag or free
   pass, plus N×M dead nodes → new evaluator-inert `addresses` relation via
   `--answers <id>`; discharge is computable; small schema addition
   accepted as the price of the mechanism meaning anything.
6. Item 2: single-level inheritance launders objections through chained
   syntheses → computed over the transitive `synthesizes` closure; `reframe`
   severing documented, making item 1 the sole guard on that path.
7. Items 6+4: open-cardinality issues had no closure story (`Decision`
   holds one position; second `decide` rejected), so the guidance routed
   composable questions into a dead end that ends in re-bundling → specify
   per-position decisions on open issues first, guidance second, worked
   example in the skill.
8. Item 8: "roster makes separation auditable" refuted by the exercise
   itself (seven author strings, one mind) → claim dropped; context-isolated
   subagent as default procedure; new `single_author_convergence` finding
   tests the shape of scrutiny, not the names.
9. Item 1: agenda-level flooding during the divergence phase habituates
   agents to skip the section → broad definition for `check --strict` only;
   agenda surfaces untested nodes only when decide-adjacent, ranked, one
   node not six.
10. Item 1: contradictory co-advice (undecided says "prefer works", untested
    says "don't") → `moves`/stalemate advice annotate or withhold `prefer`
    on untested targets; one ordering, not two options.
11. Item 3: a warning after an append-only move is advice about the past →
    intervention moved upstream into the `moves` suggestion; result warning
    kept as backstop with the remedial command embedded.
12. Item 4: prose-only drop rule unenforceable and self-reviewed →
    repeatable `--drops` flag, move-result warning at ≥3 parents with none,
    `bundle_synthesis` strict finding; two-parent syntheses stay
    friction-free.
13. Item 5: unconditional Adopter obligation is a reflexive veto or Kabuki;
    "more than one mechanism" undefined → conditional on ≥2 retained parent
    mechanisms per the `--drops` record; objection must state a cost model;
    distinct author per item 8.
14. Item 5: "owns the value adoptability" implied a move the tool rejects
    (`promote` requires node ownership) → persona spec rewritten
    operationally: `--promotes` on its own objections, `adoptability`-first
    audience.
15. Items 1+2+5: three uncoordinated prompts for the same act is nag
    fatigue → coalesced into one composite "stress-test this hybrid" `moves`
    suggestion listing inherited questions with the cost question among
    them.
16. Omission: warnings obligate nothing — the exercise closed over a live
    `untested_decision` unremarked → item 9 added: warnings are work items;
    `unacknowledged_warning` finding; process-rule family
    (`premature_preference` et al.) makes 0.10.0's "mechanically checkable"
    claim real.
17. Items 1+3 interaction: same defect, two findings → suppression rule
    (item 1's finding wins); finding kind renamed to snake_case
    (`self_elevated_synthesis`).
18. Item 1 migration: the new definition flips shipped stores, including the
    exercise's own decided hybrid → budget for newly-firing warnings on
    existing examples; supersede the example's decision on-record as
    dogfood.
19. Item 9 (repairs): a version line nobody consumes is F4 with extra
    steps → skill↔contract version handshake at session start, performed by
    the agent loading the skill; supersede-field semantics for map↔position
    transitions specified (link by issue, record superseded kind).
