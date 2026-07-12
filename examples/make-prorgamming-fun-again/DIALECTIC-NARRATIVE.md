# The dialectic, in prose

*A plain account of discussion `sulog-galak` — "make programming fun again" —
organized by participant and by round. Every position, objection, and ruling
below corresponds to a node in [`dialectic.ndjson`](dialectic.ndjson). The
engine's grounded-semantics labelling is reported as "the engine ruled";
participants never assigned verdicts themselves.*

## Participants

Seven personae took part, each a distinct author identity:

- **Maintainer** ⟨maint-bot⟩ — argues from long-term cost and code health.
- **Shipper** ⟨ship-bot⟩ — argues from velocity and economic viability.
- **Advocate of Mastery** ⟨craft-bot⟩ — argues from hands-on skill retention.
- **Advocate of Learning** ⟨learn-bot⟩ — argues from knowledge acquisition.
- **First-Principles** ⟨firstp-bot⟩ — names hidden premises and attacks them.
- **Analogist** ⟨analogist-bot⟩ — imports evidence from historical practice.
- **Reframer** ⟨reframer-bot⟩ — challenges question formulations; raises
  derived questions.

## The question

The Reframer raised the root issue: *how should the process of agentic coding
be changed so that the human programmer retains skill, code familiarity, and
domain knowledge, and the software avoids accelerated aging?* The issue was
created with select_one cardinality, which makes all positions on it mutual
rivals. This shaped the rest of the session.

## Round 1: proposals

Six positions were proposed, one per persona, each tagged with the value it
promotes:

1. **Maintainer — comprehension gates** (longevity). The merge workflow
   requires the human to explain each change-set in their own words — what it
   does, why, how it fits the design — before it lands. Unexplained diffs do
   not merge.
2. **Advocate of Mastery — inverted division of labor** (mastery). The human
   writes the core domain logic and interfaces by hand; agents are restricted
   to the periphery: tests, boilerplate, migrations, refactors under
   human-authored contracts.
3. **Advocate of Learning — pedagogical loop** (mastery). The agent is also a
   tutor: every session emits learning artifacts (walkthroughs, quizzes,
   spaced-repetition cards) generated from the actual diffs, and the human's
   retention is tracked like test coverage.
4. **First-Principles — theory ownership** (longevity). Following Naur: the
   human owns a living conceptual core — domain model, ADRs, invariants —
   that agents must conform to; design-shifting changes require the human to
   update the theory first. Code is generated, theory is hand-made.
5. **Analogist — rotating driver/navigator** (mastery). Import the
   pair-programming protocol: the human types on a scheduled fraction of the
   work with the agent navigating, and they swap. Hands-on contact is
   scheduled rather than left to chance.
6. **Shipper — change nothing about generation** (velocity). Invest instead
   in on-demand comprehension — code maps, generated architecture tours,
   interactive walkthroughs — so familiarity is cheap to rebuild when needed
   instead of maintained continuously.

The engine ruled a six-way mutual stalemate, all positions UNDEC, and advised
that a preference would resolve it, a synthesis might transcend it, and a
reframe was worth considering if the rivalry was false. No preference was
stated at this point, per the divergence-phase quota.

## Round 2: assumptions

Before any objections, First-Principles recorded four positions' load-bearing
premises as challengeable assumptions:

- On the pedagogical loop: retention artifacts transfer into working skill —
  recall of facts about a diff equals ability to modify the system.
- On on-demand comprehension: familiarity rebuilt on demand from generated
  tours is as good as familiarity maintained by continuous contact.
- On theory ownership (his own position): a human can hold a program's theory
  in Naur's sense without writing most of its implementation.
- On the inverted division: teams can absorb the velocity loss of
  hand-writing the core and survive competition that doesn't.

All four appeared in the agenda's undischarged-assumptions section.

## Round 3: objections

Each persona attacked a position not its own:

- **Shipper → gates:** under deadline pressure, explanation gates degrade
  into rubber-stamping — the human pastes a paraphrase of the agent's own
  summary; the gate measures prose production, not comprehension.
- **Shipper → inverted division:** restricting agents to the periphery
  forfeits most of the productivity gain and is economically unstable;
  competitors on full agentic flow will outship you, so the discipline erodes
  when it matters.
- **Maintainer → change-nothing:** on-demand comprehension concedes exactly
  what Parnas calls aging — by the time you need to rebuild familiarity, the
  design integrity you would comprehend has already eroded; a guided tour of
  a degraded structure does not restore it.
- **Advocate of Mastery → pedagogical loop:** learning artifacts decoupled
  from production stakes are homework; quizzes get skipped like
  documentation, and skills built on recall do not survive real debugging.
- **First-Principles → driver/navigator:** scheduled driving on arbitrary
  work practices typing, not design; hands-on contact without ownership of
  the problem builds no theory of the program.
- **Analogist → theory ownership:** architecture-only ownership is the
  ivory-tower architect pattern, which reliably decays — the theory diverges
  from code it never touches; Naur's point was that theory is built *by*
  implementing, not beside it.

## Round 4: defenses and a derived question

Two positions were defended by attacking their attackers (the only defense
the labelling counts):

- **Maintainer** answered the rubber-stamping objection: the gate need not be
  paraphrase — require a falsifiable prediction (what will this change break,
  what test would catch it) checked against the CI outcome; predictions
  cannot be rubber-stamped from the agent's summary.
- **Advocate of Learning** answered the homework objection: decoupling is a
  design choice, not a fate — artifacts generated from your own repo's diffs
  and reviewed in the merge flow are situated practice with production
  stakes.

Two further moves in this round:

- **Advocate of Mastery** attacked the on-demand assumption directly: passive
  tours produce recognition, not recall; the assumption fails for
  modification tasks. The assumption was thereby discharged (defeated).
- **Reframer** raised a new issue *from* the Analogist's ivory-tower
  objection, with provenance recorded: *if theory is built by implementing,
  and agents now do most implementing, where must a program's theory live so
  that it survives agent-scale code churn?*

The engine then ruled on the root issue: inverted division, theory ownership,
driver/navigator, and change-nothing all OUT, each defeated by an unanswered
objection. Gates and the pedagogical loop, both reinstated, remained UNDEC.

## Interlude: read-only analysis

Three read-only queries were run before continuing:

- `crux` reported that the two defense arguments (falsifiable prediction,
  situated practice) were the pivotal nodes: removing either flips the
  contest.
- `worlds` reported exactly two coherent worldviews — one accepting the
  gates, one accepting the pedagogical loop — and that the ivory-tower
  argument was IN in both. "Theory is built by implementing" was therefore
  common ground across all surviving stances.
- `whatif` confirmed that defeating the ivory-tower objection would restore
  theory ownership to UNDEC, making a three-way contest. No one acted on
  this; the query left no trace in the record.

## The sub-issue: where must the theory live?

Three positions were proposed:

1. **First-Principles:** in the human's head, maintained by hand-implementing
   the load-bearing core modules on a recurring cadence. (While this stood
   alone it appeared in the agenda's *untested* section — IN only because
   unattacked.)
2. **Advocate of Learning:** in executable artifacts the human authors and
   the agent must satisfy — domain types, contracts, property tests,
   invariants; theory as enforced specification, exercised by CI.
3. **Analogist:** in a co-maintained design record — decision graphs, ADRs,
   argued rationale — read and written by both human and agent, checked in CI
   for drift.

Each was then defeated by one objection:

- **Maintainer → head:** heads do not scale or persist; turnover and
  agent-scale churn outpace any one human's memory — the original problem
  restated as its own solution.
- **First-Principles → executable artifacts:** types, contracts, and tests
  encode the *what*; Naur's theory is precisely the *why* and the
  not-yet-formalized — the residue that resists specification is where aging
  starts.
- **Advocate of Mastery → design record:** a record nobody executes is the
  ivory tower in file form; prose ADRs rot like comments, and drift checks on
  text are weak.

The engine ruled all three OUT. The two fatal objections were complementary —
the executable spec lacked the why, the record lacked enforcement — and the
**Advocate of Learning** synthesized the two positions, lineage recorded:
*the theory lives in an executable specification bound to its decision
record. Every human-authored invariant, type, or property test links to the
argued rationale that justifies it, and CI checks both — code against spec,
spec against standing decisions.*

The engine ruled the synthesis IN, but it had not yet been attacked. Per the
devil's-advocate obligation, the **Shipper** objected: binding every
invariant to a rationale is ceremony; under deadline pressure the links get
stubbed or go stale — the same failure mode as prose ADRs. The **Maintainer**
defeated the objection: staleness here is a build break, not a hope — an
invariant whose linked decision is no longer standing fails CI, so the link
cannot silently rot.

With the synthesis stress-tested and reinstated, **First-Principles** decided
the sub-issue for it, with a review horizon of 2027-01-11.

## The root issue: synthesis

Returning to the root question, the **Reframer** synthesized a position from
four parents — theory ownership, gates, the pedagogical loop, and
driver/navigator — arguing they were components of one process rather than
rivals:

> **The theory-anchored agentic loop.** (1) The human owns an executable
> conceptual core bound to its decision record (per the sub-issue decision)
> that agents must satisfy. (2) The merge gate demands a falsifiable human
> prediction checked against CI, not a paraphrase. (3) The agent emits
> situated learning artifacts from real diffs against that core. (4) The
> human hand-implements scheduled katas on load-bearing modules so the theory
> stays embodied. Generation stays unrestricted everywhere else.

Each parent survives as the part of it that had withstood attack: theory
ownership scoped to an executable core, gates as prediction discipline, the
tutor loop as the artifact stream, rotation as katas confined to the core.

The engine ruled a three-way contest: gates, pedagogical loop, and the
hybrid, all UNDEC.

## Values and audiences

Rather than break the tie with a bare preference, the value advocates
declared audiences — named rankings over the values in play:

- **craft** (Advocate of Mastery): mastery ≻ longevity ≻ velocity
- **enterprise** (Maintainer): longevity ≻ mastery ≻ velocity
- **startup** (Shipper): velocity ≻ mastery ≻ longevity

The `audiences` report showed that **no position was robust across all
three rankings**. Under craft and enterprise values the three-way contest
held. Under the startup ranking, every position went OUT — including the
already-decided sub-issue synthesis — except the Shipper's change-nothing
position, which flipped to IN. The disagreement was therefore value-driven,
not argument-driven.

Following the playbook's instruction to name such a conflict, the
**Reframer** raised a further issue (from the change-nothing position):
*whose value ranking should govern the agentic-coding process, and is
velocity-first a coherent audience for this question?* **First-Principles**
proposed the answer that was adopted: the problem statement takes skill decay
and software aging as harms to be avoided, so it presupposes that mastery and
longevity matter; a velocity-first ranking answers a different question —
velocity constrains the *how* (keep generation unrestricted off the core),
not the *whether*. The Reframer decided the issue on scope grounds.

Note: this position was decided without ever being attacked. `check --strict`
subsequently flagged it (`untested_decision` — "its IN label is unexamined,
not vindicated"), the session's one audit warning.

## Closure

The **Maintainer** and the **Advocate of Learning** each stated a preference
elevating the hybrid over the position of theirs it absorbed — hybrid over
gates, hybrid over pedagogical loop — with basis *subsumption* in both cases:
the hybrid contained their positions rather than defeating them.

The engine ruled the theory-anchored loop the sole justified position. The
**Maintainer** decided the root issue for it, recording in the basis that the
verdict is audience-sensitive (it fails under velocity-first values) and
setting a review horizon of 2026-12-15, by which time evidence about the cost
of the katas should exist.

Final state: three issues, all decided provisionally; two syntheses with
recorded lineage; one derived question with recorded provenance; one audit
warning. Any intermediate state can be reconstructed with
`dlktk --store .pudl -d sulog-galak replay sulug-butuk --as-of <T>`.
