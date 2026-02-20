#!/usr/bin/env bash
set -uo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_FLOWK_BIN="$ROOT_DIR/../bin/flowk"
FLOWK_BIN="${FLOWK_BIN:-$DEFAULT_FLOWK_BIN}"

if [[ ! -x "$FLOWK_BIN" && -x "${FLOWK_BIN}.exe" ]]; then
  FLOWK_BIN="${FLOWK_BIN}.exe"
fi

usage() {
  cat <<'USAGE'
Usage: run_all_flows.sh [flows-root]

Run every FlowK definition under [flows-root] recursively, skipping any
directory named 'subflows'. The default root is the directory containing
this script.

You can override the FlowK binary by setting FLOWK_BIN.
USAGE
}

if [[ $# -gt 1 ]]; then
  usage
  exit 2
fi

ROOT_PATH="${1:-$ROOT_DIR}"

if [[ ! -d "$ROOT_PATH" ]]; then
  echo "error: '$ROOT_PATH' is not a directory" >&2
  exit 1
fi

if [[ ! -x "$FLOWK_BIN" ]]; then
  echo "error: cannot execute FlowK binary at $FLOWK_BIN" >&2
  exit 1
fi

declare -a flows=()
while IFS= read -r -d '' flow; do
  flows+=("$flow")
done < <(find "$ROOT_PATH" -type d -name subflows -prune -o -type f -name '*.json' -print0)

if [[ ${#flows[@]} -eq 0 ]]; then
  echo "info: no flow definitions found under '$ROOT_PATH'" >&2
  exit 0
fi

success=0
failure=0
declare -a success_flows=()
declare -a failed_flows=()

for flow in "${flows[@]}"; do
  printf 'Running %s\n' "$flow"
  if "$FLOWK_BIN" run -flow "$flow"; then
    success=$((success + 1))
    success_flows+=("$flow")
  else
    status=$?
    failure=$((failure + 1))
    failed_flows+=("$flow")
    printf 'Failed %s (exit %s)\n' "$flow" "$status" >&2
  fi
done

echo "Summary: $success success, $failure failed"
if [[ ${#success_flows[@]} -gt 0 ]]; then
  echo "Succeeded flows:"
  for flow in "${success_flows[@]}"; do
    echo "OK: $flow"
  done
else
  echo "Succeeded flows: none"
fi

if [[ ${#failed_flows[@]} -gt 0 ]]; then
  echo "Failed flows:"
  for flow in "${failed_flows[@]}"; do
    echo "FAIL: $flow"
  done
else
  echo "Failed flows: none"
fi

if [[ $failure -gt 0 ]]; then
  exit 1
fi
