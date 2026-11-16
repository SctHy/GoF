#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for tool in go node npm python3; do
  command -v "$tool" >/dev/null || { echo "Missing tool: $tool" >&2; exit 1; }
done
node -e 'if (+process.versions.node.split(".")[0] !== 26) { console.error("Use Node 26 LTS (see .nvmrc)"); process.exit(1); }'
python3 - "${ROOT_DIR}" <<'PY'
from pathlib import Path
import secrets
import sys
root = Path(sys.argv[1])
env = root / 'apps/.env'
if not env.exists():
    defaults = dict(line.split('=', 1) for line in (root / 'apps/.env.example').read_text().splitlines() if line and not line.startswith('#'))
    with env.open('x') as out:
        env.chmod(0o600)
        for key, default in defaults.items():
            value = secrets.token_hex(32) if default == "generated-by-setup" else default
            out.write(f'{key}={value}\n')
    print('Created apps/.env (local credentials; ignored by Git).')
else:
    print('Keeping existing apps/.env and credentials.')
(root / '.local').mkdir(exist_ok=True)
PY
(cd "${ROOT_DIR}/apps/api" && go mod download)
npm --prefix "${ROOT_DIR}/apps/web" ci
echo "Setup complete. Start with ./scripts/start.sh or ./scripts/compose.sh up -d --build --wait"
