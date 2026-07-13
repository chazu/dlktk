---
name: dlktk-dialectic
description: "Use when a design or implementation question has rival answers and you want the decision argued out and recorded, not just asserted — especially with multiple personas (a swarm or a human + agents). Conduct a defeasible argument with dlktk: capture issues/positions/arguments, let grounded semantics compute what stands, attribute each move to an author under a role. Triggers: \"argue this out\", \"run a dialectic\", \"deliberate on X\", \"weigh these options and record why\", \"have the agents debate this\"."
---

# Conducting a dialectic with dlktk

`dlktk` records a design discussion as an IBIS argument graph and computes which
positions currently *stand* via Dung grounded semantics. You never assign a
verdict yourself: you state issues, positions, arguments, and preferences; the
engine derives the labels. This skill is how to drive it — solo or as several
personas — and how to use identity/roster so a multi-agent debate is honest.

## When to use

- A question has rival answers and you want the *reasoning* captured, not just
  the conclusion ("which lock?", "REST vs gRPC?", "should we cache here?").
- Multiple personas should argue (a swarm, or a human + agents) and you want
  every move attributed and the decision auditable later.
- You want a decision that CI can re-check for drift, not a comment that rots.

If you just need a fact lookup or a quick opinion, this is overkill.

## Mental model (read once)

- Three node kinds: **issue** (a question), **position** (a candidate answer),
  **argument** (a claim bearing on a position *or another argument*).
- Links: `responds_to` (position→issue), `objects_to` (an **attack**, feeds the
  labelling), `supports` (rationale only — **the engine ignores it**),
  `synthesizes` (hybrid→parent lineage), `addresses` (an answer→the parent
  objection it discharges). The last three never reach the evaluator.
- Every position/argument gets a label: **IN** (justified), **OUT** (defeated),
  **UNDEC** (genuinely contested — the live agenda).
- Objecting to an objection **reinstates** its target. That recursion is the
  whole point; a flat pro/con list can't express it.
- A tie between positions is broken with a **preference**: `prefer A B --basis
  <label>`. Defeat = an attack that survives preference.

## Setup

**Version handshake (mandatory first move).** This skill assumes contract
version **0.17.0 or later** (`MIN_CONTRACT` below). Run `dlktk discover` first
and read its `version`:

```
dlktk discover --format json     # the full contract: moves, reads, flags, exit codes
```

- If `version` ≥ `0.17.0`, proceed.
- If `version` < `0.17.0`, **stop and warn the user visibly**: the installed
  `dlktk` predates the playbook this skill teaches (rival-aware tested-ness,
  synthesis discipline, value-map closure, the warnings-are-work-items rule).
  Continuing would run a stale playbook against a newer engine or vice-versa —
  the exact failure the handshake exists to catch. Ask them to rebuild/upgrade
  `dlktk` before continuing.

The agent that loads this skill is the checker — this works on machines a
`make` never touched. `MIN_CONTRACT = 0.17.0`.

```
dlktk new "<title>" --subject "q:<the question>"   # creates + selects a discussion
```

Output auto-detects: text on a terminal, JSON when piped. Pass `--format json`
when you parse output. Errors are `{error, detail, node?}` on stderr with stable
exit codes: `2` illegal move, `3` not found, `4` store, `5` check failed.

## Identity and roles (do this for multi-persona debates)

Two distinct things — keep them separate:

- `--author <id>` — the **stable identity** a move is attributed to, and the
  thing ownership is checked against. You can only `concede`/`retract` nodes
  *your author* created. Defaults to the OS user.
- `--role <persona>` — the **hat** the move is argued under (Maintainer,
  Shipper, Security, Historian…). Optional. The first move an author makes under
  a role **auto-records** an author↔role binding; `dlktk roster` lists them.

Two agents can share the `Security` persona and still own only their own nodes —
because ownership rides on `--author`, never the spoofable persona. For a swarm,
give each agent a distinct `--author` (e.g. `security-bot`) and a `--role`.

```
dlktk object <pos> "writer starvation under load" --author security-bot --role Security
dlktk roster                       # who argued under which persona
dlktk roster carol Maintainer      # optional: pre-declare a binding
```

Suggested personas: **Maintainer** (objects on long-term cost, prefers on
`maintainability`), **Shipper** (proposes, prefers on `velocity`), **Security**
(objects on threat, prefers on `security`), **Historian** (queries prior
decisions with `log`/`replay`/`anchored`/`search --all` and raises an issue when
a new position contradicts a standing one). For open-ended questions add the
generative personas: **Reframer** (challenges framings with `reframe` and
`raise --from`), **Analogist** (mines prior discussions for candidate
positions, cited by value), **First-Principles** (states premises with
`assume`, then attacks them), per-value **stakeholder advocates** (each
declares an `audience` ranking and tags moves with `--promotes`), and the
**Adopter** (argues from what a real team will sustain; when a synthesis
retains ≥2 parent mechanisms it must object with a cost model — who does the
added work and what they stop doing — under an author distinct from the
synthesis author).

## The loop

1. **`agenda`** — the worklist, in five sections:
   - `undecided` (UNDEC): contested — needs an argument or a preference.
   - `ready`: settled on one justified position — `decide` it (or surface to a
     human if deciding isn't your call).
   - `unpopulated`: issues with no positions — `propose` one.
   - `untested`: issues about to close on a winner no substantive objection
     ever engaged (rival edges, self-objections, and preference-excused
     attacks don't count). **IN by silence is unexamined, not vindicated** —
     stress-test before deciding. Only decide-adjacent winners appear here;
     `status` marks the rest per position.
   - `assumptions`: premises (`assume`) nobody has examined — support or
     object; the biggest reframes start here.
2. **`moves <issue>`** — the legal, useful next moves, each with its effect. In
   text mode these print as runnable `dlktk …` command lines; over JSON they
   arrive as `{move, args, effect}`. Pick one; don't invent moves outside it.
3. **Act** — `raise` / `reframe` / `propose` / `synthesize` / `support` /
   `object` / `assume` / `prefer` / `promote` / `audience` / `decide`.
4. **Re-read** — labels may have flipped *anywhere* (reinstatement). Repeat
   until the agenda is empty or your budget is spent.

To understand a label before contesting it: `why <node>` (its attackers + the
moves that flip it), `show <node>` (the node in full), `explain <issue>` (the
whole derivation, round by round).

To explore before committing (all read-only, nothing written): `whatif <issue>
--object <t> / --prefer <w>:<l> / --without <n>` (label diff of hypothetical
moves), `crux <issue>` (the load-bearing arguments — attack the crux or shore
it up), `worlds <issue>` (the coherent stances a contested issue admits, with
positions sorted robust/contingent/hopeless), `audiences` (which positions
survive every declared value ranking).

## Diverge before you converge (for open-ended / wicked questions)

- **No `prefer` until the issue has ≥ 2 positions from ≥ 2 authors.**
- **Each persona makes at least one generative move** (propose / synthesize /
  reframe / assume / raise --from) before stating any preference.
- **Rotate a devil's advocate** against the strongest untested IN position
  (the agenda's `untested` section names the decide-adjacent one; `status`
  marks the rest). The objection must come from a *different author* than the
  position — a self-objection does not count as a test, and preferring the
  position over its objection un-tests it.
- **Stress-test a synthesis from outside it.** Default procedure: run the
  devil's-advocate turn as a **separate agent process whose only context is the
  exported graph** (`dlktk export`), with three distinct `--author` identities
  for the synthesis author, the advocate, and the decider. A roster of distinct
  names proves *attribution, not independence* — one mind can type every
  `--author`. `check --strict` fires `single_author_convergence` when a decided
  synthesis's decider shares its author or every objection against it does,
  regardless of how many names the session wore — so make the isolation real.
- **Every wicked problem is a symptom of another problem**: when an argument
  reveals a deeper question, `raise "<question>" --from <that node>` so the
  provenance is recorded and the sub-issue nests under it in `tree`.

## Compose or choose: pick the cardinality when you raise (§item 6)

Cardinality is fixed at `raise` time. **If the candidate answers could compose**
— practices, process tweaks, layered mitigations — raise it `--card open`: an
open issue records a **standing decision per position** (`decide <issue> <A>`
*and* `decide <issue> <B>` both stand — the winners are adopted together),
`agenda`'s `ready` lists each undecided-but-justified position, and `supersede`
revises one position's decision while its siblings stand. **Use `select_one`
only when the answers genuinely exclude one another.** A `select_one` contest
between *partial* answers tends to end in a synthesis — if you are synthesizing
because complementary answers were seated as rivals, the issue wanted
`--card open`; false rivalry manufactures a bundle at close time.

*Example:* "which practices raise comprehension?" (pairing, spaced retrieval,
code review — all adoptable) → `raise … --card open`, propose all three,
stress-test each, `decide` each in turn: three standing decisions, no artificial
contest.

## Move discipline (the rules that keep a debate honest)

- **Search before you argue.** `search "<phrase>"` — if the claim already exists,
  `object`/`support` the existing node instead of restating it. Duplicate
  arguments don't change labels; they just bloat the graph. This is *the*
  predictable multi-agent failure mode — avoid it.
- **To change the verdict, `object` — never `support`.** `supports` is recorded
  rationale the evaluator ignores. Defend a position by defeating its attacker
  (object to the objection); reinstatement does the rest.
- **Stalemates have three exits — try them in this order.**
  1. `synthesize <issue> "<hybrid>" --from <p1> --from <p2> --drops "<what it
     excludes>"` — recombine the rivals with recorded lineage. Caveat: the
     hybrid *joins* the rivalry until the parents are conceded or a
     preference/audience elevates it. **A synthesis that drops nothing is a
     bundle** (the move warns at ≥3 parents with no `--drops`; deciding it
     as-is draws `bundle_synthesis`). The hybrid **inherits its parents'
     undefeated objections as open questions** (`show`/`why`/`moves` list
     them): re-aim one with `object <hybrid> "…" --answers <objection-id>` or
     dismiss it with `support <hybrid> "…" --answers <objection-id>` — a
     hybrid must not enter the arena cleaner than its content. If the rivals
     only compete because of a `select_one` framing and actually compose, the
     deadlock is a cardinality mistake — the issue wanted `--card open`.
  2. `reframe <issue> "<new framing>" --basis <label>` — when the deadlock is
     a false dichotomy. Positions don't carry over; the dead framing leaves
     the agenda; lineage is recorded.
  3. `prefer <winner> <loser> --basis <label>` — an honest value call, when
     the options really are exhaustive. Burying a parent under its own hybrid
     while its inherited questions are open warns on the move and draws
     `self_elevated_synthesis` — address the questions first (≥1 address by
     another author), then prefer.
  A new argument helps only if it attacks from *outside* the cycle.
- **Name the values, then the audiences.** Tag contributions with `--promotes
  <value>` (or `promote <node> <value>` on your own nodes); record stakeholder
  rankings with `audience <name> <v1> <v2>…`; read the conflict with
  `audiences` and `status --under <name>`. Robust-across-audiences is the
  strongest justification a wicked question admits.
- **If nothing is robust, the map is the deliverable.** When the real decision
  is *whose values govern*, close honestly without a winner:
  `decide <issue> --map --basis <label> --review-by <T>` records the audience-
  conditional map as the outcome. Legal only when the issue is audience-
  sensitive right now (≥2 audiences, ≥1 position whose verdict differs);
  `--review-by` is mandatory; `check` reports `map_drift` if the map later
  changes (verdicts are never frozen). Then `raise "whose ranking governs?"
  --from <a position>` — until you do, `check` carries the non-fatal note
  `mapped_pending_governance`. Convert to a winner or re-affirm via `supersede`
  (`--map` to stay a map); the superseded kind is recorded.
- **Decisions are closing acts — provisionally.** On a `select_one` issue,
  `decide <issue> <position>` is rejected once the issue is decided; overturning
  goes through `supersede <issue> <position> --basis <label>`, which records
  *why* and links the prior decision. On an `open` issue, `decide` records one
  standing decision **per position** (a repeat is rejected only on the same
  position) and `supersede` revises a single position's decision. Deciding
  against the justified position is allowed but flagged as an override — say so.
  Record known provisionality with `--review-by <T>`; `check` flags the decision
  when the horizon passes.
- **Withdraw only your own mistakes** with `concede <node>` (checked against
  `--author`).

## A multi-persona deliberation (the shape)

Each persona, in turn: read `agenda` + `moves`, make **at most one** move under
its `--author`/`--role`, then pass. The engine referees after every move.

```
DISC=$(dlktk new "cache lock choice" --subject "q:lock for the read cache" --format json | jq -r .id)
ISSUE=$(dlktk raise "which lock?" -d $DISC --author shipper-bot --role Shipper --format json | jq -r .id)
A=$(dlktk propose $ISSUE "plain mutex" -d $DISC --author shipper-bot --role Shipper --format json | jq -r .id)
B=$(dlktk propose $ISSUE "RWLock"     -d $DISC --author maint-bot   --role Maintainer --format json | jq -r .id)
C=$(dlktk object $B "writer starvation" -d $DISC --author sec-bot   --role Security --format json | jq -r .id)
dlktk object $C "cache is 99% reads; starvation can't occur" -d $DISC --author maint-bot --role Maintainer  # reinstates B
dlktk status $ISSUE -d $DISC          # still a tie? -> a persona states a preference
dlktk prefer $B $A --basis throughput -d $DISC --author shipper-bot --role Shipper
dlktk status $ISSUE -d $DISC          # B now justified
dlktk decide $ISSUE $B --basis throughput -d $DISC --author shipper-bot --role Shipper
dlktk roster -d $DISC                 # the audit of who argued as what
```

**Stop when** the agenda is empty; or a stalemate persists after every persona
has had a chance to break it (escalate to a human — don't loop adding
objections); or a move budget runs out.

## Afterwards

- `dlktk export -d $DISC > design/dialectics/<name>.ndjson` and commit it — the
  dialectic reviews like code, beside what it constrains.
- `dlktk check [--all] [--strict]` in CI: exit `5` means a recorded decision has
  drifted (its position is no longer justified) — re-argue it or `supersede` it.
  This is what turns a decision from archaeology into a living constraint.
  `--strict` also fails on warnings: stalemates, decisions never substantively
  attacked (`untested_decision` — rival edges, self-objections, and
  preference-excused attacks don't count), expired review horizons
  (`review_due`), decisions resting on defeated assumptions
  (`defeated_assumption`), a hybrid preferred over a parent whose objections
  it never answered (`self_elevated_synthesis`), a decided ≥3-parent synthesis
  with no recorded drops (`bundle_synthesis`), a mapped issue whose audience map
  has drifted (`map_drift`), a decided synthesis scrutinised or decided only
  within its own author (`single_author_convergence`), a preference made before
  two authors staked positions (`premature_preference`), and an issue decided
  over a warning its basis never acknowledged (`unacknowledged_warning`). The
  note `mapped_pending_governance` (never fatal) flags a mapped issue whose
  governance question is still unraised.
- **A warning is a work item.** Before any convergent move (`prefer`/`decide`/
  `supersede`), clear the warnings on that issue — or, if you proceed anyway,
  name them in `--basis` (the finding kind, or an override rationale). Closing
  over an unacknowledged warning draws `unacknowledged_warning`.

## Postmortem

- `dlktk replay <issue> --as-of <T> [--diff]` — the labelling as it stood at T,
  and what changed since ("is this decision still load-bearing?").
- `dlktk log [<node>]` — the transaction-time audit trail.
- `dlktk --valid-at <T> ...` — which decisions were in force at T.
