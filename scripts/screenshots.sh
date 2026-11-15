#!/usr/bin/env bash
# Renders README screenshots from demo data (no real chats): go test writes HTML, Chrome turns it into PNG.
set -euo pipefail
cd "$(dirname "$0")/.."
export PATH="/usr/local/go/bin:$PATH"
go test -tags screenshots ./internal/ui -run TestScreenshots -count=1 >/dev/null
for page in docs/screenshots/*.html; do
  google-chrome --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
    --window-size=1300,770 --screenshot="${page%.html}.png" "file://$PWD/$page" >/dev/null 2>&1
  rm "$page"
done
ls docs/screenshots
