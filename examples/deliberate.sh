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
# Force the human view for the display steps: output is piped here, so without
# --format text the reads would auto-switch to JSON; --color always keeps the
# formatted/colored output visible in CI logs (and a terminal).
VIEW="--format text --color always"
say() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
move() { # move <role> <verb> [args...] -> node id on stdout
  # Each persona is a distinct agent: --author is its stable identity (and what
  # ownership checks ride on), --role its persona. The first move under a role
  # auto-records the author↔role roster binding.
  local role=$1; shift
  "$DLKTK" "$@" -d "$DISC" --store "$STORE" --author "$role-bot" --role "$role" --format json | python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))'
}

say "shipper opens the discussion"
DISC=$("$DLKTK" new "cache lock choice" --subject "q:lock for the read cache" --store "$STORE" --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
ISSUE=$(move shipper raise "which lock for the read cache?")

say "agenda: the issue has no positions yet"
"$DLKTK" agenda -d "$DISC" --store "$STORE" $VIEW

say "shipper and maintainer propose rival positions"
MUTEX=$(move shipper propose "$ISSUE" "plain mutex — simplest thing that works")
RWLOCK=$(move maintainer propose "$ISSUE" "RWLock — readers should not serialize")

say "status: a symmetric tie — the engine says so"
"$DLKTK" status "$ISSUE" -d "$DISC" --store "$STORE" $VIEW

say "first-principles names the premise everything rests on (assume)"
ASSUME=$(move first-principles assume "$RWLOCK" "the cache stays read-heavy as traffic grows")

say "agenda: the unexamined assumption is on the worklist"
"$DLKTK" agenda -d "$DISC" --store "$STORE" $VIEW

say "security checks for prior art before arguing (search), then objects"
"$DLKTK" search "starvation" -d "$DISC" --store "$STORE" $VIEW
STARVE=$(move security object "$RWLOCK" "writer starvation under sustained read load")

say "maintainer rebuts the objection (defend by counter-attack)"
move maintainer object "$STARVE" "cache is 99% reads; starvation cannot occur" >/dev/null

say "worlds: the coherent stances the deadlock admits (exploration, not verdict)"
"$DLKTK" worlds "$ISSUE" -d "$DISC" --store "$STORE" $VIEW

say "whatif: probe a preference before committing to it (nothing written)"
"$DLKTK" whatif "$ISSUE" --prefer "$RWLOCK:$MUTEX" -d "$DISC" --store "$STORE" $VIEW

say "moves: what the engine considers useful next (note the generative exits)"
"$DLKTK" moves "$ISSUE" -d "$DISC" --store "$STORE" $VIEW

say "shipper tries the synthesis exit: a hybrid with recorded lineage"
HYBRID=$(move shipper synthesize "$ISSUE" "RWLock with a writer-priority escape hatch" --from "$MUTEX" --from "$RWLOCK")

say "the hybrid joins the rivalry — preferences (honest bases) settle the 3-way tie"
move maintainer prefer "$RWLOCK" "$HYBRID" --basis simplicity >/dev/null
move shipper prefer "$RWLOCK" "$MUTEX" --basis throughput >/dev/null

say "crux: which argument does the verdict actually rest on?"
"$DLKTK" crux "$ISSUE" -d "$DISC" --store "$STORE" $VIEW

say "explain: the full derivation that settled it"
"$DLKTK" explain "$ISSUE" --brief -d "$DISC" --store "$STORE" $VIEW

say "agenda is ready -> decide (with a review horizon), then verify nothing drifted"
"$DLKTK" agenda -d "$DISC" --store "$STORE" $VIEW
REVIEW=$(python3 -c 'import time; print(int(time.time()) + 180*24*3600)')
"$DLKTK" decide "$ISSUE" "$RWLOCK" --basis throughput --review-by "$REVIEW" -d "$DISC" --store "$STORE" --author shipper-bot --role shipper
"$DLKTK" check -d "$DISC" --store "$STORE" $VIEW

say "the roster: who argued under which persona (auto-recorded from the moves)"
"$DLKTK" roster -d "$DISC" --store "$STORE" $VIEW

say "the git-native record (commit this)"
RECORD="$STORE/dialectic.ndjson"
"$DLKTK" export -d "$DISC" --store "$STORE" > "$RECORD"
head -3 "$RECORD"
echo "... ($(wc -l < "$RECORD") facts total, written to $RECORD)"
