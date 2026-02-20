#!/usr/bin/env bash
set -euo pipefail

publish=false

for arg in "$@"; do
  case "$arg" in
    --publish)
      publish=true
      ;;
    --snapshot)
      publish=false
      ;;
    *)
      echo "Unknown option: $arg" >&2
      echo "Usage: $0 [--snapshot|--publish]" >&2
      exit 1
      ;;
  esac
done

if [ "$publish" = true ]; then
  goreleaser release --clean
else
  goreleaser release --snapshot --clean
fi

if [ -d "ui/dist-release" ]; then
  rm -rf ui/dist-release
fi
