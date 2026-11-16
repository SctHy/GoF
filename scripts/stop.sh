#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_FILE="${ROOT_DIR}/.local/dev.pid"
if [[ ! -f "${PID_FILE}" ]]; then
  echo "No GCFeed development process recorded. Containers: ./scripts/compose.sh stop"
  exit 0
fi
pid="$(cat "${PID_FILE}")"
if [[ ! "$pid" =~ ^[0-9]+$ ]]; then
  echo "Invalid PID file." >&2
  exit 1
fi
command_line="$(ps -p "$pid" -o command= 2>/dev/null || true)"
case "$command_line" in
  *scripts/start.sh*)
    kill -TERM "$pid"
    echo "Stopping GCFeed development processes; data services stay running."
    for ((attempt=0; attempt<200; attempt++)); do
      if ! kill -0 "$pid" 2>/dev/null; then exit 0; fi
      sleep 0.1
    done
    echo "Development processes have not exited within 20 seconds; inspect .local/dev.log before restarting." >&2
    exit 1
    ;;
  *) echo "Recorded process has exited; no process was stopped." ;;
esac
