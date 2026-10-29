#!/bin/bash
# Exports Gmail Trash into a folder named after the time window it covers
# (previous run -> this run), then empties Trash. Stops if the export fails.
set -euo pipefail

: "${GMAIL_USER:?GMAIL_USER must be set in the environment}"
export GMAIL_USER
GMAIL_APP_PASSWORD="$(security find-generic-password -a "$GMAIL_USER" -s gmail-trash-cleaner -w)"
export GMAIL_APP_PASSWORD

BIN="$HOME/bin/gmail-trash-cleaner"
ROOT="$HOME/gmail-trash-backup"
STATE="$ROOT/.last-run"
FMT="%Y-%m-%d_%H%M%S"

mkdir -p "$ROOT"
NOW="$(date +$FMT)"
FROM="$(cat "$STATE" 2>/dev/null || echo first-run)"
OUT="$ROOT/${FROM}_to_${NOW}"

echo "=== $(date) window: ${FROM} -> ${NOW} ==="
"$BIN" -export "$OUT" -attachments
"$BIN" -yes

# Advance the window only after a fully successful run.
echo "$NOW" > "$STATE"
