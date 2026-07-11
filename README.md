# dlktk — defeasible-argumentation tool for design dialectics

A lightweight, git-native CLI that records design discussions as an [IBIS](https://en.wikipedia.org/wiki/Issue-based_information_system) graph, evaluates *what currently stands* via Dung grounded semantics over a defeat relation, and exposes the whole thing through a dual human/agent interface. Storage is [pudl](https://github.com/chazu/pudl)'s bitemporal fact store.

> New here? Read [`INTRODUCTION.md`](INTRODUCTION.md) — a from-zero guide to the ideas (defeasible reasoning, argument graphs, automated inference) and how the tool implements them. No logic background needed.

> Status: early but functional. Design phases 1–4 implemented (MVP, agent surface, replay/postmortem, git-native export/import). See [`dlktk-design.md`](dlktk-design.md) for the full design.

## What it does

Three separable layers, each tiny on its own:

- **Capture** — a typed IBIS graph: `issue` / `position` / `argument` nodes, linked by `responds_to` / `supports` / `objects_to`.
- **Evaluate** — map to a Dung argumentation framework, compute the grounded labelling: every position is `IN` (justified), `OUT` (defeated), or `UNDEC` (genuinely contested — the live agenda). Arguments objecting to other arguments give reinstatement, which a flat pro/con list cannot express.
- **Defeasibility** — `prefer(winner, loser, basis)` turns a tied dispute into a decision: defeat = attack that survives preference.

Every verb is equally drivable by a human at a terminal and by an agent over JSON.

## Example

```
dlktk new "lock choice"
I=$(dlktk raise "which lock?" --format json | jq -r .id)
dlktk propose $I "mutex"
B=$(dlktk propose $I "RWLock" --format json | jq -r .id)
C=$(dlktk object $B "writer starvation" --format json | jq -r .id)
dlktk object $C "workload is read-heavy; starvation won't occur"   # reinstates RWLock
dlktk status $I                 # mutex vs RWLock — tied, UNDEC
dlktk prefer $B $A --basis throughput
dlktk status $I                 # RWLock justified
```

Agent commands: `discover` (CUE/JSON capability schema), `agenda` (the worklist: UNDEC set, ready-to-decide, untested winners, unexamined assumptions), `moves <issue>` (legal next moves), `why <node>` (label explanation + how to flip it), `explain <issue>` (full derivation: how attacks/defeats were built and the round-by-round grounded fixpoint that produced the labelling). Structured JSON errors with stable exit codes (`2` illegal, `3` not-found, `4` store, `5` check-failed).

Wicked-problem support (divergence, exploration, plural values — see [`wicked-problems.md`](wicked-problems.md) for the rationale):

- **Epistemic honesty** — an IN position with no attackers is marked *untested* (IN by silence, not by surviving attack); `agenda` lists them, `moves` suggests stress-testing before `decide`, and `check --strict` flags never-attacked decisions.
- **Generative stalemate exits** — `synthesize <issue> "<text>" --from <p1> --from <p2>` (a hybrid with recorded lineage) and `reframe <issue> "<text>" --basis <label>` (replace a mis-framed question; the dead framing leaves the agenda, lineage recorded). `raise --from <node>` spawns a deeper question from the argument that revealed it.
- **Counterfactual exploration** — `whatif <issue> --object <t> / --prefer <w>:<l> / --without <n>` applies hypothetical moves in memory and shows the label diff (nothing written); `crux <issue>` finds the load-bearing arguments the verdict rests on.
- **`worlds <issue>`** — enumerates the coherent maximal stances (preferred extensions) a contested issue admits; positions sorted robust / contingent / hopeless. Grounded semantics stay the referee; worlds is the exploration lens.
- **Values and audiences** — `--promotes <value>` on moves (or `promote <node> <value>`), `audience <name> <v1> <v2>…` records a stakeholder's strict value ranking; `status --under <name>` evaluates through that lens (value-based AF) and `audiences` reports which positions survive *every* declared ranking vs which hinge on whose values govern.
- **Assumptions** — `assume <target> "<text>"` records a challengeable premise; `agenda` lists unexamined ones and `check --strict` flags decisions resting on a defeated assumption.
- **Review horizons** — `decide/supersede --review-by <T>` records that a decision is provisional; `check` reports it once the horizon passes.

Output adapts to its consumer: text on a terminal, JSON when piped (override with `--format`). Text output is colorized by grounded label and word-wrapped to the terminal (`--color auto|always|never`, honors `NO_COLOR`); `why` and `moves` print their suggestions as runnable `dlktk …` command lines, and read errors add a one-line `hint:`.

Decisions are closing acts: a bare re-`decide` on a decided issue is rejected; overturning one is `supersede <issue> <position> --basis <label>`, which records why and links the prior decision.

Multi-agent identity: `--author <id>` is the stable identity attributed to a move and the thing `concede`/`retract` ownership is checked against; `--role <persona>` is the optional hat a move is argued under and is kept distinct from identity (two agents sharing a persona still own only their own nodes). The first move under a role auto-records an author↔role binding; `roster` lists the bindings, `roster <author> <role>` pre-declares one (design §16 Q8).

Postmortem: `replay <issue> --as-of T [--diff]` (labelling as it stood at T, and what changed since), `--valid-at T` (which decisions were in force), `log [node]` (transaction-time audit trail). Git-native: `export` (NDJSON move log; `--history` adds retract/invalidate events so the full audit trail replays) / `import` (validated, idempotent by content), `schema` (the `pudl/dlktk` CUE; built into pudl ≥ v0.1.3), `anchored <ref>` (discussions governing a code artifact).

Claude skill: [`skills/dlktk-dialectic/SKILL.md`](skills/dlktk-dialectic/SKILL.md) is a drop-in Claude Code skill teaching an agent how to conduct a dialectic — the loop, move discipline, and the author/role/roster mechanics for multi-persona debates. Copy it into `.claude/skills/` (or a plugin) to install.

MCP: `dlktk mcp` serves the same verb set over the Model Context Protocol (stdio) — one tool per move/read, returning the identical JSON envelopes — so any MCP-capable agent harness can drive a dialectic without shelling out. Moves are serialized in-process, which also closes the legality-check/write race concurrent CLI invocations have. Example client config:

```json
{"mcpServers": {"dlktk": {"command": "dlktk", "args": ["mcp", "--store", "/path/to/repo/.pudl"]}}}
```

CI: `check [--all] [--strict]` verifies that recorded decisions still stand — exit 5 when a decided position is no longer justified (the dialectic moved out from under it), when stored preferences are cyclic, or when store invariants are violated; `--strict` also fails on the warnings: lingering stalemates, never-attacked decisions (`untested_decision`), expired review horizons (`review_due`), and decisions resting on defeated assumptions (`defeated_assumption`). Run it in CI so decisions stay living constraints — this repo's own workflow drift-checks the `examples/` dialectics on every PR.

## Build

dlktk pins a tagged `github.com/chazu/pudl` release in `go.mod`, so a plain clone builds reproducibly with no sibling checkout:

```
git clone https://github.com/chazu/dlktk
cd dlktk && go build ./...
```

To co-develop pudl and dlktk together, clone both side by side and use a local (git-ignored) Go workspace — it overrides the pin with your local pudl without touching `go.mod`:

```
git clone https://github.com/chazu/pudl
git clone https://github.com/chazu/dlktk
cd dlktk
cat > go.work <<'EOF'
go 1.25.8
use (
	.
	../pudl
)
EOF
go build ./...        # uses ../pudl; GOWORK=off builds the pinned release
```

Bump the pin deliberately with `go get github.com/chazu/pudl@latest` (design §16 Q1).

## License

TBD.
