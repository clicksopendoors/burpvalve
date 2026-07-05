#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/config-root" "$tmp/repo"
cd "$tmp/repo"

HOME="$tmp/config-root" \
SHELL=/bin/zsh \
BURPVALVE_CONFIG="$tmp/config-root/.config/burpvalve/config.json" \
  "$repo_root/bin/burpvalve" completion --color never |
  sed "s#$tmp/config-root#~#g"
