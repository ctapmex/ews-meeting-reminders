#!/usr/bin/env bash
# Uninstall user service and binary; keep config (~/.config/ews-meeting-reminders).
set -euo pipefail

SHARE="${XDG_DATA_HOME:-$HOME/.local/share}/ews-meeting-reminders"
STATE="${XDG_STATE_HOME:-$HOME/.local/state}/ews-meeting-reminders"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ews-meeting-reminders"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
SERVICE="ews-meeting-reminders.service"

if command -v systemctl >/dev/null 2>&1; then
  systemctl --user stop "$SERVICE" >/dev/null 2>&1 || true
  systemctl --user disable "$SERVICE" >/dev/null 2>&1 || true
fi

rm -f "$UNIT_DIR/$SERVICE"
rm -rf "$SHARE" "$STATE"

if command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload >/dev/null 2>&1 || true
fi

echo "OK: removed binary, state, and user unit"
echo "kept: $CONFIG_DIR"
