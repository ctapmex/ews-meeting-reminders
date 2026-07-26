#!/usr/bin/env bash
# Build static binary inside Docker and write it to ./bin/ews-reminders.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OUT="${1:-$ROOT/bin}"

mkdir -p "$OUT"
docker build \
  --build-arg "VERSION=${VERSION}" \
  --build-arg "COMMIT=${COMMIT}" \
  --build-arg "BUILD_TIME=${BUILD_TIME}" \
  --target export \
  --output "type=local,dest=${OUT}" \
  .

echo "OK: ${OUT}/ews-reminders"
"${OUT}/ews-reminders" -version
