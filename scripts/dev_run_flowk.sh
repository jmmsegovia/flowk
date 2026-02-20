#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DATE="$(date +%Y-%m-%d)"

UI_HOST="127.0.0.1"
UI_PORT="5173"
FLOWK_HOST="127.0.0.1"
FLOWK_PORT="8080"

UI_DEV_URL="http://${UI_HOST}:${UI_PORT}"
DEV_UI_DIR="${ROOT_DIR}/tmp/ui-dev"
DEV_CONFIG="${ROOT_DIR}/tmp/ui-dev-config.yaml"
UI_PID=""

cleanup() {
  if [[ -n "${UI_PID}" ]]; then
    kill "${UI_PID}" >/dev/null 2>&1 || true
    wait "${UI_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

echo "Building core binary..."
mkdir -p "${ROOT_DIR}/bin"
CGO_ENABLED=0 go build \
  -ldflags "-X main.version=vdev -X main.commit=local -X main.date=${BUILD_DATE}" \
  -o "${ROOT_DIR}/bin/flowk" \
  "${ROOT_DIR}/cmd/flowk/main.go"

echo "Starting UI dev server..."
pushd "${ROOT_DIR}/ui" >/dev/null
npm run dev -- --host "${UI_HOST}" --port "${UI_PORT}" --strictPort &
UI_PID=$!
popd >/dev/null

mkdir -p "${DEV_UI_DIR}"
cat > "${DEV_UI_DIR}/index.html" <<EOF
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta http-equiv="refresh" content="0; url=${UI_DEV_URL}" />
    <title>FlowK UI Dev</title>
  </head>
  <body>
    <p>FlowK UI dev server: <a href="${UI_DEV_URL}">${UI_DEV_URL}</a></p>
  </body>
</html>
EOF

cat > "${DEV_CONFIG}" <<EOF
ui:
  host: "${FLOWK_HOST}"
  port: ${FLOWK_PORT}
  dir: "tmp/ui-dev"
EOF

echo "Starting FlowK UI server (API at http://${FLOWK_HOST}:${FLOWK_PORT}, UI at ${UI_DEV_URL})..."
"${ROOT_DIR}/bin/flowk" run -serve-ui -config "${DEV_CONFIG}" "$@"
