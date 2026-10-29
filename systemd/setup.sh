#!/bin/bash
# Builds the binary, stores the app password in the keyring, and installs a
# systemd user timer that runs systemd/run-cleaner.sh. Code stays where it is.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
UNITS="$HOME/.config/systemd/user"

: "${GMAIL_USER:?export GMAIL_USER first}"

mkdir -p "$HOME/bin" "$UNITS"
(cd "$REPO" && go build -o "$HOME/bin/gmail-trash-cleaner" .)

# Keyring if usable, else a chmod 600 password file.
if ! { command -v secret-tool >/dev/null && secret-tool store --label=gmail-trash-cleaner service gmail-trash-cleaner account "$GMAIL_USER"; }; then
  echo "no usable keyring, using password file"
  PW_DIR="$HOME/.config/gmail-trash-cleaner"
  mkdir -p "$PW_DIR"
  read -rsp "Gmail app password: " PW; echo
  (umask 077; printf '%s' "$PW" > "$PW_DIR/password")
fi

cat > "$UNITS/gmail-trash-cleaner.service" <<UNIT
[Unit]
Description=Gmail Trash export and cleanup

[Service]
Type=oneshot
Environment=GMAIL_USER=$GMAIL_USER
ExecStart=$REPO/systemd/run-cleaner.sh
UNIT

cat > "$UNITS/gmail-trash-cleaner.timer" <<UNIT
[Unit]
Description=Run gmail-trash-cleaner every 2 minutes

[Timer]
OnBootSec=1min
OnUnitActiveSec=2min

[Install]
WantedBy=timers.target
UNIT

systemctl --user daemon-reload
systemctl --user enable --now gmail-trash-cleaner.timer
echo "installed. logs: journalctl --user -u gmail-trash-cleaner"
