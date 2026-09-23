#!/usr/bin/env sh
# Builds EMIFilterDesigner.exe (Windows, GUI subsystem) and a native binary.
set -e
cd "$(dirname "$0")"
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/EMIFilterDesigner.exe .
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/emifilterdesigner .
echo "Built dist/EMIFilterDesigner.exe and dist/emifilterdesigner"
