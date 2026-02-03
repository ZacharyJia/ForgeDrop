#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SKIP_WEB=0
SKIP_GO=0
DO_INSTALL=0
OUT_BIN="${ROOT_DIR}/bin/forge-drop"

usage() {
  cat <<'EOF'
Usage: scripts/build.sh [options]

Builds forge-drop (Go backend) and the embedded web UI.

Options:
  --skip-web       Skip building web UI (web/dist)
  --skip-go        Skip building Go backend
  --install        Run npm ci (or npm install) before web build
  --out <path>     Output path for Go binary (default: bin/forge-drop)
  -h, --help       Show this help

Examples:
  scripts/build.sh
  scripts/build.sh --install
  scripts/build.sh --skip-web
  scripts/build.sh --out /tmp/forge-drop
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-web) SKIP_WEB=1; shift ;;
    --skip-go) SKIP_GO=1; shift ;;
    --install) DO_INSTALL=1; shift ;;
    --out) OUT_BIN="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
done

cd "$ROOT_DIR"

echo "==> forge-drop build"
echo "root: $ROOT_DIR"

if [[ $SKIP_WEB -eq 0 ]]; then
  echo "==> building web UI"
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm not found. Install Node.js/npm or re-run with --skip-web" >&2
    exit 1
  fi

  if [[ $DO_INSTALL -eq 1 ]]; then
    if [[ -f "${ROOT_DIR}/web/package-lock.json" ]]; then
      (cd "${ROOT_DIR}/web" && npm ci)
    else
      (cd "${ROOT_DIR}/web" && npm install)
    fi
  else
    if [[ ! -d "${ROOT_DIR}/web/node_modules" ]]; then
      echo "web/node_modules missing. Run: scripts/build.sh --install" >&2
      exit 1
    fi
  fi

  (cd "${ROOT_DIR}/web" && npm run build)
fi

if [[ $SKIP_GO -eq 0 ]]; then
  echo "==> building Go binary"
  if ! command -v go >/dev/null 2>&1; then
    echo "go not found. Install Go or re-run with --skip-go" >&2
    exit 1
  fi

  mkdir -p "$(dirname "$OUT_BIN")"
  go build -o "$OUT_BIN" ./cmd/forge-drop
  echo "built: $OUT_BIN"
fi

echo "==> done"
