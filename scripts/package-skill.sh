#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${VERSION:-dev}"
goos="${GOOS:-$(go env GOOS)}"
goarch="${GOARCH:-$(go env GOARCH)}"
dist_dir="${DIST_DIR:-$repo_root/dist}"
asset_name="burpvalve_${goos}_${goarch}"
work_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

mkdir -p "$dist_dir"

binary_dir="$work_dir/build-bin"
make -C "$repo_root" build BINARY_DIR="$binary_dir" VERSION="$version"

package_root="$work_dir/burpvalve"
mkdir -p "$package_root"
cp -R "$repo_root/skill/burpvalve/." "$package_root/"
mkdir -p "$package_root/scripts/bin"
cp "$binary_dir/burpvalve" "$package_root/scripts/bin/burpvalve"
chmod 0755 "$package_root/scripts/bin/burpvalve"
printf '%s\n' "$version" > "$package_root/VERSION"
for notice_file in LICENSE THIRD_PARTY_NOTICES.md; do
  if [ -f "$repo_root/$notice_file" ]; then
    cp "$repo_root/$notice_file" "$package_root/$notice_file"
  fi
done

tar_path="$dist_dir/$asset_name.tar.gz"
tar -C "$work_dir" -czf "$tar_path" burpvalve

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$dist_dir" && sha256sum "$(basename "$tar_path")" > "$(basename "$tar_path").sha256")
else
  (cd "$dist_dir" && shasum -a 256 "$(basename "$tar_path")" > "$(basename "$tar_path").sha256")
fi

echo "wrote $tar_path"
