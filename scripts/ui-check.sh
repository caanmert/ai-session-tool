#!/bin/sh
# Run the fixture-based browser checks in light and dark: text contrast,
# overflow and the 960x640 minimum window. Uses fictional data only; no agent
# or provider is contacted.
#
# Needs Google Chrome, a built frontend (npm run build in desktop/frontend) and
# Playwright. Point AIS_PLAYWRIGHT_MODULE at an existing install, or let the
# script install the pinned version into a temporary folder.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
port=${AIS_UI_CHECK_PORT:-5183}
out=${AIS_SCREENSHOT_DIR:-$work/screenshots}
preview=

cleanup() {
  [ -z "$preview" ] || kill "$preview" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

if [ -z "${AIS_PLAYWRIGHT_MODULE:-}" ]; then
  npm install --prefix "$work/pw" --no-audit --no-fund playwright@1.58.2 >/dev/null
  AIS_PLAYWRIGHT_MODULE=$work/pw/node_modules/playwright/index.mjs
fi
export AIS_PLAYWRIGHT_MODULE
export AIS_CONTRAST_MODULE="$root/scripts/ui-contrast.mjs"

(cd "$root/desktop/frontend" && exec npm run preview -- --port "$port" --strictPort) \
  >"$work/preview.log" 2>&1 &
preview=$!
url=http://127.0.0.1:$port
tries=0
until curl -fs "$url" >/dev/null 2>&1; do
  tries=$((tries + 1))
  if [ "$tries" -gt 50 ] || ! kill -0 "$preview" 2>/dev/null; then
    cat "$work/preview.log" >&2
    echo "The frontend preview did not start. Run npm run build in desktop/frontend first." >&2
    exit 1
  fi
  sleep 0.2
done

for mode in light dark; do
  echo "== $mode"
  AIS_APPEARANCE=$mode AIS_SCREENSHOT_DIR="$out/$mode" \
    node "$root/scripts/desktop-screenshots.mjs" "$url"
  AIS_APPEARANCE=$mode AIS_SCREENSHOT_DIR="$out/$mode" \
    node "$root/scripts/run-screenshots.mjs" "$url"
done
echo "UI checks passed."
