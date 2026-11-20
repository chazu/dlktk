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

## Quickstart

Run `command -v dlktk` to see which executable your shell selects. Run `dlktk discover --format json | jq -r .version` to see its contract version. From the checkout, compare it with `GOWORK=off go run ./cmd/dlktk discover --format json | jq -r .version`. To use the checkout, run `GOWORK=off make build` and set `DLKTK="$PWD/dlktk"` before starting the example.

This Bash example uses jq and a temporary store. It checks that RWLock becomes the only justified position.

```bash
(
  set -euo pipefail
  binary=${DLKTK:-$(command -v dlktk)}
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT
  cd "$work"
  run() { "$binary" --store "$work/store" --format json "$@"; }
  disc=$(run new "lock choice" | jq -er .id)
  move() { run -d "$disc" "$@"; }
  issue=$(move raise "which lock?" | jq -er .id)
  mutex=$(move propose "$issue" "mutex" --author ann | jq -er .id)
  rwlock=$(move propose "$issue" "RWLock" --author bob | jq -er .id)
  objection=$(move object "$rwlock" "a writer can starve" --author ann | jq -er .id)
  move object "$objection" "this lock queues a waiting writer ahead of new readers" --author bob
  move prefer "$rwlock" "$mutex" --basis throughput
  move status "$issue" | jq -e --arg id "$rwlock" \
    '[.[0].positions[] | select(.label == "IN") | .id] == [$id]'
)
```

Agent commands: `discover` (CUE/JSON capability schema), `agenda` (the worklist: UNDEC set, ready-to-decide, untested winners, unexamined assumptions), `moves <issue>` (legal next moves), `why <node>` (label explanation + how to flip it), `explain <issue>` (full derivation: how attacks/defeats were built and the round-by-round grounded fixpoint that produced the labelling). Structured JSON errors with stable exit codes (`2` illegal, `3` not-found, `4` store, `5` check-failed).

### Find an existing claim

Plain `dlktk search "writer starvation"` matches a case-insensitive substring in the current discussion. To look for the same claim in different words, use contract 0.20.0 or later:

```sh
dlktk search "A writer can wait forever when readers keep arriving" --semantic
```

Set `OPENROUTER_API_KEY` for the CLI or MCP server before using semantic search. It sends the query and up to eight candidate IDs and texts to Jev on OpenRouter. It returns up to three original nodes, each with its discussion ID and text. A missing candidate or a model choice of “no match” can yield an empty result. Inspect the returned text before treating claims as duplicates. Plain search works offline.

Add `--all` to search across discussions. In MCP, call `search` with `query`, `semantic: true`, and an optional `discussion` ID. Omitting `discussion` searches all discussions. If the provider is unavailable, retry without the semantic option for local substring search.

### Read and restore decisions

On an open issue, `dlktk show <issue>` lists every adopted position and its basis. CLI JSON and MCP return these adoptions in `decisions`. Superseding one adoption updates only that position. For a single winner or an audience map, `show` returns `decided`. A map has `kind: "map"` and represents the outcome under each audience's ranking.

When two hybrids inherit the same objection, answer it on each hybrid where it applies. An answer on one hybrid closes only that hybrid's question. If the answer is conceded, the question reopens there. Inspect `inherited_questions` with `show` or `why`.

To inspect the saved audience map in a fresh store, run these commands from the checkout:

```sh
store=$(mktemp -d)
dlktk import examples/make-prorgamming-fun-again/dialectic.ndjson --store "$store"
dlktk show fojas-pipit -d fivij-zatad --store "$store" --format json
dlktk audiences -d fivij-zatad --store "$store"
```

Both snapshot and history imports preserve the map decision, basis, and review horizon. Repeated imports of the same export are idempotent.

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

Use `--ack untested_decision --basis "release owner accepts the review risk"` on `decide` or `supersede` to acknowledge an untested adoption. Repeat `--ack` for each warning. Include the node for a finding on another node, such as `--ack defeated_assumption:<premise-id>`. MCP accepts the same keys in an `ack` array. The rationale applies to this decision's acknowledgments.

`show` and `status` expose the warning keys present at closure and the acknowledged subset in their JSON decision records. A later warning still appears in `check`. Only a warning present at closure can trigger a missing acknowledgment on a new decision. Exports carry closure warnings alongside the decision arguments. The store keeps them in the fact provenance. For older records, the checker looks for warning-specific words or the affected node in that decision's basis. It reports the original closure state as unknown.

Multi-agent identity: `--author <id>` is the stable identity attributed to a move and the thing `concede`/`retract` ownership is checked against; `--role <persona>` is the optional hat a move is argued under and is kept distinct from identity (two agents sharing a persona still own only their own nodes). The first move under a role auto-records an author↔role binding; `roster` lists the bindings, `roster <author> <role>` pre-declares one (design §16 Q8).

Postmortem: `replay <issue> --as-of T [--diff]` (labelling as it stood at T, and what changed since), `--valid-at T` (which decisions were in force), `log [node]` (transaction-time audit trail). Git-native: `export` (NDJSON move log; `--history` adds retract/invalidate events so the full audit trail replays) / `import` (validated, idempotent by content), `schema` (the `pudl/dlktk` CUE; built into pudl ≥ v0.1.3), `anchored <ref>` (discussions governing a code artifact).

Claude skill: [`skills/dlktk-dialectic/SKILL.md`](skills/dlktk-dialectic/SKILL.md) is a drop-in Claude Code skill teaching an agent how to conduct a dialectic — the loop, move discipline, and the author/role/roster mechanics for multi-persona debates. Copy it into `.claude/skills/` (or a plugin) to install.

MCP: `dlktk mcp` serves the same verb set over the Model Context Protocol (stdio) — one tool per move/read, returning the identical JSON envelopes — so any MCP-capable agent harness can drive a dialectic without shelling out. The CLI and MCP server use store transactions to check move legality and write facts atomically. Example client config:

```json
{"mcpServers": {"dlktk": {"command": "dlktk", "args": ["mcp", "--store", "/path/to/repo/.pudl"]}}}
```

## Verification and benchmarks

CI restores each tracked example snapshot into a separate temporary store and checks the expected decisions and findings. A fixed review date keeps historical warning expectations reproducible. CI also runs the reference deliberation and the Bash quickstart above.

To review benchmark replies locally, run `python3 benchmarks/objection-review/review.py` and open the URL it prints. The page saves labels and optional reasons and resumes at the first unanswered case. Editing a completed case updates the saved human judgment. The benchmark guide includes the command for rescoring saved predictions. [Objection review guide and results](benchmarks/objection-review/README.md)

The duplicate-search trial qualified Jev for opt-in retrieval. The objection reviewer and automatic framing prompt were deferred after evaluation. The search and framing report includes the test cases, limits, and reproduction commands. [Search and framing results](benchmarks/search-framing/README.md)

The witness pilot compares one agent with and without dlktk on ten published testimony puzzles. The paired results include final verdicts, captured attack graphs, and disagreement diagnoses. [Witness pilot](benchmarks/witness/README.md)

The witness extension scores 20 fresh pairs by graph family. The offline scorer and optional DeepEval report measure graph capture, repeated attacks, and agreement with the tool verdict. [Extension protocol](benchmarks/witness/EXTENSION.md) · [Scoring commands](benchmarks/witness/README.md#paired-diagnostics-and-optional-deepeval-report)

## Build

dlktk pins a `github.com/chazu/pudl` revision in `go.mod`, so a plain clone builds reproducibly with no sibling checkout:

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
go build ./...        # uses ../pudl; GOWORK=off builds the pinned revision
```

Bump the pin deliberately with `go get github.com/chazu/pudl@latest` (design §16 Q1).

## License

TBD.
