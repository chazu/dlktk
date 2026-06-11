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
error/exit-code catalog. Pass `--format json` on every read; parse stderr as
`{error, detail, node?}` on nonzero exit.

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

1. **`agenda`** — the worklist. Three sections:
   - `undecided`: genuinely contested nodes. These need an argument or a
     preference.
   - `ready`: issues whose labelling has settled on a unique justified
     position. `decide` them (or surface them to a human if deciding is not
     your call).
   - `unpopulated`: issues with no positions. `propose` one.
2. **`moves <issue>`** — the legal, useful moves for one issue, each with its
   effect. Pick one; don't invent moves outside this list.
3. **Act** — `raise` / `propose` / `support` / `object` / `prefer` / `decide`.
4. **Re-read** — labels may have flipped anywhere in the graph
   (reinstatement). Repeat until the agenda is empty or your budget is spent.

To understand a label before contesting it: `why <node>` (attackers with their
text, and the moves that would flip it), `show <node>` (the node in full),
`explain <issue>` (the whole derivation, round by round).

## Move discipline

- **Search before you argue.** `search "<phrase>"` — if the claim already
  exists as a node, don't restate it; `object`/`support` the existing graph
  instead. Duplicate arguments don't change labels, they just bloat the graph.
- **An argument that should change the verdict must `object`,** not `support`.
  `supports` is recorded rationale only; the evaluator ignores it. To defend a
  position, defeat its attacker (object to the objection) — reinstatement does
  the rest.
- **Stalemates need a preference, not more arguments.** When `status` reports
  a stalemate, every position is UNDEC and symmetric; a new argument helps
  only if it attacks from outside the cycle. State `prefer <winner> <loser>
  --basis <label>` with an honest basis.
- **Decisions are closing acts.** `decide` is rejected on an already-decided
  issue; reversals go through `supersede <issue> <position> --basis <label>`,
  which records *why* and links the prior decision. Deciding against the
  justified position is allowed but recorded as an override — say so
  explicitly if you do it.
- **Withdraw your own mistakes** with `concede <node>`. You can only concede
  nodes you authored.
- **Attribute yourself**: pass `--role <name>` (CLI) or `author` (MCP) on
  every move so the record shows who argued what.

## Personas (optional, for multi-agent deliberation)

Roles are conventions, not engine features — compose them in your harness:

- **Maintainer** — objects on long-term cost; prefers on `maintainability`.
- **Shipper** — proposes; prefers on `velocity`.
- **Security** — objects on threat; prefers on `security`.
- **Historian** — queries prior decisions (`log`, `replay`, `anchored`,
  `search --all`) and raises an issue when a new position contradicts a
  standing decision.

A minimal deliberation: each persona in turn reads `agenda` + `moves`, makes
at most one move, and passes. Stop when the agenda is empty, a stalemate
persists after every persona has had a chance to break it (escalate to a
human), or a move budget runs out. `examples/deliberate.sh` demonstrates the
mechanics end to end.

## Afterwards

- `export > <file>.ndjson` and commit it — the dialectic reviews like code.
- `check [--all] [--strict]` in CI: exit 5 means a recorded decision has
  drifted (its position is no longer justified) — re-argue or supersede it.
