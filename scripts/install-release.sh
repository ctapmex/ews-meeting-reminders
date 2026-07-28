#!/usr/bin/env bash
# Install from a release archive (no Go toolchain required).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
SHARE="${XDG_DATA_HOME:-$HOME/.local/share}/ews-meeting-reminders"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ews-meeting-reminders"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
SERVICE="ews-meeting-reminders.service"

BIN_SRC="$ROOT/ews-reminders"
CFG_SRC="$ROOT/config.example.yaml"
UNIT_SRC="$ROOT/$SERVICE"

if [[ ! -x "$BIN_SRC" ]]; then
  echo "ERROR: $BIN_SRC not found or not executable" >&2
  exit 1
fi
if [[ ! -f "$CFG_SRC" ]]; then
  echo "ERROR: $CFG_SRC not found" >&2
  exit 1
fi
if [[ ! -f "$UNIT_SRC" ]]; then
  echo "ERROR: $UNIT_SRC not found" >&2
  exit 1
fi

mkdir -p "$SHARE" "$CONFIG_DIR" "$UNIT_DIR"

# Stop service before replacing binary to avoid "text file busy" errors.
if command -v systemctl >/dev/null 2>&1; then
  systemctl --user stop "$SERVICE" >/dev/null 2>&1 || true
fi

install -m 0755 "$BIN_SRC" "$SHARE/ews-reminders"

if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then
  install -m 0600 "$CFG_SRC" "$CONFIG_DIR/config.yaml"
  echo "Created $CONFIG_DIR/config.yaml — edit credentials, then:"
  echo "  echo 'EWS_PASSWORD=...' > $CONFIG_DIR/env && chmod 600 $CONFIG_DIR/env"
fi

install -m 0644 "$UNIT_SRC" "$UNIT_DIR/$SERVICE"
systemctl --user daemon-reload
systemctl --user enable --now "$SERVICE"

echo "OK: $SHARE/ews-reminders"
echo "logs: journalctl --user -u $SERVICE -f"
