#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  tools/dev/agent/create-oauth-app.sh --root PATH --pg-dsn DSN --name NAME [options]

Options:
  --root PATH            BorealValley root directory (required)
  --pg-dsn DSN           PostgreSQL DSN (required)
  --name NAME            OAuth app display name (required)
  --description TEXT     OAuth app description (default: "BorealValley agent app")
  --redirect-uri URI     OAuth redirect URI (default: http://127.0.0.1:8787/callback)
  --verbosity N          Log verbosity for ctl (default: 3)
  -h, --help             Show this help

Environment:
  CTL_BIN                Optional path to BorealValley-ctl binary.
                         If unset, runs: go run ./src/cmd/ctl

Notes:
  - Scopes are fixed to: profile:read, ticket:read, ticket:write
  - The command prints client_id and client_secret (secret shown once).
USAGE
}

ROOT_DIR=""
PG_DSN=""
APP_NAME=""
DESCRIPTION="BorealValley agent app"
REDIRECT_URI="http://127.0.0.1:8787/callback"
VERBOSITY="3"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      ROOT_DIR="${2:-}"
      shift 2
      ;;
    --pg-dsn)
      PG_DSN="${2:-}"
      shift 2
      ;;
    --name)
      APP_NAME="${2:-}"
      shift 2
      ;;
    --description)
      DESCRIPTION="${2:-}"
      shift 2
      ;;
    --redirect-uri)
      REDIRECT_URI="${2:-}"
      shift 2
      ;;
    --verbosity)
      VERBOSITY="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$ROOT_DIR" || -z "$PG_DSN" || -z "$APP_NAME" ]]; then
  echo "error: --root, --pg-dsn, and --name are required" >&2
  usage >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

run_ctl() {
  if [[ -n "${CTL_BIN:-}" ]]; then
    "$CTL_BIN" "$@"
    return
  fi
  (
    cd "$REPO_ROOT"
    go run ./src/cmd/ctl "$@"
  )
}

run_ctl oauth-app create \
  --root "$ROOT_DIR" \
  --pg-dsn "$PG_DSN" \
  --verbosity "$VERBOSITY" \
  --name "$APP_NAME" \
  --description "$DESCRIPTION" \
  --redirect-uri "$REDIRECT_URI" \
  --scope profile:read \
  --scope ticket:read \
  --scope ticket:write
