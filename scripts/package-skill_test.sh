#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

VERSION="test-package" DIST_DIR="$tmp_dir/dist" "$repo_root/scripts/package-skill.sh" >"$tmp_dir/burpvalve-package-skill-test.log"

archive="$tmp_dir/dist/burpvalve_$(go env GOOS)_$(go env GOARCH).tar.gz"
if [[ ! -f "$archive" ]]; then
  echo "missing package archive: $archive" >&2
  exit 1
fi

tar -tzf "$archive" > "$tmp_dir/contents.txt"

for path in \
  "burpvalve/LICENSE" \
  "burpvalve/THIRD_PARTY_NOTICES.md" \
  "burpvalve/VERSION" \
  "burpvalve/scripts/bin/burpvalve"; do
  if ! grep -Fxq "$path" "$tmp_dir/contents.txt"; then
    echo "archive missing $path" >&2
    exit 1
  fi
done

tar -xzf "$archive" -C "$tmp_dir" burpvalve/LICENSE burpvalve/THIRD_PARTY_NOTICES.md

grep -q "MIT License" "$tmp_dir/burpvalve/LICENSE"
grep -q "gopkg.in/yaml.v3 Upstream NOTICE" "$tmp_dir/burpvalve/THIRD_PARTY_NOTICES.md"
grep -q "Copyright 2011-2016 Canonical Ltd." "$tmp_dir/burpvalve/THIRD_PARTY_NOTICES.md"

go list -deps -f '{{if not .Standard}}{{with .Module}}{{.Path}}{{end}}{{end}}' "$repo_root/cmd/burpvalve" |
  sort -u |
  grep -v '^burpvalve$' > "$tmp_dir/binary-modules.txt"

while IFS= read -r module; do
  if ! grep -Fq "$module" "$tmp_dir/burpvalve/THIRD_PARTY_NOTICES.md"; then
    echo "THIRD_PARTY_NOTICES.md missing binary dependency $module" >&2
    exit 1
  fi
done < "$tmp_dir/binary-modules.txt"

echo "package-skill license validation passed"
