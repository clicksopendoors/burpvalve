#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mkdir -p docs/demos/generated

if ! command -v freeze >/dev/null 2>&1; then
  echo "freeze is required; install with: go install github.com/charmbracelet/freeze@latest" >&2
  exit 1
fi

if ! command -v vhs >/dev/null 2>&1; then
  echo "vhs is required; install with: go install github.com/charmbracelet/vhs@latest" >&2
  exit 1
fi

make build >/dev/null

freeze docs/demos/init-question-answer.txt \
  --language text \
  --output docs/demos/generated/init-question-answer.svg \
  --window \
  --padding 24 \
  --border.radius 8 \
  --font.size 16 \
  --line-height 1.25

freeze --execute "bash docs/demos/completion-demo.sh" \
  --language text \
  --output docs/demos/generated/completion-guide.svg \
  --window \
  --padding 24 \
  --border.radius 8 \
  --font.size 16 \
  --line-height 1.25 \
  --width 980

freeze --execute "bash docs/demos/config-demo.sh" \
  --language json \
  --output docs/demos/generated/config-json.svg \
  --window \
  --padding 24 \
  --border.radius 8 \
  --font.size 16 \
  --line-height 1.25 \
  --width 980

vhs docs/demos/agent-turn.tape
