# dlktk — project analysis and improvement plan

*An assessment of dlktk's viability as a tool for robust agentic/human exploration of
design and implementation questions, based on a full read of the design doc and source
plus adversarial exercise of the built CLI.*

## Overall assessment

The foundation is strong: the layering is clean (only `internal/store` is pudl-aware,
`internal/af` is pure), the design doc records *why* for every decision, and several
design questions (§16 Q1–Q4, Q7) were dogfooded through dlktk itself — the best possible
credibility signal for a tool of this kind. The agent surface (`discover`, `agenda`,
`moves`, `why`, `explain`, structured errors, stable exit codes) is well ahead of most
CLI tools.

What stands between this and a *viable* tool falls into four buckets:

1. **Verified bugs** that break the tool's own stated guarantees.
2. A missing **multi-agent trust and concurrency** story.
3. **Agent-loop and integration** gaps.
4. **Project hygiene** (tests, CI, license).

## 1. Verified bugs

These were each reproduced against a built binary. *Items marked **[fixed]** are
addressed on this branch.*

### 1.1 Nondeterministic `status` ordering **[fixed]**

Design §8.1 promises byte-identical JSON for the same inputs ("ordering of arrays is
canonicalized"). It wasn't: `targetIssues` (`cmd/dlktk/main.go`) iterated the
`g.Nodes` map, so the all-issues `status` (and `replay`) issue order was random — five
consecutive runs produced two different orderings. Agents diffing snapshots or caching
on output cannot tolerate this. Fix: sort the issue list.

### 1.2 `decide` silently auto-supersedes, violating the dogfooded Q4 policy **[fixed]**

Design §16 Q4 — resolved *by running the dialectic through dlktk itself* — says: reject a
bare re-`decide` on an already-decided issue; require an explicit `supersede` move with a
mandatory `--basis` that links back to the prior decision. The implementation instead
auto-superseded inside `Mover.Decide` (with a comment citing a nonexistent "§16.7"): a
second `decide` succeeded with exit 0, no basis, no record of why the prior decision was
overturned. The tool whose thesis is "provenance outranks convenience" silently flipped
decisions. Fix: bare re-`decide` now exits 2; a new `supersede <issue> <position>
--basis <label>` move records the reversal and links the prior decision.

### 1.3 Soundness hole: cyclic preferences make contradictory positions both IN **[fixed]**

`prefer` rejects cycles at assert time (per Q2), but `import` wrote raw facts with no
legality pass, and `af.Build` did not defend against cycles already in stored data.
Importing a single NDJSON line creating `B>A` against an existing `A>B` produced:

```
IN    p:…  "mutex"    (reinstated)
IN    p:…  "rwlock"   (reinstated)
```

— both rivals on a `select_one` issue simultaneously justified, exactly the
all-prefer-all collapse §16 Q2 worried about. Since the design blesses "NDJSON as the
system of record", import is a first-class write path. Fix: `import` now validates the
batch (known relations, well-formed args, preference acyclicity against the combined
store + batch) before writing anything, and `af.Build` fails loud on a preference cycle
in stored data (store-invariant violation, exit 4) rather than computing nonsense.

### 1.4 TOCTOU between legality check and write **[mitigated, open]**

Every move is read-graph → check → write as separate store operations. Two concurrent
agents running `prefer A B` and `prefer B A` can both pass the cycle check and both
land, producing the corrupt state of §1.3 with no import needed. The §1.3 read-time
fail-loud is the backstop (the store can no longer *silently* serve contradictory
labellings), but the real fix is check-and-write inside one transaction, which likely
needs a small pudl API addition. Tracked as open.

### 1.5 Exit-code inconsistencies **[fixed]**

The discover contract says exit 3 = "a referenced id does not exist", yet every
legality not-found path returned 2 (`illegal_move`), and `store` errors were untyped
`fmt.Errorf` → exit 1 instead of 3/4. Agents branch on these codes. Fix: legality
checks distinguish not-found (3) from kind/ownership violations (2); store failures map
to 4; covered by tests.

## 2. Multi-agent trust and identity (not yet implemented)

- `--role` *is* the author: persona and identity are conflated, `concede` ownership is
  enforced on a freely spoofable string, and the design's `dlktk/roster` relation (§3.1)
  is unimplemented. Minimum: separate `--author` from `--role`, record both, implement
  roster. Signing (Q6) can stay deferred, but identity should be explicit rather than
  accidental.
- `export`/`import` round-trip only **current** facts: retractions, superseded
  decisions, and the tt audit trail are lost in the git-native log — the headline
  "reviewable provenance in PRs" feature drops the provenance. Full-history export is
  deferred in §15; it should be promoted to the top of the queue.

## 3. Highest-leverage additions for agentic/human exploration

In rough order of impact:

1. **`dlktk check` — a CI verb for decision drift.** **[implemented on this branch]**
   Exit 5 when a standing decision's position is no longer IN, when stored preferences
   are cyclic, or when store invariants are violated; `--strict` also fails on
   lingering stalemates; `--all` covers the whole store. This turns recorded decisions
   from archaeology into living constraints — the strongest adoption lever. The repo's
   CI now runs it against the shipped `examples/` dialectics on every PR.
2. **An MCP server (`dlktk serve --mcp`).** The discover schema already defines the
   moves/reads with legality strings and envelopes; generating MCP tool definitions
   from it is mostly mechanical. It makes every agent harness a client and resolves the
   per-process/TOCTOU issues as a bonus (one process, serialized writes).
3. **A reference deliberation harness.** The design correctly scopes out
   who-speaks-when, but viability requires *someone* to ship it. Even an `examples/`
   script that spawns the §11 personas as subagents and alternates moves until `agenda`
   is empty or a budget runs out would make the agentic half demonstrated rather than
   theoretical. Pair with a ready-made `AGENTS.md`/`CLAUDE.md` snippet documenting the
   loop (`discover` → `agenda` → `moves` → act → re-read).
4. **Close the read-surface gaps.** **[mostly implemented on this branch]** `show
   <node>` (design §6.2) now exists, and `why` embeds the node's and each attacker's
   text. Still open: `search <text>` so an agent rejoining a discussion can check
   whether an argument already exists — duplicate-argument piling is *the* predictable
   multi-agent failure mode.
5. **Richer `moves`/`agenda`.** **[implemented on this branch]** `moves` now suggests
   `decide` when an issue's labelling has settled on a unique justified position (the
   loop's terminal move), and `agenda` reports ready-to-decide and position-less issues
   alongside the UNDEC set. Still open: `support` suggestions.
6. **Batch moves.** `dlktk apply -` consuming NDJSON *moves* (not raw facts) from
   stdin: one process, one transaction, atomic legality — cheaper for harnesses and a
   natural fix-point for §1.4.
7. **Cheap design-level wins already in the back pocket:** the preference-immune
   "undercut" flag (one boolean per `objects_to`); cross-discussion citation (Q5 —
   without it the Historian persona has no teeth); requiring `--basis` on `prefer`
   (currently optional free text, which sits oddly with the Q4 "force the reasoning to
   be captured" ethos); a per-discussion CUE basis vocabulary.

## 4. Hygiene that gates adoption

- **Tests:** before this branch, one 54-line unit test covered a ~4.5k-line tool.
  Priorities: golden tests for every JSON envelope (the agent contract), a property
  test comparing `Grounded()` against brute force on random small graphs, an
  end-to-end CLI test replaying the INTRODUCTION example, an export→import round-trip
  test, a determinism test (run twice, byte-compare). The `examples/*.ndjson` files are
  ready-made fixtures.
- **CI:** **[implemented on this branch]** `.github/workflows/ci.yml` runs vet + test +
  build on PR, plus the dialectics dogfood job (import `examples/`, `check --all
  --strict`).
- **License is TBD** — blocks any external adoption outright.
- **`.dlktk/current` is written to whatever cwd you run from**, polluting arbitrary
  directories. Walk up to the git/pudl workspace root like `.pudl` discovery does, and
  document gitignore expectations.
- A canonical on-disk home for exports (e.g. `design/dialectics/<disc>.ndjson`) plus a
  `dlktk sync` that writes it — "git-native" currently means "you can pipe stdout
  somewhere".

## Suggested sequencing

1. **Bug fixes + tests** (§1) — *done on this branch*: determinism sort, `supersede`
   move, import validation + cycle fail-loud, exit-code audit.
2. **`check` + CI** (§3.1) — *done on this branch*: the `check` verb (exit 5) and a
   workflow that tests the code and drift-checks the example dialectics.
3. **MCP server + reference harness** (§3.2–3.3) — what makes "robust agentic
   exploration" real rather than aspirational.
4. **Identity/roster + full-history export** (§2) — completes the trust story.

A fitting way to settle the genuinely contested calls (MCP server vs. batch-stdin,
harness in-repo vs. separate) is to run them through dlktk itself, as was done for
Q1–Q4, and commit the NDJSON to `examples/`.
