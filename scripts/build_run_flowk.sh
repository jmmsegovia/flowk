#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DATE="$(date +%Y-%m-%d)"

echo "Building UI..."
pushd "${ROOT_DIR}/ui" >/dev/null
if [[ ! -x node_modules/.bin/tsc ]]; then
  echo "Installing UI dependencies..."
  npm ci
fi
npm run build
popd >/dev/null

echo "Building core binary..."
mkdir -p "${ROOT_DIR}/bin"
CGO_ENABLED=0 go build \
  -ldflags "-X main.version=vdev -X main.commit=local -X main.date=${BUILD_DATE}" \
  -o "${ROOT_DIR}/bin/flowk" \
  "${ROOT_DIR}/cmd/flowk/main.go"

echo "Done. Binary: ${ROOT_DIR}/bin/flowk"
${ROOT_DIR}/bin/flowk run -serve-ui
