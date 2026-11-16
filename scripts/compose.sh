#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ ! -f "${ROOT_DIR}/apps/.env" ]]; then
  echo "Run ./scripts/setup.sh first." >&2
  exit 1
fi
DOCKER_ARGS=()
if [[ -n "${GCFEED_DOCKER_CONTEXT:-}" ]]; then
  DOCKER_ARGS=(--context "${GCFEED_DOCKER_CONTEXT}")
elif docker context inspect colima-gcfeed >/dev/null 2>&1; then
  DOCKER_ARGS=(--context colima-gcfeed)
fi
exec docker "${DOCKER_ARGS[@]}" compose --project-directory "${ROOT_DIR}/apps" --env-file "${ROOT_DIR}/apps/.env" -f "${ROOT_DIR}/apps/docker-compose.yml" "$@"
