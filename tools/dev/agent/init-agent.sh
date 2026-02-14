#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  tools/dev/agent/init-agent.sh --server-url URL --client-id ID --client-secret SECRET --model NAME [options]

Options:
  --server-url URL       BorealValley server base URL (required)
  --client-id ID         OAuth client id (required)
  --client-secret SECRET OAuth client secret (required)
  --model NAME           LM Studio model name (required)
  --redirect-uri URI     OAuth redirect URI (default: http://127.0.0.1:8787/callback)
  --lmstudio-url URL     LM Studio API URL (default: http://127.0.0.1:1234)
  --state-file PATH      Optional explicit state file path
  --reuse-session        Reuse the current browser login instead of forcing a fresh login
  --verbosity N          Log verbosity for agent (default: 3)
  -h, --help             Show this help

Environment:
  AGENT_BIN              Optional path to BorealValley-agent binary.
                         If unset, runs: go run ./src/cmd/agent
USAGE
}

SERVER_URL=""
CLIENT_ID=""
CLIENT_SECRET=""
MODEL_NAME=""
REDIRECT_URI="http://127.0.0.1:8787/callback"
LMSTUDIO_URL="http://127.0.0.1:1234"
STATE_FILE=""
REUSE_SESSION=0
VERBOSITY="3"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-url)
      SERVER_URL="${2:-}"
      shift 2
      ;;
    --client-id)
      CLIENT_ID="${2:-}"
      shift 2
      ;;
    --client-secret)
      CLIENT_SECRET="${2:-}"
      shift 2
      ;;
    --model)
      MODEL_NAME="${2:-}"
      shift 2
      ;;
    --redirect-uri)
      REDIRECT_URI="${2:-}"
      shift 2
      ;;
    --lmstudio-url)
      LMSTUDIO_URL="${2:-}"
      shift 2
      ;;
    --state-file)
      STATE_FILE="${2:-}"
      shift 2
      ;;
    --reuse-session)
      REUSE_SESSION=1
      shift
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

if [[ -z "$SERVER_URL" || -z "$CLIENT_ID" || -z "$CLIENT_SECRET" || -z "$MODEL_NAME" ]]; then
  echo "error: --server-url, --client-id, --client-secret, and --model are required" >&2
  usage >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

run_agent() {
  if [[ -n "${AGENT_BIN:-}" ]]; then
    "$AGENT_BIN" "$@"
    return
  fi
  (
    cd "$REPO_ROOT"
    go run ./src/cmd/agent "$@"
  )
}

CMD=(
  init
  --server-url "$SERVER_URL"
  --client-id "$CLIENT_ID"
  --client-secret "$CLIENT_SECRET"
  --redirect-uri "$REDIRECT_URI"
  --model "$MODEL_NAME"
  --lmstudio-url "$LMSTUDIO_URL"
  --verbosity "$VERBOSITY"
)

if [[ -n "$STATE_FILE" ]]; then
  CMD+=(--state-file "$STATE_FILE")
fi
if [[ "$REUSE_SESSION" -eq 1 ]]; then
  CMD+=(--reuse-session)
fi

run_agent "${CMD[@]}"
