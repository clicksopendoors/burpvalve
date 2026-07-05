#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/config-root/.config/burpvalve" "$tmp/repo"

cat >"$tmp/config-root/.config/burpvalve/config.json" <<'JSON'
{
  "defaults": {
    "shell": "zsh",
    "bin_dir": "~/.local/bin",
    "init": {
      "beads": false,
      "ntm": false
    }
  }
}
JSON

cat >"$tmp/repo/.burpvalve.json" <<'JSON'
{
  "defaults": {
    "init": {
      "repo_bin": true
    }
  }
}
JSON

HOME="$tmp/config-root" \
XDG_CONFIG_HOME="$tmp/config-root/.config" \
  "$repo_root/bin/burpvalve" config --target "$tmp/repo" --json |
  sed "s#$tmp/config-root#~#g" |
  sed "s#$tmp/repo#./demo-repo#g"
