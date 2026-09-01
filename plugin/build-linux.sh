#!/usr/bin/env bash
set -euo pipefail

# Build the Linux native C ABI plugin beside the source tree.
# Override GOTOOLCHAIN when the target CLIProxyAPI uses a different Go version.
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN_DIR="$ROOT_DIR/plugin"
OUTPUT_DIR="$ROOT_DIR/dist"

mkdir -p "$OUTPUT_DIR"

WEB_DIR="$PLUGIN_DIR/web-ui"
if [[ ! -d "$WEB_DIR/node_modules" ]]; then
  echo "Missing Web UI dependencies. Run: (cd $WEB_DIR && pnpm install --ignore-scripts)" >&2
  exit 1
fi
(
  cd "$WEB_DIR"
  pnpm run build
)

cd "$PLUGIN_DIR"

: "${GOTOOLCHAIN:=auto}"
export GOTOOLCHAIN

go test ./...
go build -buildmode=c-shared -o "$OUTPUT_DIR/stickyproxy.so" ./cmd/stickyproxy
rm -f "$OUTPUT_DIR/stickyproxy.h"

echo "Built $OUTPUT_DIR/stickyproxy.so"
