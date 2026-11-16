#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="${ROOT_DIR}/apps/api"
WEB_DIR="${ROOT_DIR}/apps/web"
if [[ ! -f "${ROOT_DIR}/apps/.env" || ! -d "${WEB_DIR}/node_modules" ]]; then
  "${ROOT_DIR}/scripts/setup.sh"
fi
command -v ffmpeg >/dev/null || { echo "Install FFmpeg first (macOS: brew install ffmpeg)." >&2; exit 1; }
python3 - <<'PY'
import socket
for port in (18080, 15173, 19091):
    with socket.socket() as sock:
        # 与 Go HTTP listener 一致，允许重用 TIME_WAIT 端口；真实监听仍会阻止 bind。
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind(('127.0.0.1', port))
        except OSError:
            raise SystemExit(f'Port {port} is occupied. Stop the existing GCFeed application first.')
PY
"${ROOT_DIR}/scripts/compose.sh" up -d --wait mysql redis rabbitmq
set -a
source "${ROOT_DIR}/apps/.env"
set +a
export GCFEED_PORT=18080
export GCFEED_DATABASE_HOST=127.0.0.1 GCFEED_DATABASE_PORT=13307
export GCFEED_DATABASE_USER=gcfeed GCFEED_DATABASE_PASSWORD="${GCFEED_DB_PASSWORD}"
export GCFEED_REDIS_ADDR=127.0.0.1:16379
export GCFEED_RABBITMQ_URL="amqp://gcfeed:${GCFEED_MQ_PASSWORD}@127.0.0.1:15673/"
export GCFEED_WORKER_METRICS_ADDR=127.0.0.1:19091
export GCFEED_WEB_PORT=15173 GCFEED_API_PROXY=http://127.0.0.1:18080
mkdir -p "${ROOT_DIR}/.local"
(cd "${API_DIR}" && go build -o "${ROOT_DIR}/.local/gcfeed-api" ./cmd/feed && go build -o "${ROOT_DIR}/.local/gcfeed-worker" ./cmd/worker)
PIDS=()
echo "$$" > "${ROOT_DIR}/.local/dev.pid"
cleanup() {
  local code=$?
  trap - EXIT INT TERM
  for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "$pid" 2>/dev/null || true; done
  rm -f "${ROOT_DIR}/.local/dev.pid"
  exit "$code"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
(cd "${API_DIR}" && exec "${ROOT_DIR}/.local/gcfeed-api") &
PIDS+=("$!")
python3 - <<'PY'
import time, urllib.request
for _ in range(60):
    try:
        with urllib.request.urlopen('http://127.0.0.1:18080/ready', timeout=1) as r:
            if r.status == 200:
                break
    except OSError:
        pass
    time.sleep(0.5)
else:
    raise SystemExit('API dependencies did not become ready within 30 seconds.')
PY
(cd "${API_DIR}" && exec "${ROOT_DIR}/.local/gcfeed-worker") &
PIDS+=("$!")
(cd "${WEB_DIR}" && exec node node_modules/vite/bin/vite.js) &
PIDS+=("$!")
echo "GCFeed: http://127.0.0.1:15173 | API: http://127.0.0.1:18080"
echo "Ctrl+C stops API, worker and web; infrastructure keeps running."
while true; do
  for pid in "${PIDS[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid"
      exit 1
    fi
  done
  sleep 1
done
