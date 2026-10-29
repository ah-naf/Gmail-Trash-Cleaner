#!/bin/bash
# Linux counterpart of launchd/run-cleaner.sh: exports Gmail Trash into a folder
# named after the time window it covers, then empties Trash. Stops if export fails.
set -euo pipefail

: "${GMAIL_USER:?GMAIL_USER must be set in the environment}"
export GMAIL_USER
PW_FILE="$HOME/.config/gmail-trash-cleaner/password"
# Keyring first; fall back to a chmod 600 file (WSL/headless have no keyring).
GMAIL_APP_PASSWORD="$(secret-tool lookup service gmail-trash-cleaner account "$GMAIL_USER" 2>/dev/null || cat "$PW_FILE")"
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
