#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'USAGE'
Usage: secret_scan.sh [gitleaks-args...]

Runs gitleaks against the repository (including git history).

Examples:
  secret_scan.sh
  secret_scan.sh --log-opts "-1"
  secret_scan.sh --no-git
  secret_scan.sh --report-format sarif --report-path gitleaks.sarif
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if ! command -v gitleaks >/dev/null 2>&1; then
  echo "error: gitleaks is not installed or not on PATH" >&2
  echo "install: brew install gitleaks  (or)  go install github.com/gitleaks/gitleaks/v8@latest" >&2
  exit 1
fi

gitleaks detect --source "${ROOT_DIR}" --redact "$@"
