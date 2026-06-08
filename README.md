# dlktk — defeasible-argumentation tool for design dialectics

A lightweight, git-native CLI that records design discussions as an [IBIS](https://en.wikipedia.org/wiki/Issue-based_information_system) graph, evaluates *what currently stands* via Dung grounded semantics over a defeat relation, and exposes the whole thing through a dual human/agent interface. Storage is [pudl](https://github.com/chazu/pudl)'s bitemporal fact store.

> Status: early but functional. Design phases 1–4 implemented (MVP, agent surface, replay/postmortem, git-native export/import). See [`dlktk-design.md`](dlktk-design.md) for the full design and [`pudl-rename-plan.md`](pudl-rename-plan.md) for the storage prerequisite.

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

Agent commands: `discover` (CUE/JSON capability schema), `agenda` (live UNDEC set), `moves <issue>` (legal next moves), `why <node>` (label explanation + how to flip it). Structured JSON errors with stable exit codes (`2` illegal, `3` not-found, `4` store).

Postmortem: `replay <issue> --as-of T [--diff]` (labelling as it stood at T, and what changed since), `--valid-at T` (which decisions were in force), `log [node]` (transaction-time audit trail). Git-native: `export` (NDJSON move log) / `import` (idempotent by content), `schema` (the `pudl/dlktk` CUE), `anchored <ref>` (discussions governing a code artifact).

## Build

dlktk depends on pudl. During co-development `go.mod` uses a local `replace github.com/chazu/pudl => ../pudl`, so clone both side by side:

```
git clone https://github.com/chazu/pudl
git clone https://github.com/chazu/dlktk
cd dlktk && go build ./...
```

## License

TBD.
