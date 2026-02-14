#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  tools/dev/agent/run-once.sh --workspace PATH [options]

Options:
  --workspace PATH       Workspace path used by tool sandbox (required)
  --state-file PATH      Optional explicit state file path
  --max-iter N           Max LM Studio round-trips (default: 3)
  --model NAME           Optional model override for this run
  --lmstudio-url URL     Optional LM Studio URL override
  --verbosity N          Log verbosity for agent (default: 3)
  -h, --help             Show this help

Environment:
  AGENT_BIN              Optional path to BorealValley-agent binary.
                         If unset, runs: go run ./src/cmd/agent
USAGE
}

WORKSPACE=""
STATE_FILE=""
MAX_ITER="3"
MODEL_NAME=""
LMSTUDIO_URL=""
VERBOSITY="3"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --workspace)
      WORKSPACE="${2:-}"
      shift 2
      ;;
    --state-file)
      STATE_FILE="${2:-}"
      shift 2
      ;;
    --max-iter)
      MAX_ITER="${2:-}"
      shift 2
      ;;
    --model)
      MODEL_NAME="${2:-}"
      shift 2
      ;;
    --lmstudio-url)
      LMSTUDIO_URL="${2:-}"
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

if [[ -z "$WORKSPACE" ]]; then
  echo "error: --workspace is required" >&2
  usage >&2
  exit 2
fi

if [[ ! -d "$WORKSPACE" ]]; then
  echo "error: workspace does not exist or is not a directory: $WORKSPACE" >&2
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
  run
  --workspace "$WORKSPACE"
  --max-iter "$MAX_ITER"
  --verbosity "$VERBOSITY"
)

if [[ -n "$STATE_FILE" ]]; then
  CMD+=(--state-file "$STATE_FILE")
fi
if [[ -n "$MODEL_NAME" ]]; then
  CMD+=(--model "$MODEL_NAME")
fi
if [[ -n "$LMSTUDIO_URL" ]]; then
  CMD+=(--lmstudio-url "$LMSTUDIO_URL")
fi

run_agent "${CMD[@]}"
