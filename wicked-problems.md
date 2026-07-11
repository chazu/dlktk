# dlktk and wicked problems — ten improvements for finding novel solutions

*Why dlktk, as built, is an adjudication engine; what wicked problems additionally
demand; and ten concrete improvements — each explained with its rationale and
expected utility. The companion document
[`wicked-problems-plan.md`](wicked-problems-plan.md) turns these into an
implementation plan.*

> **Status: implemented** (contract 0.10.0). All ten items shipped, revised
> per the adversarial review recorded at the end of the plan. User-facing
> documentation: README (feature summary), INTRODUCTION §7 (concepts),
> AGENTS.md + `skills/dlktk-dialectic` (the agent playbook), `dlktk discover`
> (the machine contract). This document remains the rationale.

## The diagnosis

There is a pleasing historical resonance in this project: IBIS was invented by
Horst Rittel — the same person who coined the term *wicked problem* — precisely
as his answer to wicked problems. dlktk therefore stands on the right capture
ontology. But nearly everything layered on top of that ontology is
**convergence machinery**:

- Grounded semantics is deliberately the *most skeptical* labelling — it only
  calls something justified when forced to.
- `moves` suggests attacks and preferences — moves that narrow the space.
- The stalemate advice steers agents to `prefer` — a value judgment that ends
  the contest.
- `agenda` drives toward `decide`; `check` polices decisions.

That makes dlktk an excellent *referee*. Wicked problems, as Rittel described
them, additionally need support for the **divergent** half of deliberation:

1. **There is no definitive formulation of a wicked problem** — the formulation
   *is* the problem, so reframing the question must be a first-class act.
2. **Every wicked problem is a symptom of another problem** — arguments should
   be able to spawn deeper questions.
3. **Solutions are not true/false but better/worse, and there is no ultimate
   test** — stakeholders rank values differently; a single verdict is often the
   wrong deliverable, while a map of the value conflict is exactly the right one.
4. **There is no enumerable set of solutions** — generating candidates (often by
   recombination) matters as much as adjudicating them.
5. **There is no stopping rule** — closure is always provisional.

An agent following the current playbook is systematically steered to narrow the
space and never to widen it. The good news: the architecture makes the fixes
cheap. `internal/af` is pure, the epistemic data several fixes need is already
computed, and a few fixes are documentation-only.

The ten improvements below are ordered roughly by leverage-per-effort within
their groups: first *surfacing what the engine already knows*, then *new lenses
over the same graph*, then *new moves*, then *process and provisionality*.

---

## 1. Surface epistemic status: "untested" ≠ "justified"

**What.** Distinguish a position that is IN because it *survived attack* from a
position that is IN because *nobody bothered to attack it*, everywhere labels
are shown; give the untested winners their own agenda section; have `moves`
suggest stress-testing an unchallenged position before suggesting `decide`; let
`check --strict` flag decisions whose position was never attacked.

**Why.** `af.GroundedSteps` already computes the distinction (`why:
"unattacked"` vs `why: "reinstated"`), but every surface collapses both into
`IN`. For a tool whose job is finding *good* answers to open-ended questions,
that collapse is the single most misleading thing in the output: an IN label
reads as "vindicated" when it often means "unexamined." Wicked problems are
exactly where the first plausible answer is least likely to be the best one —
the cheap, obvious candidate arrives first, draws no fire because nobody has
thought hard yet, and sails through `ready` → `decide` untested.

**Utility.**

- The agenda stops being only "resolve contests" and becomes "also stress-test
  the unexamined winner" — the cheapest possible way to get agents (and humans)
  generating novel objections, because it points at exactly the node that needs
  one.
- A decision that survived zero tests is the kind most likely to rot;
  `check --strict` flagging it turns "we never really examined this" from
  invisible into a CI conversation.
- Zero new semantics: the labelling is untouched; this is honesty about data the
  engine already has.

## 2. Give stalemates a divergent exit

**What.** Extend the stalemate advice (and the corresponding `moves`
suggestions) beyond "state a preference": also offer **propose a hybrid
position** (see #7) and **re-raise the issue under a different framing** (see
#3). Same for the `contested` advice.

**Why.** The Q3-dogfooded advice — "a preference resolves this; a new argument
helps only if it defeats from outside" — is technically correct about the
*labelling* and behaviorally wrong for wicked problems. A persistent stalemate
between rival positions is often a **diagnostic**: the issue is mis-framed, or
the rivals are a false dichotomy and the real answer combines them. The current
advice trains every agent that reads it to reach for `prefer` — the move that
ends deliberation with a value judgment — at precisely the moment when the
generative moves have the highest expected value.

**Utility.** This is a render-layer change with outsized behavioral effect,
because the advice string is the thing agents act on (the skill says "don't
invent moves outside `moves`"). It converts the tool's highest-friction moment
(deadlock) into its best prompt for novelty. It also keeps the honest core: a
preference *is* still listed — the point is to present the full option set, not
to hide the convergent exit.

## 3. Make reframing a first-class, lineage-tracked act

**What.** Two additions:

- **`reframe <issue> "<new text>" --basis <label>`** — raise a replacement
  issue and record a `reframes` lineage between new and old, with a mandatory
  basis. The old issue is marked *reframed* (excluded from the live agenda);
  positions do not carry over — under a new framing they mean different things,
  which is the Q7 argument.
- **`raise --from <node>`** — allow an issue to be raised *from* a position or
  argument, recording that provenance ("this objection revealed a deeper
  question"), not just from a parent issue.

**Why.** Rittel's first property — the formulation *is* the problem — means a
tool for wicked problems must treat reframing as a normal, recordable move, not
an out-of-band restart. Q7 already decided (correctly) that changing an issue's
meaning is a *new issue*; but with no recorded lineage, reframing is invisible
to the graph, to `tree`, and to postmortems — the most important events in a
wicked-problem deliberation leave no trace. Likewise "every wicked problem is a
symptom of another problem": today `--parent` only accepts an issue, so the
moment an *argument* reveals a deeper question, the connection is lost.

**Utility.**

- `tree` and `show` can display framing history and problem genealogy; `replay`
  can answer "how did our understanding of the question evolve?" — for wicked
  problems that trajectory *is* the deliverable, more than any single decision.
- A mandatory `--basis` extends the Q4 "force the reasoning to be captured"
  ethos to the reframing act, which is currently the *least* captured, most
  consequential move a team makes.
- Excluding reframed issues from the agenda stops agents from litigating a
  question the team has already moved past.

## 4. `whatif` — a counterfactual sandbox (and crux-finding)

**What.** A read-only hypothetical mode:

- `dlktk whatif <issue> [--object <target>]… [--prefer <winner>:<loser>]…
  [--without <node>]…` — apply hypothetical moves to an in-memory copy of the
  graph and print the **label diff** against the real labelling. Nothing is
  written.
- `dlktk crux <issue>` — built on the same machinery: for each argument bearing
  on the issue, compute the labelling *without* it and report the nodes whose
  absence changes which positions are IN — the **load-bearing** arguments.

**Why.** Because `af.Build` + `Grounded()` are pure and fast, hypotheticals are
nearly free — yet today an agent exploring "what would flip this?" must either
guess from `why`'s local hints or pollute the permanent record with speculative
moves and then concede them. That is a real deterrent to exploration in a store
whose ethos is "no edit, no delete." Crux-finding generalizes `why` from local
("who attacks this node") to global ("which claims does the whole verdict
actually rest on").

**Utility.**

- Agents can search the move space cheaply: "which single objection flips the
  decision?" becomes one read per candidate rather than a move + concede cycle.
  This makes deliberation *cheaper to explore than to pollute*, aligning the
  incentives with the append-only design.
- `crux` tells a human or agent exactly where novel thinking has maximum
  leverage: "the whole decision hinges on argument X" is the best possible
  prompt for directed creativity — attack X, or shore it up.
- For facilitators of a stuck discussion, `crux` identifies the *actual*
  disagreement, which in wicked problems is routinely not the one being argued
  loudest.

## 5. `worlds` — enumerate coherent alternatives (preferred semantics)

**What.** A read `dlktk worlds <issue>` that enumerates the **preferred
extensions** of the same argumentation framework — the maximal internally
coherent "worldviews" — and reports which positions are IN in *every* world
(robust), in *some* worlds (contingent), or in *none* (hopeless). Grounded
remains the sole basis for `status`, `decide`, and `check`.

**Why.** Grounded is the right referee (unique, skeptical, deterministic), but a
poor explorer: any genuinely contested issue collapses to "everything UNDEC" —
nearly information-free. The same framework supports a second, standard query:
Dung's preferred extensions, which package the UNDEC residue into its coherent
maximal stances. For a wicked problem, "there are exactly three self-consistent
positions a reasonable person can hold here, and they differ on these two
arguments" is *far* more generative than a flat UNDEC list: it shows the shape
of the disagreement rather than the fact of it.

**Utility.**

- Instantly answers "what are the live options, as *packages*?" — the question
  stakeholder deliberation actually revolves around.
- Credulous/skeptical acceptance ("IN in some world" / "IN in every world")
  gives agents a principled vocabulary between "justified" and "contested" —
  e.g. an argument that appears in every preferred extension is safe to build
  on even while its issue is formally undecided.
- Theoretically clean: no new semantics invented, no change to the referee —
  the exploration lens and the adjudication lens are the same framework asked
  two different classical questions.
- Tractable in practice: preferred extensions differ from grounded only on the
  UNDEC residue, which in real dlktk discussions is small; enumeration over
  that residue is cheap (with an explicit guard for pathological graphs).

## 6. Values and audiences — sensitivity analysis over value rankings

**What.** Lift preferences from opaque pairwise edges toward Bench-Capon's
value-based argumentation:

- Nodes may **promote a value** (`--promotes throughput` on
  propose/object/support, or `promote <node> <value>` after the fact).
- An **audience** is a named strict ranking over values:
  `dlktk audience ops security velocity` ("for ops, security ≻ velocity").
- `status --under <audience>` computes the labelling with attacks neutralized
  by the audience's value ranking (an attack fails when the target promotes a
  value the audience ranks strictly above the attacker's).
- `dlktk audiences` reports, per issue, which positions are **robust** (IN
  under every declared audience and under no-audience) and which are
  **audience-sensitive** (the verdict depends on whose values win).

**Why.** Rittel again: solutions to wicked problems are not true or false but
better or worse, *and there is no ultimate test* — stakeholders judge against
different value orderings, and that disagreement is the deepest fact about the
problem. Today that fact is squeezed through a single free-text pairwise
`prefer`, which means whoever states a preference first wins the tie and the
value conflict itself is never represented, never computed over, never
surfaced. The result is false closure — the exact failure mode wicked-problem
theory warns about.

**Utility.**

- Turns "we disagree" from a stalemate to be bulldozed into a **computed,
  explainable map of the value conflict**: which conclusions survive *any*
  reasonable value ordering (act on those now), and which hinge on whose values
  govern (that's the real decision, name it and argue it).
- Robustness across audiences is the closest thing a wicked problem has to
  objective justification — a genuinely novel capability no adjacent tool has.
- The value disagreement becomes arguable *inside* the tool: an audience is a
  recorded, attributed artifact, so "should ops's ranking govern this system?"
  can itself be raised as an issue.
- Pairwise `prefer` remains for one-off tiebreaks; audiences don't replace it,
  they make the systematic case honest.

## 7. `synthesize` — hybrid positions with recombination lineage

**What.** A move `synthesize <issue> "<text>" --from <posA> --from <posB> [--from …]`
creating a new position that records `synthesizes` lineage to two or more
existing positions on the same issue. Evaluation treats it as an ordinary
position; the lineage is metadata for `tree`/`show`/postmortem, and `moves`
suggests synthesis when a stalemate persists.

**Why.** Novel solutions to wicked problems are very often *recombinations* —
"A's caching layer with B's invalidation strategy." Today such a hybrid is just
a third rival with no memory of its parents: the tool can neither suggest
recombination at the moment it is most valuable (a stalemate between partial
answers), nor answer the retrospective question practitioners actually ask —
"which parts of the losing position survived into what we shipped?"

**Utility.**

- Gives the stalemate advice of #2 a concrete generative move to point at,
  with recorded provenance instead of an unexplained third position.
- Solution genealogy: `tree`/`show` reveal that the decided position descends
  from both rivals — which is *the* common happy ending of a wicked-problem
  deliberation and currently invisible.
- Costs almost nothing semantically: no evaluator change; one link relation.

## 8. Assumptions as challengeable, tracked premises

**What.** A move `assume <target> "<text>"` recording an argument node tagged
as an **assumption** (supports its target). Assumptions are ordinary AF nodes —
attackable, defeatable — plus bookkeeping: `agenda` lists **undischarged**
assumptions (never examined: no support, no objection), and `check --strict`
warns when a standing decision's position rests on an assumption that is
currently OUT.

**Why.** Wicked problems hinge on unstated premises — about the workload, the
users, the constraints — and the biggest reframings come from someone finally
saying "wait, why do we believe that?" Today a premise can only be smuggled in
as an ordinary `support` argument, which the evaluator ignores and no surface
tracks; there is no way to see what a position *rests on*, and no pressure to
examine it. Naming assumptions is the cheapest ASPIC-flavored idea worth
stealing — premise-level challenge — without importing the rule calculus
(consistent with design §3.4's "resist the machinery" call).

**Utility.**

- The agenda's `assumptions` section gives a First-Principles persona (see #9)
  a concrete worklist, and gives humans a one-glance answer to "what are we
  taking on faith here?"
- The decision-rests-on-defeated-assumption warning catches the quiet failure
  where a rebuttal demolished a premise but nobody re-visited the conclusion
  that stood on it — supports stay inert in the labelling (§3.5 holds), yet
  finally earn epistemic weight in the bookkeeping.
- Encourages a healthy division of labor: positions state answers, assumptions
  state the world-model those answers need. Attacking the world-model is where
  reframes come from.

## 9. Encode diverge-then-converge in the harness playbooks

**What.** Documentation changes to `AGENTS.md` and
`skills/dlktk-dialectic/SKILL.md` (and `examples/deliberate.sh`):

- An explicit **divergence phase with quotas**: no `prefer` until the issue has
  ≥ 2 positions from ≥ 2 authors; each persona contributes at least one
  *generative* move (propose / reframe / assume-challenge) before any
  preference is stated.
- A rotating **devil's-advocate obligation**: each round, some persona must
  object to the currently-untested IN position (which #1 makes discoverable).
- New personas alongside Maintainer/Shipper/Security/Historian: **Reframer**
  (challenges issue formulations; uses `reframe`), **Analogist** (imports
  candidate positions from prior discussions via `search --all` / `anchored` —
  giving the Historian's tooling a *generative* role, not just a policing one),
  **First-Principles** (surfaces and attacks assumptions), and per-value
  **stakeholder advocates** (each argues one value and declares its audience,
  feeding #6).

**Why.** The playbooks are where agent behavior is actually shaped — the skill
explicitly tells agents to stay inside the documented loop, and the current
loop is purely convergent (agenda → moves → act → decide). Every structured
ideation method (and the wicked-problems literature itself) insists on
separating divergence from convergence, because evaluation pressure kills
generation. This is zero code and probably the highest behavior-per-effort
change on the list.

**Utility.**

- Quotas are mechanically checkable by the harness (count positions/authors
  before allowing `prefer`), so the discipline is enforceable, not aspirational.
- The persona additions convert the features above (#1 untested surfacing, #3
  reframe, #6 audiences, #8 assumptions) from *available* to *actually
  exercised* — features agents aren't scripted to use don't get used.

## 10. Decisions with review horizons

**What.** `decide`/`supersede` accept `--review-by <T>`. `check` reports a
finding when a standing decision's review horizon has passed; extending or
re-affirming goes through `supersede` (same position, new basis and horizon),
so the re-examination is itself recorded.

**Why.** "Wicked problems have no stopping rule" is in structural tension with
`decide` as a closing act. The design resolves the tension correctly — closure
must be recordable — but the honest version of closing a wicked question is
*provisional* closure: "we choose A, and we commit to re-examining this when
the load doubles / in Q3 / when the migration lands." Today that provisionality
lives in a comment nobody re-reads. The bitemporal store makes the mechanism
nearly free.

**Utility.**

- Converts "we'll revisit this someday" from a hope into a CI finding — the
  same living-constraint lever that makes `check` the strongest adoption story,
  extended from *logical* drift (the argument moved) to *temporal* drift (the
  world moved).
- Encourages deciding at all: teams facing a wicked question often defer
  deciding *because* closure feels dishonest. A decision with a recorded
  horizon is easier to make and healthier to live with.

---

## How the ten fit together

Three reinforcing loops, worth seeing whole:

- **The novelty loop** (#1 → #2 → #7/#3): untested winners get challenged;
  challenges produce stalemates; stalemates prompt synthesis or reframing
  instead of a premature preference.
- **The leverage loop** (#4 → #5 → #6): `crux` finds what the verdict rests on;
  `worlds` shows the coherent alternatives; `audiences` shows which
  disagreements are value-driven — together they aim creative effort where it
  changes the outcome.
- **The honesty loop** (#8 → #10 → `check`): assumptions and review horizons
  make the *provisionality* of every conclusion explicit and machine-checkable,
  so closure never silently outlives its justification.

\#9 is the connective tissue: playbooks that make agents actually run the loops.
