#!/bin/sh
# Run from the repository root. Used by GoReleaser before assembling archives.
set -eu

mkdir -p build/completions
for shell in bash zsh fish; do
  go run ./cmd/ais completion "$shell" > "build/completions/ais.$shell"
done
