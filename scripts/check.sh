#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# Keep an enclosing agent session from changing fixture identity or configuration.
for name in $(env | sed -nE 's/^((HOLLER|CLAUDE|CODEX)[A-Za-z0-9_]*)=.*/\1/p'); do
  unset "$name"
done
export HOLLER_REQUIRE_OPENCODE_PLUGIN_TEST=1
python3 scripts/validate.py
python3 -m unittest discover -s tests -v
python3 scripts/check-boundary.py
go test ./...
go vet ./...
go build -o dist/holler-connector ./cmd/holler-connector
