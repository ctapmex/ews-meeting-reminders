#!/usr/bin/env bash
# Build static binaries and install as user systemd service (no Docker).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
SHARE="${XDG_DATA_HOME:-$HOME/.local/share}/ews-meeting-reminders"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ews-meeting-reminders"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

mkdir -p "$SHARE" "$CONFIG_DIR" "$UNIT_DIR"
cd "$ROOT"
make build

install -m 0755 "$ROOT/bin/ews-reminders" "$SHARE/ews-reminders"

if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then
  install -m 0600 "$ROOT/config.example.yaml" "$CONFIG_DIR/config.yaml"
  echo "Created $CONFIG_DIR/config.yaml — edit credentials, then:"
  echo "  echo 'EWS_PASSWORD=...' > $CONFIG_DIR/env && chmod 600 $CONFIG_DIR/env"
fi

install -m 0644 "$ROOT/systemd/ews-meeting-reminders.service" \
  "$UNIT_DIR/ews-meeting-reminders.service"
systemctl --user daemon-reload
systemctl --user enable --now ews-meeting-reminders.service

echo "OK: $SHARE/ews-reminders"
echo "logs: journalctl --user -u ews-meeting-reminders.service -f"
