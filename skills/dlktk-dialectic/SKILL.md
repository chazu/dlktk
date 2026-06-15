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
  labelling), `supports` (rationale only — **the engine ignores it**).
- Every position/argument gets a label: **IN** (justified), **OUT** (defeated),
  **UNDEC** (genuinely contested — the live agenda).
- Objecting to an objection **reinstates** its target. That recursion is the
  whole point; a flat pro/con list can't express it.
- A tie between positions is broken with a **preference**: `prefer A B --basis
  <label>`. Defeat = an attack that survives preference.

## Setup

```
dlktk discover --format json     # the full contract: moves, reads, flags, exit codes
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
a new position contradicts a standing one).

## The loop

1. **`agenda`** — the worklist, in three sections:
   - `undecided` (UNDEC): contested — needs an argument or a preference.
   - `ready`: settled on one justified position — `decide` it (or surface to a
     human if deciding isn't your call).
   - `unpopulated`: issues with no positions — `propose` one.
2. **`moves <issue>`** — the legal, useful next moves, each with its effect. In
   text mode these print as runnable `dlktk …` command lines; over JSON they
   arrive as `{move, args, effect}`. Pick one; don't invent moves outside it.
3. **Act** — `raise` / `propose` / `support` / `object` / `prefer` / `decide`.
4. **Re-read** — labels may have flipped *anywhere* (reinstatement). Repeat
   until the agenda is empty or your budget is spent.

To understand a label before contesting it: `why <node>` (its attackers + the
moves that flip it), `show <node>` (the node in full), `explain <issue>` (the
whole derivation, round by round).

## Move discipline (the rules that keep a debate honest)

- **Search before you argue.** `search "<phrase>"` — if the claim already exists,
  `object`/`support` the existing node instead of restating it. Duplicate
  arguments don't change labels; they just bloat the graph. This is *the*
  predictable multi-agent failure mode — avoid it.
- **To change the verdict, `object` — never `support`.** `supports` is recorded
  rationale the evaluator ignores. Defend a position by defeating its attacker
  (object to the objection); reinstatement does the rest.
- **Stalemates need a preference, not more arguments.** When `status` reports a
  stalemate, every position is UNDEC and symmetric; a new argument helps only if
  it attacks from *outside* the cycle. State `prefer <winner> <loser> --basis
  <label>` with an honest basis.
- **Decisions are closing acts.** `decide <issue> <position>` is rejected on an
  already-decided issue; overturning goes through `supersede <issue> <position>
  --basis <label>`, which records *why* and links the prior decision. Deciding
  against the justified position is allowed but flagged as an override — say so.
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

## Postmortem

- `dlktk replay <issue> --as-of <T> [--diff]` — the labelling as it stood at T,
  and what changed since ("is this decision still load-bearing?").
- `dlktk log [<node>]` — the transaction-time audit trail.
- `dlktk --valid-at <T> ...` — which decisions were in force at T.
