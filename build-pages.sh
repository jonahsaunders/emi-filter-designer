#!/usr/bin/env sh
# Builds the single-file web version (GitHub Pages): pages/index.html
set -e
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o pages/engine.wasm ./cmd/wasm
python3 tools/build_pages.py pages/engine.wasm pages/index.html
rm pages/engine.wasm
