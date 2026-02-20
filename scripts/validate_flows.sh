#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_FLOWK_BIN="$ROOT_DIR/bin/flowk"
FLOWK_BIN="${FLOWK_BIN:-$DEFAULT_FLOWK_BIN}"

if [[ ! -x "$FLOWK_BIN" && -x "${FLOWK_BIN}.exe" ]]; then
  FLOWK_BIN="${FLOWK_BIN}.exe"
fi

usage() {
  cat <<'EOF'
Usage: validate_flows.sh <flows-root>

Validate every FlowK definition inside <flows-root> using the
'-validate-only' mode. Files whose name ends with 'sf.json' are skipped
because they are treated as subflows.

You can override the FlowK binary by setting FLOWK_BIN.
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

ROOT_PATH="$1"

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
done < <(find "$ROOT_PATH" -type f -name '*.json' ! -name '*sf.json' -print0)

if [[ ${#flows[@]} -eq 0 ]]; then
  echo "info: no flow definitions found under '$ROOT_PATH'" >&2
  exit 0
fi

for flow in "${flows[@]}"; do
  printf 'Validating %s\n' "$flow"
  "$FLOWK_BIN" run -flow "$flow" -validate-only
done
