#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if command -v promtool >/dev/null; then
  cd "$DIR"
  promtool check rules alerts.yml
  promtool test rules alerts.test.yml
else
  # Use the same engine/version as Compose; the explicit context is optional in CI.
  DOCKER=(docker)
  if [[ -n "${GCFEED_DOCKER_CONTEXT:-}" ]]; then DOCKER+=(--context "$GCFEED_DOCKER_CONTEXT"); fi
  "${DOCKER[@]}" run --rm --entrypoint /bin/promtool -v "$DIR:/rules:ro" -w /rules prom/prometheus:v3.16.0 check rules alerts.yml
  "${DOCKER[@]}" run --rm --entrypoint /bin/promtool -v "$DIR:/rules:ro" -w /rules prom/prometheus:v3.16.0 test rules alerts.test.yml
fi
