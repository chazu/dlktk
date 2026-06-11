#!/usr/bin/env bash
# deliberate.sh — reference harness mechanics, end to end.
#
# Demonstrates the multi-persona deliberation loop from AGENTS.md with the
# personas scripted (no LLM required): each persona reads the worklist, makes
# one move, and the grounded labelling referees. The same shape works with
# real agents generating the moves — only the "think" step changes.
#
# Usage: ./examples/deliberate.sh [store-dir]   (default: a temp dir)
set -euo pipefail

DLKTK=${DLKTK:-dlktk}
STORE=${1:-$(mktemp -d)}
say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
move() { # move <role> <verb> [args...] -> node id on stdout
  local role=$1; shift
  "$DLKTK" "$@" -d "$DISC" --store "$STORE" --role "$role" --format json | python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))'
}

say "shipper opens the discussion"
DISC=$("$DLKTK" new "cache lock choice" --subject "q:lock for the read cache" --store "$STORE" --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
ISSUE=$(move shipper raise "which lock for the read cache?")

say "agenda: the issue has no positions yet"
"$DLKTK" agenda -d "$DISC" --store "$STORE"

say "shipper and maintainer propose rival positions"
MUTEX=$(move shipper propose "$ISSUE" "plain mutex — simplest thing that works")
RWLOCK=$(move maintainer propose "$ISSUE" "RWLock — readers should not serialize")

say "status: a symmetric tie — the engine says so"
"$DLKTK" status "$ISSUE" -d "$DISC" --store "$STORE"

say "security checks for prior art before arguing (search), then objects"
"$DLKTK" search "starvation" -d "$DISC" --store "$STORE"
STARVE=$(move security object "$RWLOCK" "writer starvation under sustained read load")

say "maintainer rebuts the objection (defend by counter-attack)"
move maintainer object "$STARVE" "cache is 99% reads; starvation cannot occur" >/dev/null

say "moves: what the engine considers useful next"
"$DLKTK" moves "$ISSUE" -d "$DISC" --store "$STORE"

say "stalemate -> a preference, with an honest basis"
move shipper prefer "$RWLOCK" "$MUTEX" --basis throughput >/dev/null

say "explain: the full derivation that settled it"
"$DLKTK" explain "$ISSUE" --brief -d "$DISC" --store "$STORE"

say "agenda is ready -> decide, then verify nothing drifted"
"$DLKTK" agenda -d "$DISC" --store "$STORE"
"$DLKTK" decide "$ISSUE" "$RWLOCK" --basis throughput -d "$DISC" --store "$STORE" --role shipper
"$DLKTK" check -d "$DISC" --store "$STORE"

say "the git-native record (commit this)"
RECORD="$STORE/dialectic.ndjson"
"$DLKTK" export -d "$DISC" --store "$STORE" > "$RECORD"
head -3 "$RECORD"
echo "... ($(wc -l < "$RECORD") facts total, written to $RECORD)"
