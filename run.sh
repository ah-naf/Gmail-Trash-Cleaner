#!/bin/bash
# Picks the platform folder by OS.
#   ./run.sh           run one export+cleanup now
#   ./run.sh install   Linux only: install the systemd timer
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

case "$(uname -s)" in
  Darwin) PLATFORM=launchd ;;
  Linux)  PLATFORM=systemd ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

if [ "${1:-}" = install ]; then
  [ "$PLATFORM" = systemd ] || { echo "macOS install: see README (launchd)" >&2; exit 1; }
  exec "$DIR/systemd/setup.sh"
fi

exec "$DIR/$PLATFORM/run-cleaner.sh"
