#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "${GCFEED_REQUIRE_INTEGRATION:-0}" == 1 ]]; then
  : "${GCFEED_TEST_MYSQL_ADMIN_DSN:?Set an isolated-test MySQL admin DSN}"
  : "${GCFEED_TEST_RABBITMQ_URL:?Set the test RabbitMQ URL}"
  command -v redis-server >/dev/null || { echo "redis-server is required for isolated cache tests" >&2; exit 1; }
fi
(cd "${ROOT_DIR}/apps/api" && go test -race ./... && go vet ./... && go mod verify)
npm --prefix "${ROOT_DIR}/apps/web" test
npm --prefix "${ROOT_DIR}/apps/web" run test:ui
npm --prefix "${ROOT_DIR}/apps/web" run build
python3 -m py_compile "${ROOT_DIR}/scripts/backup.py" "${ROOT_DIR}/apps/monitoring/drill.py"
python3 -m unittest discover -s "${ROOT_DIR}/scripts" -p 'backup_test.py'
