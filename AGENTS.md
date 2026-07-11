# Driving dlktk as an agent

dlktk records design discussions as a typed argument graph and computes which
positions currently stand. You never label anything yourself: you state issues,
positions, arguments, and preferences; the grounded labelling (IN / OUT /
UNDEC) is derived. This file is the playbook for driving it — drop it (or its
gist) into any repo that uses dlktk.

## Learn the tool cold

```
dlktk discover --format json
```

returns the full capability contract: every move with its legality
precondition, every read with its JSON envelope, the global flags, and the
error/exit-code catalog. Output auto-detects: text on a terminal, JSON when
piped — so a harness reading stdout gets JSON for free. Pass `--format json`
explicitly anyway for predictability; parse stderr as `{error, detail, node?}`
on nonzero exit (in a terminal, errors also print a one-line `hint:` for humans).

| exit | meaning |
|------|---------|
| 0 | success |
| 2 | illegal move (nothing written) |
| 3 | referenced id does not exist |
| 4 | store/engine failure |
| 5 | `check` found drift or invariant violations |

Over MCP, run `dlktk mcp` — one tool per verb, same envelopes, errors as
`isError` results.

## The loop

1. **`agenda`** — the worklist. Five sections:
   - `undecided`: genuinely contested nodes. These need an argument or a
     preference.
   - `ready`: issues whose labelling has settled on a unique justified
     position. `decide` them (or surface them to a human if deciding is not
     your call).
   - `unpopulated`: issues with no positions. `propose` one.
   - `untested`: positions that are IN only because nobody ever attacked them.
     **IN by silence is unexamined, not vindicated** — stress-test these
     (object, or find the strongest objection and state it) before anyone
     decides on them.
   - `assumptions`: premises recorded with `assume` that nobody has examined.
     Support or object to them; the biggest reframes start here.
2. **`moves <issue>`** — the legal, useful moves for one issue, each with its
   effect. Pick one; don't invent moves outside this list.
3. **Act** — `raise` / `reframe` / `propose` / `synthesize` / `support` /
   `object` / `assume` / `prefer` / `promote` / `audience` / `decide`.
4. **Re-read** — labels may have flipped anywhere in the graph
   (reinstatement). Repeat until the agenda is empty or your budget is spent.

To understand a label before contesting it: `why <node>` (attackers with their
text, and the moves that would flip it), `show <node>` (the node in full),
`explain <issue>` (the whole derivation, round by round). In text mode `why` and
`moves` print the flip suggestions as runnable `dlktk …` command lines with
`<text>`/`<label>` placeholders to fill; over JSON the same moves arrive as the
structured `to_flip` / `moves` arrays (`{move, args, effect}`).

To **explore before you commit** — all read-only:

- `whatif <issue> --object <t> / --prefer <w>:<l> / --without <n>` — apply
  hypothetical moves in memory and see the label diff. Probing is cheaper than
  polluting the append-only record with moves you then concede.
- `crux <issue>` — the load-bearing arguments: which single argument's removal
  flips a position. Attack the crux, or shore it up — that is where novel
  thinking has maximum leverage.
- `worlds <issue>` — the coherent maximal stances (preferred extensions) a
  contested issue admits, with positions sorted robust / contingent /
  hopeless. Use it when everything is UNDEC and you need the *shape* of the
  disagreement, not just the fact of it.
- `audiences` — which positions are justified under *every* declared value
  ranking (act on those) vs audience-sensitive (the real decision is whose
  values govern — name that and argue it).

## Diverge before you converge

Evaluation pressure kills generation. On any open-ended or wicked question,
run a divergence phase before any convergent move:

- **No `prefer` until the issue has ≥ 2 positions from ≥ 2 authors.** A
  preference stated against a single candidate is a rubber stamp.
- **Each persona contributes at least one generative move** (propose /
  synthesize / reframe / assume / raise --from) before stating any preference.
- **Rotate a devil's advocate**: each round, one persona must object to the
  strongest currently-untested IN position (the agenda's `untested` section
  points at it).

**Stalemates have three exits, in this order of consideration:**

1. `synthesize` — recombine the deadlocked rivals into a hybrid (lineage
   recorded). Honest caveat: the hybrid *joins* the rivalry (the stalemate
   becomes N+1-way) until the parents are conceded or a preference/audience
   elevates it.
2. `reframe` — if the deadlock signals a false dichotomy, replace the framing
   (`--basis` required; positions do not carry over; the old framing leaves
   the agenda and the lineage is recorded).
3. `prefer` — an honest value call, when the options really are exhaustive and
   the values really do decide.

## Values and audiences (for multi-stakeholder questions)

- Tag what a position/argument is *for*: `--promotes throughput` at creation,
  or `promote <node> <value>` later (own nodes only; one value per node).
- Record each stakeholder's ranking: `audience ops security velocity`
  (most important first). Re-declaring a name requires `--supersede --basis`.
- `status --under ops` evaluates under that ranking; `audiences` reports which
  conclusions survive every ranking. Robust-across-audiences is the closest
  thing a wicked problem has to objective justification.

## Move discipline

- **Search before you argue.** `search "<phrase>"` — if the claim already
  exists as a node, don't restate it; `object`/`support` the existing graph
  instead. Duplicate arguments don't change labels, they just bloat the graph.
- **An argument that should change the verdict must `object`,** not `support`.
  `supports` is recorded rationale only; the evaluator ignores it. To defend a
  position, defeat its attacker (object to the objection) — reinstatement does
  the rest.
- **Stalemates need a preference, a synthesis, or a reframe — not more
  arguments.** When `status` reports a stalemate, every position is UNDEC and
  symmetric; a new argument helps only if it attacks from outside the cycle.
  Work the three exits above; if you prefer, use an honest basis.
- **Decisions are closing acts — and provisionally so.** `decide` is rejected
  on an already-decided issue; reversals go through `supersede <issue>
  <position> --basis <label>`, which records *why* and links the prior
  decision. Deciding against the justified position is allowed but recorded as
  an override — say so explicitly if you do it. For a decision you know is
  provisional, record the horizon: `--review-by <T>`; `check` reports it once
  the horizon passes (re-affirm by superseding with the same position and a
  new horizon).
- **Withdraw your own mistakes** with `concede <node>`. You can only concede
  nodes *you authored* — ownership is checked against `--author`, the stable
  identity, never the persona.
- **Attribute yourself**: `--author <id>` is your stable identity (defaults to
  the OS user; on MCP, the `author` field). `--role <persona>` is the hat you
  argue under and is optional — set both for multi-agent runs. The first move
  you make under a role auto-records an author↔role binding (idempotent); read
  the bindings with `roster`, or pre-declare one with `roster <author> <role>`.
  Identity and persona are distinct: two agents sharing the `Security` persona
  still own only their own nodes.

## Personas (optional, for multi-agent deliberation)

Roles are conventions, not engine features — compose them in your harness:

- **Maintainer** — objects on long-term cost; prefers on `maintainability`.
- **Shipper** — proposes; prefers on `velocity`.
- **Security** — objects on threat; prefers on `security`.
- **Historian** — queries prior decisions (`log`, `replay`, `anchored`,
  `search --all`) and raises an issue when a new position contradicts a
  standing decision.

For open-ended / wicked questions, add the generative personas:

- **Reframer** — challenges issue formulations; uses `reframe` when a
  deadlock smells like a false dichotomy, and `raise --from` when an argument
  reveals a deeper question.
- **Analogist** — mines prior discussions (`search --all`, `anchored`) for
  candidate positions and imports them by value (a local node citing the
  source) — the Historian's tooling turned generative.
- **First-Principles** — names the premises with `assume`, then attacks them;
  works the agenda's `assumptions` section.
- **Stakeholder advocates** — one per value: each declares its `audience`
  ranking, tags its contributions with `--promotes`, and argues its corner;
  `audiences` then shows what survives everyone.

A minimal deliberation: each persona in turn reads `agenda` + `moves`, makes
at most one move, and passes. Stop when the agenda is empty, a stalemate
persists after every persona has had a chance to break it (escalate to a
human), or a move budget runs out. `examples/deliberate.sh` demonstrates the
mechanics end to end — including the divergence phase, synthesis, and the
exploration reads.

## Afterwards

- `export > <file>.ndjson` and commit it — the dialectic reviews like code.
- `check [--all] [--strict]` in CI: exit 5 means a recorded decision has
  drifted (its position is no longer justified) — re-argue or supersede it.
  `--strict` also fails on the warnings: lingering stalemates, decisions whose
  position was never attacked (`untested_decision`), decisions past their
  `--review-by` horizon (`review_due`), and decisions resting on a defeated
  assumption (`defeated_assumption`).
