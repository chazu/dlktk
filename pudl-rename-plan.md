# Phase 0 — pudl module rename (prerequisite for dlktk)

**Status:** planned, NOT executed. Execute once the dlktk design is nailed down.

## Why

dlktk must `require github.com/chazu/pudl` and import `.../pkg/factstore` + `.../pkg/eval`.
pudl currently declares `module pudl` (bare). Go requires the import path to match the
dependency's module declaration, so a bare module name is **un-importable by github path**
— and a `replace` does not fix it (the replaced dir still says `module pudl` → version
mismatch). The module must be renamed.

## Blast radius (audited 2026-06)

| Repo | Coupling | Action |
|------|----------|--------|
| **pudl** | declares `module pudl`; ~all `.go` files import `pudl/...` | rename module + rewrite every internal import |
| **nous** | `nous/go.mod`: `require pudl v0.0.0…` + `replace pudl => ../pudl`; `nous/internal/pudlbridge/bridge.go` imports `pudl/pkg/eval`, `pudl/pkg/factstore` | lockstep update |
| **mu** | **none** — the `"pudl/..."` occurrences are CUE namespace *string literals* in code/tests, not Go imports; `mu/go.mod` has no pudl require | verify-only build, no edits |

## Steps

1. **pudl/go.mod**: `module pudl` → `module github.com/chazu/pudl`.
2. **pudl internal imports**: rewrite every `"pudl/..."` import string → `"github.com/chazu/pudl/..."`
   across `main.go`, `cmd/`, `internal/`, `pkg/`, `test/`. (Mechanical; `gofmt`/`goimports`
   or a scripted `sed` over `.go` files, then `go build ./...` to catch misses.)
   Leave CUE/string-literal `pudl/...` namespaces (e.g. `pudl/core`, `pudl/dlktk`) untouched —
   those are data, not imports.
3. **pudl verify**: `go build ./...`, `go test ./...`, build the binary, `go install`.
4. **nous lockstep**:
   - `nous/go.mod`: `require pudl …` → `require github.com/chazu/pudl …`;
     `replace pudl => ../pudl` → `replace github.com/chazu/pudl => ../pudl` (keep `replace`
     for local co-dev).
   - `nous/internal/pudlbridge/bridge.go`: the two import lines `pudl/pkg/eval`,
     `pudl/pkg/factstore` → `github.com/chazu/pudl/pkg/...`.
   - `go build ./...` in nous.
5. **mu verify**: `go build ./...` in mu — expect green with no edits (confirms the
   string-literal finding).
6. **Commit + push** pudl and nous (two repos). Outward-facing/breaking — confirm before push.

## Decisions to settle before executing

- **Version strategy** (open Q §16.1): after rename, do pudl/nous/dlktk pin tagged
  `github.com/chazu/pudl` releases, or share a local `replace … => ../pudl` during early
  co-development? Recommend `replace` locally now, tag-and-pin once `pkg/factstore` stabilizes.
- **Tag** a `v0.x.y` on pudl post-rename so dlktk has something to `require` even if a
  `replace` shadows it locally.

## Rollback

Pure rename; revert is `git revert` on both repos. No data/schema migration involved — the
`.pudl/` store is untouched by this change.
