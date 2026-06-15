# agon — a defeasible argumentation tool for agentic + human design dialectic

**Status:** draft / for review
**Working name:** `dlktk` (dialectic) Binary, package, and on-disk artifact names below assume `dlktk`.
**One-liner:** A lightweight, git-native CLI that records design dialectics as an IBIS graph, evaluates "what currently stands" via Dung grounded semantics over a defeat relation, and exposes the whole thing through a dual human/agent interface backed by [pudl](https://github.com/chazu/pudl)'s bitemporal fact store.

**Storage decision:** dlktk embeds pudl's public Go API (`github.com/chazu/pudl/pkg/factstore`, `.../pkg/eval`) as its persistence layer. It writes into the repo's existing pudl store (`.pudl/`), under a reserved `dlktk/*` relation namespace, so dialectics live beside observations and the catalog without interfering with normal pudl use. See §3 and §12.

---

## 1. Goals and non-goals

### Goals
- **Capture** design discussions as a typed, recursive argument graph (IBIS), scoped to a codebase artifact (file, package, commit, or free-form question).
- **Evaluate** the graph: report which positions are currently *justified*, *defeated*, or *genuinely open*, with reinstatement (a defeated attacker can be re-defeated, reinstating its target).
- **Defeasibility**: support priorities so that "security beats velocity" resolves an otherwise-tied dispute, without a heavyweight rule engine.
- **Dual-use**: every operation is equally drivable by a human at a terminal and by an agent over stdio/JSON. Neither is a second-class citizen.
- **Replayable**: bitemporal storage gives postmortem replay for free — "what did the argument state look like when we decided X, and what has changed since."
- **Lightweight**: a single static binary, no daemon, no server. Persistence is the repo's pudl store (`.pudl/`, one SQLite file); the daemon/coordination story is delegated to existing tooling — see §14.

### Non-goals
- Not a general argument-mining or NLP system. Text is opaque to the engine; structure carries all the semantics.
- Not a full defeasible-logic rule engine (no strict rules / defeaters / superiority à la Nute–Governatori). We deliberately stop at attack-modulo-preference; see §3.4 for why that's the right amount.
- Not bipolar argumentation. `supports` links are rationale only and do **not** affect labelling. Deliberate; revisited in §3.5.
- Not a chat transport or a scheduler. Moves are atomic appends; who-speaks-when is the harness's problem.

---

## 2. Conceptual model: a layered stack

The tool is three separable concerns that are usually tangled together. Keeping them separate is what keeps each one tiny.

```
L5  Interface     Cobra CLI · dual text/JSON · discover schema      (human + agent)
L4  Protocol      dialectical moves · legality · legal-move generation
L3  Defeasibility preference → defeat                              (Go)
L2  Evaluation    Dung grounded extension over the defeat relation (Go fixpoint)
L1  Capture       IBIS typed graph (nodes, links)
L0  Storage       pudl · bitemporal fact store over SQLite (pkg/factstore)
```

Each layer depends only on the one below it. The evaluator (L2/L3) never sees roles, personas, or natural language. The interface (L5) never reaches past the protocol (L4) into raw storage. Only `internal/store` is pudl-aware; everything above it speaks dlktk domain types.

**Evaluation is entirely in Go.** pudl's Datalog engine is positive/stratified (semi-naive, no negation) and the grounded labelling needs non-stratified recursion (§4). Rather than split the evaluation across the engine and Go, dlktk reads the EDB facts via pudl and computes the *whole* pipeline — attack, transitive preference, defeat, and the grounded labelling — in Go. pudl's Datalog is used only for optional integration queries that join against the catalog (§14), never for the core verdict.

### 2.1 IBIS as the capture ontology (L1)

The entire IBIS vocabulary:

| node kind  | meaning                                   |
|------------|-------------------------------------------|
| `issue`    | a question at stake ("which lock?")       |
| `position` | a candidate answer ("use an RWLock")      |
| `argument` | a claim bearing on a position or argument |

| link rel      | from → to                          | semantics            |
|---------------|------------------------------------|----------------------|
| `responds_to` | position → issue, issue → issue    | structural           |
| `supports`    | argument → position\|argument      | rationale (L1 only)  |
| `objects_to`  | argument → position\|argument      | **attack** (feeds L2)|

The recursion — arguments objecting to *other arguments* — is what makes this a dialectic rather than a bipartite pro/con list. It is also what makes reinstatement (§2.3) possible.

### 2.2 Mapping IBIS onto a Dung framework (L2)

A Dung Abstract Argumentation Framework is just `(Args, attack ⊆ Args × Args)`. We map:

- **AF arguments** = IBIS `position`s and `argument`s. (Issues are *not* AF nodes; they are the questions whose positions compete.)
- **attack** comes from two sources:
  1. every `objects_to` link, and
  2. mutual exclusivity: positions on a `select_one` issue attack each other (you can't pick two). Positions on an `open` issue do **not** auto-attack — they may be independently acceptable.

The **grounded extension** is the unique, most-skeptical set of acceptable arguments. We compute a three-valued labelling:

- `IN` — every attacker is `OUT` (unattacked nodes are vacuously `IN`). *Currently justified.*
- `OUT` — some attacker is `IN`. *Defeated.*
- `UNDEC` — neither. *Genuinely contested; this is the live agenda.*

The `UNDEC` bucket is the most useful output for agents: it is exactly the set of questions that still need a move or a tiebreak.

### 2.3 Reinstatement (why this beats a pro/con list)

Worked example — issue *"which lock?"*, `select_one`:

- positions **A** (mutex), **B** (RWLock) → mutually attack (select-one).
- argument **C** objects to B: "writer starvation."
- argument **D** objects to C: "workload is read-heavy; starvation won't occur."

D is unattacked → `IN`. So C → `OUT`. C was B's only non-positional attacker, so B is now attacked only by A. With no preference, A and B attack each other with no defeater → **both `UNDEC`**: the tool correctly reports *"still open, you need a tiebreaker."* Add `prefer(B, A)` and B → `IN`, A → `OUT`. Note that **D reinstated B** by defeating B's attacker. A flat list cannot express this.

### 2.4 Decisions are acts, not derivations

The grounded labelling tells you what is *justified*. A **decision** is the separate, recorded act of closing an issue by accepting a position. Normally you accept an `IN` position, but a human (or an agent with authority) may override and pick an `UNDEC` or even `OUT` position — that divergence is itself valuable signal and is recorded with a flag. The tool *advises*; the decider *decides*.

---

## 3. Data model (L0/L1 over pudl)

Everything is a pudl **fact**: `(id, relation, args, valid_start/end, tx_start/end, source, provenance)` where `args` is a JSON object with meaningful keys. dlktk's logical relations map one-to-one onto facts under a reserved `dlktk/*` relation namespace, so the design's n-ary tuples carry over verbatim (this is why pudl's fact store fits where its EAV cousin would not — see §3.6). Both temporal axes are pudl-native and keyed by Unix time:

- **transaction-time (tt):** when a fact was recorded / retracted. Drives audit and "what did we know when." A `retract`/`concede` move is `factstore.RetractFact` — it sets `tx_end`, closing the fact's tt interval without deleting it. History stays intact.
- **valid-time (vt):** when a fact is *true in the world*. Mostly degenerate for utterances (a position is "valid" as of when it was raised), but load-bearing for **decisions**, which are valid from an effective date until superseded. Supersession is `factstore.InvalidateFact` (sets `valid_end`).

### 3.1 Logical relations (written by moves, stored as `dlktk/*` facts)

Each row below is one pudl fact: `relation = "dlktk/<name>"`, `args = {…}`. The canonical dlktk node id (§3.3) lives in `args.id` — pudl's own content-addressed `Fact.ID` is an opaque storage key dlktk never surfaces.

```
dlktk/discussion   args{ id, title, subject, created_by }
dlktk/node         args{ id, disc, kind, text, author }       -- kind ∈ {issue, position, argument}
dlktk/link         args{ id, disc, src, dst, rel, author }    -- rel  ∈ {responds_to, supports, objects_to}
dlktk/issue_card   args{ issue, cardinality }                 -- cardinality ∈ {select_one, open}; default select_one
dlktk/preference   args{ id, disc, winner, loser, basis, author }
dlktk/decision     args{ disc, issue, position, basis, decider, override }   -- vt = effective period
dlktk/roster       args{ disc, author, role }                 -- author ↔ role binding (metadata only; see §11)
```

- `subject` is a structured ref to the thing under discussion: `file:pkg/cache/store.go`, `pkg:internal/af`, `commit:9f3a…`, or `q:<free text>`. Agents resolve this to anchor the dialectic to code; it can also be joined against pudl's `catalog_entry` relation (§14).
- `override` on `dlktk/decision` is set true iff the accepted position was not `IN` at decision time (recorded, not blocked).
- `source` (the pudl fact field) and `args.author` both carry the move's **author identity** (`--author`, default OS user) — the thing `concede`/`retract` ownership is checked against. The **persona** (`--role`) is recorded separately as a `dlktk/roster` binding, not on each move (§11, §16 Q8); `provenance` can carry the move's CLI invocation for audit.

**Reading.** `factstore.QueryFacts{Relation:"dlktk/node"}` (optionally `ValidAt`/`TxAt`) returns all node facts; dlktk filters by `args.disc` in Go. Discussions are tiny (tens of facts), so the absence of an arg-level filter on `QueryFacts` costs nothing. `internal/store` deserializes `args` JSON into domain types and back.

**Mutation.** Each move is one `AddFact`. Because `Fact.ID = SHA256(relation ∥ args ∥ valid_start ∥ source)` and every node carries a unique `args.id`, facts dedup naturally and `import` is idempotent (§12). To `concede`/`retract` node X, `internal/store` resolves the current `dlktk/node` fact whose `args.id == X`, then calls `RetractFact(fact.ID)`.

**"Current fact for id X" — precise resolution.** Across tt, several pudl facts can share an `args.id` (e.g. an asserted node later retracted). dlktk defines the *current* fact for id X as **the one pudl returns under AsOfNow** — i.e. `QueryFacts{Relation:"dlktk/node"}` (no `TxAt`/`ValidAt`) reads pudl's `current_facts` view, which is exactly the rows with `tx_end IS NULL AND valid_end IS NULL`. `internal/store` filters that result for `args.id == X` and asserts the invariant **|result| ≤ 1** (a node id is current in at most one fact). If two current facts share an id, that is a store bug, not a valid state — fail loud. `RetractFact` and all reads use this same AsOfNow lookup, so they can never select a stale row. Time-travel reads (`--as-of`/`--valid-at`) pass `TxAt`/`ValidAt` instead and accept whatever was current at that instant.

### 3.2 Derived relations (computed in Go)

These are derived from the EDB facts. They are shown below in datalog for clarity, but dlktk computes all of them in Go inside `internal/af` (see §3.6 / §4 for why); pudl's Datalog engine is not on this path.

Attack derivation (positive / stratified):

```prolog
% explicit objections
attack(A, B) :- link(_, D, A, B, objects_to).

% select-one positions are mutually exclusive
attack(P1, P2) :- link(_, D, P1, I, responds_to), node(P1,D,position,_,_),
                  link(_, D, P2, I, responds_to), node(P2,D,position,_,_),
                  issue_card(I, select_one), P1 \= P2.
```

Preference, with transitive closure (positive recursion — fine):

```prolog
preferred(W, L) :- preference(_, D, W, L, _, _).
preferred(W, L) :- preferred(W, M), preferred(M, L).
```

Defeat = attack that survives preference. **One** stratified negation (`defeat` depends on `not preferred`; `preferred` does not depend on `defeat`):

```prolog
defeat(A, B) :- attack(A, B), not preferred(B, A).
```

Note this `defeat` rule needs one stratified negation, which pudl's Datalog engine does **not** support (it is positive-only — `Atom` has no negated form). That alone would force `defeat` into Go; the grounded labelling over `defeat` (§4) then needs non-stratified recursion that *no* available engine supports. Both facts point the same way: compute the entire derived pipeline in Go.

### 3.3 Node identity

IDs are **proquints derived from timestamp-prefixed ULIDs** — the same scheme as `moor`. Pronounceable, sortable by creation, stable across machines, and agent-friendly to echo back. Optional one-char kind prefix for CLI ergonomics: `i:`, `p:`, `a:` (e.g. `p:kibod-marok`). The prefix is presentational; the canonical ID is the proquint.

dlktk generates these in `internal/id` (ULID → proquint) and stores them in `args.id`. This is deliberately *not* pudl's `Fact.ID`: pudl content-addresses facts (`SHA256(relation ∥ args ∥ valid_start ∥ source)`), which is unordered and not author-echoable. dlktk keeps its own ULID-sortable ids as the addressing scheme and treats pudl's `Fact.ID` purely as the storage key used internally for retraction lookups.

### 3.4 Why stop at attack-modulo-preference (defeasibility, L3)

Full defeasible logic (strict rules, defeasible rules, undercutting defeaters, a superiority relation) is real engineering and real cognitive load for the people writing the dialectic — and almost none of it earns its keep for *design decisions*, where the units of argument are prose claims, not formal rules. Attack + preference gives you: defeasible conclusions, priorities with a clear basis label (`security`, `velocity`, `precedent`…), and reinstatement. That is the whole point of defeasibility for this use case. Resist the rest.

### 3.5 Why `supports` is inert

Classic Dung has only attack. Making support carry weight means bipolar AF, which complicates the semantics (supported-attack, mediated-attack, several competing definitions) for little gain in a decision context. We keep `supports` as documented rationale that humans and agents read but the evaluator ignores. If a "support" should actually change the verdict, it belongs as an `argument` that `objects_to` an attacker (i.e. defend by counter-attack), which the grounded semantics already handles via reinstatement.

### 3.6 Why pudl's fact store (and not an EAV triple store)

pudl exposes two storage shapes. Its catalog and Datalog also read an Entity-Attribute-Value model (each tuple shredded into `(entity, attribute, value)` triples, reassembled by self-join in every rule). The **fact store** instead stores a whole tuple as one row: a named `relation` plus a JSON `args` object. dlktk's data model in §3.1 is n-ary tuples (`link(id, disc, src, dst, rel, author)`), so the fact store is a 1:1 fit — one move, one fact, no shredding, no per-rule reassembly tax. An EAV store would represent each link as five datoms and force every derivation to rejoin them; the fact store avoids that impedance mismatch entirely. The fact store is also bitemporal by Unix time, which makes `--as-of`/`--valid-at` (§9) a direct pass-through with no transaction-id translation.

### 3.7 Coexisting with pudl (relation namespacing)

dlktk shares the repo's pudl store, so it must not perturb normal pudl use:

- **All dlktk facts are namespaced `dlktk/*`.** pudl's own relations (`observation`, `depends`, …) and the reserved `catalog_entry` are untouched. `pudl facts list --relation observation` never sees dlktk facts because it filters by exact relation name.
- **CUE schema package `pudl/dlktk`** (shipped by `internal/discover`) formalizes the `args` shape of each `dlktk/*` relation, so the facts validate under pudl's existing CUE tooling rather than being opaque blobs. pudl ≥ v0.1.3 ships this package as a built-in bootstrap schema, so registration is automatic; the two copies are kept in lockstep, with `dlktk schema` as the reference.
- **`dlktk/*` facts are first-class, not hidden.** A design goal (confirmed) is that pudl, mu, and nous *leverage* dialectic facts — nous's bridge already scans/queries arbitrary relations through `pkg/factstore`+`pkg/eval`, so a `dlktk/decision` or `dlktk/preference` fact is consumable by other tools with zero new plumbing. The namespace exists for collision-avoidance and clarity, not concealment. Registering the `pudl/dlktk` CUE schema package is what makes these facts *interpretable* by other tools (typed args), and lets `pudl query` join dialectics against the catalog. The only optional pudl-side nicety is a cosmetic display filter so a bare `pudl facts list` isn't noisy — facts stay fully queryable regardless.

**Shared-store safety (audited).** pudl never mutates facts it doesn't own: the only writes to the `facts` table are `RetractFact(id)` / `InvalidateFact(id)`, both keyed by explicit id; there is no bulk delete, GC, compaction, or prune, and `verify`/`reinfer`/`doctor` don't touch facts. dlktk and pudl can therefore share one `.pudl/` safely. **Constraint:** dlktk mutates *only* through `pkg/factstore` (never raw SQL), so pudl's `current_facts` materialized view stays consistent.

---

## 4. Evaluation engine (L2)

### 4.1 The grounded labelling problem

The clean datalog encoding recurses through negation:

```prolog
defeated(A)             :- defeat(B, A), in(B).
has_live_attacker(A)    :- defeat(B, A), not defeated(B).
in(A)                   :- arg(A), not has_live_attacker(A).
```

`in → not has_live_attacker → not defeated → in` is not stratified. It needs **well-founded semantics (WFS)**, and there is a classical correspondence: *the WFS of this program is exactly Dung's grounded extension*.

pudl's Datalog engine is positive/stratified only — no WFS, and (as noted in §3.2) no negation at all. There is therefore one path, not two: dlktk reads the EDB facts via `factstore.QueryFacts`, derives `attack`/`preferred*`/`defeat` in Go, and computes the grounded fixpoint in Go over the materialized `arg ∪ defeat` view (§4.2). pudl remains the source of truth for the graph; the semantically-hairy bit is small, deterministic, and lives in `internal/af`.

Adding WFS to pudl would collapse the labelling into the three rules above and make replay a pure `as-of` evaluation — genuinely elegant — but it is a non-trivial engine project (an alternating-fixpoint extension to the semi-naive evaluator) that earns its keep only if *other* pudl consumers need recursion through negation. It is out of scope for dlktk and tracked separately; the 25-line Go fixpoint below is correct and sufficient. (This retires the `doctor` engine-probe and former open question on litelog WFS.)

### 4.2 The Go fixpoint (default path)

Monotone least-fixpoint of Dung's characteristic function; iteration order is irrelevant because grounded is unique → deterministic output, which agents rely on.

```go
// args: all AF node IDs. defeat: successful attacks (post-preference).
func grounded(args []string, defeat [][2]string) map[string]Label {
    attackers := map[string][]string{}
    for _, d := range defeat { // d = {attacker, target}
        attackers[d[1]] = append(attackers[d[1]], d[0])
    }
    label := map[string]Label{}
    for _, a := range args { label[a] = UNDEC }

    for changed := true; changed; {
        changed = false
        for _, a := range args {
            if label[a] != UNDEC { continue }
            allOut, anyIn := true, false
            for _, b := range attackers[a] {
                switch label[b] {
                case IN:    anyIn = true
                case UNDEC: allOut = false
                }
            }
            switch {
            case allOut: label[a] = IN;  changed = true   // incl. unattacked (vacuous)
            case anyIn:  label[a] = OUT; changed = true
            }
        }
    }
    return label
}
```

Complexity is `O(|args| · |defeat|)` worst case; design discussions are tiny (tens of nodes), so this is free. The `arg` set is read from `dlktk/node`+`dlktk/link` facts and `defeat` is derived in Go, both at the requested tt/vt snapshot (`QueryFacts` with `TxAt`/`ValidAt`).

### 4.3 Explanation (`why`)

For any node, walk the defeat graph to produce: its label, the chain that produced it (which attacker is `IN`/`OUT` and why), and — most usefully — the **actionable move** to flip it. This is the grounded dialogue game in reverse:

- to flip `IN → OUT`: introduce an undefeated attacker, or state a preference that promotes one of its existing attackers.
- to flip `OUT → IN`: defeat the attacker(s) that are currently `IN` (counter-argument or preference).
- to resolve `UNDEC`: usually a preference (the tied select-one case) or a new argument that breaks the cycle.

`why` is what lets an agent reason about *what to do next* instead of just reading a verdict.

---

## 5. Move protocol (L4)

Moves are the only way to mutate state. Each is one pudl `AddFact` (atomic append; tt-stamped by `tx_start`). Nothing is destroyed — `retract`/`concede` close tt intervals via `RetractFact`; decision supersession closes vt intervals via `InvalidateFact`.

| move      | effect                                                            | legality |
|-----------|-------------------------------------------------------------------|----------|
| `raise`   | add `issue` (optionally `responds_to` a parent issue)             | parent must be an issue |
| `propose` | add `position` + `responds_to` an issue                           | target must be an issue |
| `support` | add `argument` + `supports` a position/argument                   | target ∈ {position, argument} |
| `object`  | add `argument` + `objects_to` a position/argument                 | target ∈ {position, argument} |
| `prefer`  | add `preference(winner, loser, basis)`                            | both must be AF nodes; no preference cycle |
| `decide`  | record `decision(issue, position, basis)`; flag override if not `IN` | position must `respond_to` the issue; issue not already decided (Q4) |
| `supersede` | overturn the standing decision: `InvalidateFact` the prior, record a new decision linking it | issue already decided; `--basis` mandatory (Q4) |
| `concede` | `RetractFact` the `dlktk/node` fact for one of *your own* arguments (withdraw it) | author (identity, not persona) owns the node |
| `retract` | alias of concede for any node kind                                | author (identity, not persona) owns the node |
| `roster`  | with no args, list the author↔role bindings; with `<author> <role>`, pre-declare one. Moves under a `--role` auto-record the binding (§16 Q8) | none |

Legality is enforced in the `ibis` package *before* any write. Illegal moves exit non-zero with a structured error (§8.4) and change nothing.

### 5.1 Node immutability (decided)

There is **no `edit` move.** A node's `text` is fixed at creation. Corrections are made dialectically: `concede` the node (closing its tt interval, §3.1) and `propose`/`object` afresh with a new id. This is deliberate — it keeps the audit trail honest (you can see that a claim was withdrawn, not silently rewritten) and matches pudl's append-only ethos. The cost is that a typo fix mints a new id; acceptable for a decision-record tool where provenance outranks convenience. If a text-supersede move is ever added, it must mint a **new** `args.id` and link back to the old one rather than mutating in place. (This closes the former node-mutability open question in favour of immutability; revisit only if usage demands it.)

**Legal-move generation.** `dlktk moves <issue>` (and `dlktk agenda`) enumerate the currently-legal, *useful* moves for the caller — e.g. "B is `IN`; to contest it, `object` it or `prefer` a rival." This is the engine-side of giving agents a role in the discussion: it bounds the action space so a harness doesn't have to guess.

---

## 6. CLI surface (Cobra)

### 6.1 Global flags

```
--discussion, -d   discussion id (else: $DLKTK_DISC, else ./.dlktk current pointer)
--format           text | json   (default: text on a TTY, json when piped)
--color            auto | always | never   (auto = color on a TTY, off when piped or NO_COLOR set)
--as-of            transaction-time travel: evaluate as the graph stood at T
--valid-at         valid-time: which decisions were in force at T
--author           stable identity attributed to moves and checked for ownership (default: OS user; §11, §16 Q6)
--role             persona a move is made under; auto-records an author↔role roster binding (metadata; §11, §16 Q8)
--store            path to the pudl store dir (default: repo .pudl/ via DiscoverWorkspace, else ~/.pudl)
```

`--format` auto-detection (isatty) means humans get pretty output and pipes get JSON with zero ceremony; agents should still pass `--format json` explicitly for predictability. Text output is colorized (grounded labels `IN`/`OUT`/`UNDEC` in green/red/yellow, ids dimmed) when stdout is a TTY and `NO_COLOR` is unset; `--color always|never` overrides. In text mode `why`/`moves` render their suggestions as runnable `dlktk …` command lines, and read errors print a `hint:` next-step line.

### 6.2 Command tree

```
dlktk
  new        <title> --subject <ref>          create a discussion, set as current
  use        <disc>                            set current discussion
  list                                         discussions

  raise      <text> [--parent <issue>]
  propose    <issue> <text>
  support    <target> <text>
  object     <target> <text>
  prefer     <winner> <loser> --basis <label>
  decide     <issue> <position> [--basis <label>]
  supersede  <issue> <position> --basis <label>   overturn the standing decision
  concede    <node>
  retract    <node>                            (alias of concede)
  roster     [<author> <role>]                 list / pre-declare author↔role bindings

  status     [<issue>]                         grounded labelling of positions
  agenda                                        all UNDEC nodes = the live questions
  moves      <issue>                            legal + useful next moves
  why        <node>                             explanation + how to flip the label
  show       <node>                             one node + its links
  tree       [<issue>]                          the IBIS graph, indented

  replay     <issue> --as-of <T> [--diff]      labelling at T (and what changed since)
  log        [<node>]                           tt history (audit trail)
  check      [--all] [--strict]                 verify standing decisions: drift /
                                                stalemates / store invariants (exit 5
                                                on findings; CI-friendly)

  export     [--format ndjson]                 dump moves for git review
  import     <file>                             load moves (validated; idempotent by node id)
  mcp                                            serve the verb set over MCP stdio (agent harnesses)
  discover                                       machine-readable capability schema
  doctor                                         self-check: pudl store resolves, dlktk schema present, fixpoint sanity
```

The verb set is identical for humans and agents; only the rendering differs.

---

## 7. Human experience

- `status` renders positions grouped by issue, color-coded `IN`/`OUT`/`UNDEC`, with the one-line advice ("B justified", "A vs B tied — needs a preference").
- `tree` is the readable IBIS outline; `why` is the prose explanation.
- `--as-of` and `replay --diff` answer "what changed since we decided this" in review.
- Decision logs export to NDJSON so they live **in the repo** and show up in PRs as reviewable diffs — mirroring the `moor` git-native pattern. A reviewer sees the dialectic that produced a decision next to the code that implements it.

---

## 8. Agent interface (the part that has to be right)

### 8.1 Principles

- **Deterministic.** Same inputs + same `as-of` ⇒ byte-identical JSON. Grounded is unique; iteration order is hidden; ordering of arrays is canonicalized (by proquint).
- **Idempotent where possible.** Moves are append-only; `import` is keyed by node id so replaying a move stream converges. Retract is explicit, never implicit.
- **Self-describing.** `discover` lets an agent learn the vocabulary cold, with no prompt baked in.
- **Structured failure.** Every error is a JSON envelope on stderr with a stable code and a meaningful exit status, so a harness can branch on it.

### 8.2 `discover` (CUE — matches the AGENTS.md convention)

`dlktk discover` emits the command/schema contract as CUE (also available as JSON Schema via `--format json`). Shape:

```cue
#Dlktk: {
  tool:    "dlktk"
  version: string
  ids:     "proquint"           // how to read/echo node ids
  kinds:   ["issue","position","argument"]
  rels:    ["responds_to","supports","objects_to"]
  labels:  ["IN","OUT","UNDEC"]
  moves: [...#Move]
  reads: [...#Read]
}
#Move: { name: string, args: [...#Arg], legality: string, mutates: true }
#Read: { name: string, args: [...#Arg], returns: #Schema, mutates: false }
```

An agent's loop is then: `discover` → read `status`/`agenda`/`moves` → choose a legal move → emit it → re-read. No tool-specific prompt engineering required beyond the role persona (§11).

### 8.3 Read envelopes (`--json`)

`status`:
```json
{
  "discussion": "i:rusab-tomid",
  "issue": "i:rusab-tomid",
  "issue_text": "which lock for the cache?",
  "cardinality": "select_one",
  "positions": [
    {"id":"p:kibod-marok","text":"RWLock","label":"IN",
     "attacked_by":["p:hodup-bonil"],"defeated_by":[],"reinstated":true},
    {"id":"p:hodup-bonil","text":"mutex","label":"OUT",
     "attacked_by":["p:kibod-marok"],"defeated_by":["p:kibod-marok"]}
  ],
  "undecided": [],
  "advice": "RWLock justified; mutex defeated via prefer(RWLock,mutex)."
}
```

`why`:
```json
{
  "node":"p:hodup-bonil","label":"OUT",
  "because":[{"attacker":"p:kibod-marok","attacker_label":"IN",
              "reason":"preferred(RWLock,mutex) — basis=throughput"}],
  "to_flip":[{"move":"prefer","args":["p:hodup-bonil","p:kibod-marok"],
              "effect":"would make mutex IN, RWLock OUT"}]
}
```

`moves` returns the legal-move list in the same `{move,args,effect}` shape as `to_flip`.

### 8.4 Error envelope + exit codes

```json
{"error":"illegal_move","detail":"object target must be a position or argument, got issue","node":"i:rusab-tomid"}
```

| code | meaning            |
|------|--------------------|
| 0    | success            |
| 1    | generic error      |
| 2    | illegal/ill-formed move (nothing written) |
| 3    | not found          |
| 4    | store/engine error |

---

## 9. Bitemporal replay (the postmortem story)

- **`decide`** writes a `decision` with vt starting now and snapshots the grounded labelling reference. The labelling itself is *re-derivable*, not copied, so it can never drift from the graph.
- **`replay <issue> --as-of T`** evaluates the grounded labelling over the graph as it stood at transaction-time `T` (`QueryFacts`/`Query` with `TxAt = T`) — i.e. the exact justification state at the moment of decision.
- **`replay … --diff`** diffs the `T` labelling against now: which arguments were added, which positions flipped, whether the decided position is still `IN`. This is the "is this decision still load-bearing?" check, and it is the same replay shape as pudl's bitemporal postmortem queries.
- **`--valid-at T`** answers the orthogonal question: which *decisions* were in force on date `T` (`ValidAt = T`; superseded decisions had their vt interval closed by `InvalidateFact` when a later `decide` on the same issue landed).

Two axes, two distinct questions: tt = "what did we know / argue when," vt = "what was in effect when."

---

## 10. Discussion scoping and the codebase anchor

A discussion is the unit of scope; commands resolve the current discussion from `-d`, then `$DLKTK_DISC`, then a `./.dlktk/current` pointer (git-style). The `subject` field anchors the dialectic to code (`file:…`, `pkg:…`, `commit:…`, or `q:…`), so an agent reviewing `pkg/cache` can find the discussion that governs it, and a decision log can be cross-referenced to the lines it constrains.

---

## 11. Roles (convention, not engine)

Roles never touch the evaluator. A role (persona) is distinct from the **author** (the stable identity attributed to a move via `--author` and checked for ownership, §3.1, §16 Q6); two agents can share a persona yet own only their own nodes. Roles are: (a) a `roster(disc, author, role)` binding — auto-recorded the first time an author moves under a `--role`, and listable / pre-declarable with the `roster` move (§16 Q8), (b) a `--role` attribution on moves, and (c) the `basis` label an agent tends to use on preferences. Suggested personas, each just a prompt modifier plus a bias toward certain link types:

- **Maintainer** — objects on long-term cost; prefers on `maintainability`.
- **Shipper** — proposes; prefers on `velocity`.
- **Security** — objects on threat; prefers on `security` (usually high-priority basis).
- **Historian** — queries prior decisions (`log`, `replay`, `--valid-at`) and raises issues when a new position contradicts a standing decision.

Because roles are pure metadata, the same engine serves a solo human, a human + one agent, or a swarm, with no code change. The harness composes personas; `dlktk` referees.

---

## 12. Concurrency and storage

- Persistence is the repo's pudl store (`.pudl/`, one SQLite file), resolved via `factstore.DiscoverWorkspace`; no server. dlktk facts share the DB with pudl's catalog/observations but are isolated by the `dlktk/*` relation namespace (§3.7).
- Each move is a single `AddFact`. Append-only + bitemporal means concurrent writers rarely conflict — they simply land at different `tx_start`. SQLite WAL (pudl's setting) handles the rest for the expected (low) write rate.
- Reads snapshot at the latest committed tt unless `--as-of` is given, so an agent reasons over a consistent view within a turn.
- `export --format ndjson` / `import` give a git-native, human-reviewable move log; `import` is idempotent because each move re-asserts a fact with the same `args.id` (and pudl content-addresses by `relation ∥ args ∥ valid_start ∥ source`), so the NDJSON file can be the system of record and the pudl store a derived cache if a team prefers that topology.

---

## 13. Package layout

```
cmd/dlktk/                main()
internal/store/          pudl/pkg/factstore binding: AddFact/QueryFacts/Retract/Invalidate,
                         args (de)serialize, nid→Fact.ID resolve, tt/vt pass-through, namespacing
internal/ibis/           domain types (Node, Link, Kind, Rel) + move legality
internal/af/             attack + preferred* + defeat derivation, grounded fixpoint, explanation (all Go)
internal/proto/          move dispatch, legal-move generation
internal/render/         text vs json envelopes (one switch, two backends)
internal/discover/       CUE capability schema (+ pudl/dlktk args schema package)
internal/id/             ULID → proquint
```

The dependency arrow points strictly downward (`render`/`discover` over `proto` over `af`/`ibis` over `store`). `internal/store` is the only pudl-aware package — it imports `github.com/chazu/pudl/pkg/factstore` (and `.../pkg/eval` for the §14 catalog joins). `af` knows nothing of CLI or pudl beyond the materialized `arg ∪ defeat` view it's handed.

---

## 14. Integration points (kept optional)

- **pudl catalog join** — `subject` refs (`file:…`, `pkg:…`) can be matched against pudl's `catalog_entry` relation via a `pudl/dlktk` Datalog rule (`pkg/eval` + `Store.Query`), answering "which discussion governs this artifact?" and powering the Historian role's cross-references. This is the one place dlktk uses pudl's Datalog engine. Join-only, optional.
- **nous / mu fact consumption** — because dlktk facts are first-class `dlktk/*` rows in the shared store (§3.7), nous's bridge (`ScanFacts`/`QueryFacts` over `pkg/factstore`+`pkg/eval`) can ingest `dlktk/decision`/`dlktk/preference` facts as reasoning units, and mu can act on them, with no dlktk-specific code — provided the `pudl/dlktk` CUE schema is registered so args are typed. This is the inverse of the catalog join: other tools reading *out* of the dialectic rather than dlktk reading the catalog.
- **genso** — emit a move event on each `AddFact` for durable streaming / live dashboards.
- **AGENTS.md** — `dlktk discover` is the `discover` subcommand for this tool under the existing convention, so a generic harness picks it up with no bespoke glue.
- **pudl/CUE** — the discover schema, the `pudl/dlktk` args schema (§3.7), and (optionally) `basis` vocabularies are CUE, validatable with pudl's existing tooling.

None of these are required for the core to run standalone.

---

## 15. Phasing

0. **Prerequisite (in pudl)** — rename pudl's Go module from `pudl` to `github.com/chazu/pudl` (rewrite internal `pudl/...` imports accordingly) so dlktk can `require github.com/chazu/pudl` and import `.../pkg/factstore`. Mechanical but blocking; nothing in dlktk compiles against pudl until this lands.
1. **MVP** — `internal/id`, `internal/store` (factstore binding + namespacing + nid resolve), `internal/ibis` legality, `internal/af` (`defeat` + Go grounded fixpoint); `new/use`, the six structural moves (`raise/propose/support/object/prefer/decide`), `status`, `tree`, text + JSON. Proves the loop end-to-end against a real pudl store.
2. **Agent-complete** — `discover` (+ `pudl/dlktk` CUE schema), `agenda`, `moves`, `why`, structured errors + exit codes, `--as-of` (→ `TxAt`). This is the milestone at which an unattended agent can drive a full dialectic.
3. **Replay/postmortem** — `replay --diff`, `--valid-at` (→ `ValidAt`), `log`, decision-supersession via `InvalidateFact`.
4. **Git-native + integration** — `export/import` NDJSON, `catalog_entry` subject-anchoring (`pkg/eval` join), genso events, `pudl/dlktk` CUE schema registration + optional cosmetic display filter (§3.7).

   *Shipped:* `export`/`import` (current-state NDJSON, idempotent by content — enabled by making pudl `AddFact` `INSERT OR IGNORE`), `schema` (emits the `pudl/dlktk` CUE package), and `anchored <subject-substring>` (the practical form of subject-anchoring: matches discussions by `subject`). *Since shipped:* full-history export replay (`export --history` interleaves retract/invalidate events keyed by pudl's content-addressed fact ids; `import` replays them idempotently — event order is exact, though pudl stamps tx times at import). *Deferred:* the richer `catalog_entry` datalog join, which needs an agreed `subject`↔catalog key convention (e.g. `subject: catalog:<resource_id>`) before exact-match joining is meaningful; genso events (external, optional).

---

## 16. Open questions

_(Resolved this revision: WFS-in-engine — pudl is stratified-only, so the Go fixpoint is the single path, §4.1; supersession mechanism — `InvalidateFact`, §9; shared-store blast radius — audited safe, §3.7; cross-tool fact leverage — first-class via the shared API, §3.7; node immutability — decided, §5.1; **preference transitivity — resolved by dogfood, see Q2 below**; **decision supersession policy — resolved by dogfood, see Q4 below**; **cyclic-attack / stalemate reporting — resolved by dogfood and implemented, see Q3 below**; **pudl version pinning — resolved by dogfood and applied, see Q1 below**; **cross-discussion reference — resolved by dogfood, see Q5 below**; **roster binding policy — resolved by dogfood and implemented, see Q8 below**, which also splits identity from persona and so settles the identity half of Q6. The one remaining open question is Q6's signing half — whether move attribution needs cryptographic signatures or the audit trail suffices.)_

1. **pudl version pinning** — *resolved (dogfooded in dlktk itself; see [`examples/q1-pudl-dependency.ndjson`](examples/q1-pudl-dependency.ndjson)) and applied.* **Drop the committed `replace`; pin the tagged release (`require github.com/chazu/pudl v0.1.2`) and bump deliberately via `go get`.** Phase 0 is done (module renamed, `v0.1.2` tagged at HEAD, zero drift). The dialectic, run through dlktk's own grounded labelling, eliminated the two rivals by structure (no `prefer`): a committed `replace` breaks every build lacking a sibling checkout (CI, fresh clones, the nous consumer) and masks drift between the required version and what compiles — its sole advantage, inner-loop speed, is recovered by a *git-ignored `go.work`* that overrides the pin locally without committing it; tracking `main` makes each build depend on an unreviewed commit and removes the pinned escape hatch that replay/postmortem needs. Pinning's one cost (lagging bugfixes) is answered by the same auditability ethos as the rest of dlktk — a bump is a one-line `go get` landing as a reviewable diff. Applied: `replace` removed from `go.mod`, `go.sum` now records `v0.1.2`, `go.work`/`go.work.sum` git-ignored; both the committed (`GOWORK=off`) and co-dev (workspace) builds verified. **nous migrated identically** (dropped `replace`, pinned `v0.1.2`, git-ignored `go.work`; vet + tests green both modes).
2. **Preference transitivity** — *resolved (dogfooded in dlktk itself; see [`examples/q2-preference-transitivity.ndjson`](examples/q2-preference-transitivity.ndjson)).* **Close `preference` transitively, gated on assert-time rejection of cyclic preference.** The dialectic, run through dlktk's own grounded labelling, isolated exactly two objections any answer must survive: (a) explicit-edges-only leaves `preference` a non-strict-order, so defeat-modulo-preference loses well-foundedness; (b) naive closure over a *cyclic* preference (`A>B>C>A`) collapses to all-prefer-all. Rejecting cycles at assert-time defuses (b) while (a) stands unrebutted against the explicit-only rival — so closure is reinstated and decided **with no `prefer` tiebreak needed**, purely by argument structure. The "closure manufactures surprising defeats" worry is answered by auditability: every derived edge is a logged move and `why` prints the closure chain. The `--no-transitive` flag therefore drops from correctness-requirement to optional ergonomics.
3. **Cyclic-attack reporting** — *resolved (dogfooded in dlktk itself; see [`examples/q3-stalemate-reporting.ndjson`](examples/q3-stalemate-reporting.ndjson)) and implemented.* **Flag the stalemate.** Run through dlktk's own grounded labelling, the dialectic settled by structure — no `prefer` tiebreak. The leave-it position falls because plain `UNDEC` conflates "genuinely contested, add an argument" with "structurally unwinnable on these nodes," so an agent loops by proposing more objections that re-enter the deadlock; its rebuttal (the advice already names "a preference," and detection is engine complexity) is defeated by the observation that the trigger needs *no cycle enumeration* — "all positions `UNDEC`, none defeated-`OUT`" is already computed by the labelling, so it is cheap. The false-positive objection (even mutual attacks also go `UNDEC` and a fresh outside argument can resolve them) is answered by flagging only the *computable* fact, never a parity guess. Implemented: `IssueStatus.stalemate` (bool, agent surface) plus an advice string — "mutual stalemate — … all UNDEC, none defeated; a preference resolves this (a new argument helps only if it defeats from outside the stalemate)." Fires only when every position is `UNDEC` (≥2), so it never misleads on partially-resolved issues.
4. **Decision supersession policy** — *resolved (dogfooded in dlktk itself; see [`examples/q4-decision-supersession.ndjson`](examples/q4-decision-supersession.ndjson)).* **Reject a bare re-`decide` on an already-decided issue; require an explicit `supersede` move that links back to the prior decision.** Run through dlktk's own grounded labelling, the dialectic settled purely by argument structure — no `prefer` tiebreak. The auto-invalidate position falls to one objection: a silent auto-flip records *no basis for why* the prior decision was overturned, and provenance outranks convenience (the same ethos that bans an `edit` move, §5.1). Its rebuttal — "pudl is append-only, `InvalidateFact` only closes the tt interval so `log` keeps both decisions" — is itself defeated: retaining the bare fact in `log` is not the point; only an explicit `supersede --basis` *forces capturing the reasoning* for the reversal, not just the flip. The lone objection to the explicit policy (extra-move friction for agents) is reinstated against: `decide` is a rare, issue-closing move, so gating it is appropriate — the same logic used for cyclic-preference rejection (Q2). Mechanism was already settled (`InvalidateFact`, §9); this fixes the policy: a `supersede` move mints the new decision, invalidates the prior, and records a mandatory basis linking the two. *Implemented:* bare re-`decide` exits 2; `supersede <issue> <position> --basis <label>` records the new decision with `supersedes` naming the prior position.
5. **Cross-discussion reference** — *resolved (dogfooded in dlktk itself; see [`examples/q5-cross-discussion-reference.ndjson`](examples/q5-cross-discussion-reference.ndjson)).* **Stay strictly intra-discussion for graph edges; the Historian cross-references a foreign decision by *value* — a local `argument` node carrying the prior decision's id, text, and `as-of T` — never a live inter-discussion edge.** Run through dlktk's own grounded labelling, the dialectic settled purely by argument structure — no `prefer` tiebreak. The live-edge position falls to one objection: an edge whose `dst` lives in another discussion makes this discussion's labelling **non-local** — it now depends on the other discussion's mutable state, so a `supersede`/`concede` there with *no move here* silently flips labels here, breaking per-discussion replay and reproducibility. Its rebuttal ("pudl is append-only and `as-of` replay resolves the other side deterministically") is itself defeated: *deterministic is not local* — replay still forces evaluating the other discussion's full fixpoint at T, and §3.7 audited the shared store safe **only because labelling stays per-discussion scoped**; a live edge breaks exactly that invariant. The lone objection to cite-by-value (a frozen snapshot goes stale if the cited decision is later superseded) is reinstated against: staleness is *correct* — the cite records `as-of T`, so it is honestly scoped, not silently wrong; a change in the other discussion is a new fact the Historian surfaces as a fresh cite or issue, the same explicit-supersede discipline as Q4. The `catalog_entry`/`anchored` join (§14) is therefore how the Historian *discovers* what to cite, not a mechanism for live cross-discussion edges — consistent with §15 already deferring the richer catalog datalog join.
6. **Authorship trust** — *identity half resolved (see Q8); signing half still open.* `--author` (stable identity) is now separate from `--role` (persona), recorded per move via pudl's `source` + `args.author`, and `concede`/`retract` ownership is checked against the author, not the persona. Identity is therefore explicit and auditable rather than accidental — but still **self-asserted**: without signatures, `--author alice` is a claim, not proof. Open: whether to sign moves (sigstore/biscuit) for tamper-evident attribution, or whether the repo + NDJSON audit trail is sufficient. The Q8 split is the prerequisite that makes signing addable later (a clean identity field to sign over).
8. **Roster binding policy** — *resolved (dogfooded in dlktk itself; see [`examples/q8-roster-binding-policy.ndjson`](examples/q8-roster-binding-policy.ndjson)) and implemented.* **Auto-record the `dlktk/roster` binding on first use of an `(author, role)` in a discussion; keep a `roster` move for explicit pre-declaration and listing; never hard-gate `--role`.** Run through dlktk's own grounded labelling, the dialectic settled purely by argument structure — no `prefer` tiebreak. The "require an explicit binding before `--role` is accepted" rival falls: roles never touch the evaluator (§11), so a binding flips no labels, and the Q4/Q7 force-capture gate exists to prevent *silent graph rewrites* — that rationale does not transfer to pure metadata, so gating `--role` is friction with no soundness payoff (its consistency-with-Q4 defense is itself defeated: Q4 protects *decision* provenance, which auto-record already satisfies). The "roster is purely optional decoration" rival also falls: if the binding is never written, the author↔role fact §3.1 mandates is not queryable — "who played Security here?" is unanswerable and the relation is decorative, defeating its §3.1 purpose. Auto-record survives its one objection (auto-writing an un-issued fact is a hidden side effect) because the roster fact is content-addressed and dedups (idempotent §12), is one append first-use-only, and is fully visible in `log`/`export` — convenient, not hidden. This also **resolves Q6's identity half**: `--author` (stable identity) is now split from `--role` (persona), and `concede`/`retract` ownership rides on the author, not the spoofable persona; signing (the rest of Q6) stays deferred. *Implemented:* `--author`/`--role` split; auto-record inside each move's transaction; `roster` move/read; ownership check on author; `dlktk/roster` in the CUE schema; contract `0.8.0`.
7. **Setting cardinality** — *resolved (dogfooded in dlktk itself; see [`examples/q7-cardinality-setting.ndjson`](examples/q7-cardinality-setting.ndjson)) and implemented.* **A `--card select_one|open` flag on `raise`, fixed at issue creation (default `select_one`); no mutate-cardinality move.** Decided purely by argument structure. A dedicated cardinality move falls because changing cardinality after positions exist *silently rewrites the attack graph* (`select_one` injects mutual attacks, `open` removes them), flipping labels with no new argument — the same silent reinterpretation the immutability ethos rejects (§5.1, Q4). Its rebuttal — "`issue_card` is already a fact relation and the bitemporal `log` audits the change" — is defeated: the log records *that* it changed but not *why*, and the current labelling flips for everyone with no attached argument, exactly the Q4 defect. The objection to a creation-only flag (a mis-framed issue can't be fixed without re-raising) is reinstated against: re-framing exclusive-vs-independent choices *should* be a new issue, since the positions mean different things under each reading, so the cost is appropriate. Implemented: `raise --card` (validated, default `select_one`), surfaced in `discover`.

*(The §8.2 capability contract — `discover` — now also publishes `globals` (shared flags), `errors` (exit-code catalog), `error_envelope`, and per-read `output` envelopes, so an agent can drive the tool without sampling output shapes by trial. Contract version `0.3.0`.)*
