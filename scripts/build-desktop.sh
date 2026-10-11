#!/bin/sh
# Build the macOS app. Run from any directory; see docs/DESKTOP.md for prerequisites.
set -eu

if [ "$(uname -s)" != Darwin ]; then
  echo "The desktop app currently supports macOS only." >&2
  exit 1
fi
for tool in go npm wails; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing $tool; see docs/DESKTOP.md for desktop build setup." >&2
    exit 1
  fi
done

desktop_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../desktop" && pwd)
cd "$desktop_dir"
export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=13.0"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=13.0"
# api.ts uses Wails' runtime bridge directly; generated TypeScript is unnecessary.
exec wails build -skipbindings "$@"
