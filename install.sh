#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Install the burpvalve skill from a GitHub release package.

Usage:
  install.sh [options]

Options:
  --repo OWNER/REPO       GitHub repository to download from.
  --version VERSION       Release tag to install. Defaults to latest.
  --skills-dir DIR        Directory that contains skill folders. Defaults to ~/skills.
  --bin-dir DIR           Directory for command shims. Defaults to ~/.local/bin.
  --from-archive PATH     Install from a local release tarball instead of GitHub.
  --yes                   Use defaults without prompting.
  --no-shims              Do not create the burpvalve command shim.
  -h, --help              Show this help.

Environment:
  BURPVALVE_REPO
  BURPVALVE_VERSION
  BURPVALVE_SKILLS_DIR
  BURPVALVE_BIN_DIR

The installer prefers `gh release download` when `gh` is available, then falls
back to direct public release URLs. Set BURPVALVE_REPO or pass --repo to use a
fork or repository that requires authentication.
EOF
}

repo="${BURPVALVE_REPO:-clicksopendoors/burpvalve}"
version="${BURPVALVE_VERSION:-latest}"
skills_dir="${BURPVALVE_SKILLS_DIR:-}"
bin_dir="${BURPVALVE_BIN_DIR:-$HOME/.local/bin}"
from_archive=""
assume_yes=0
install_shims=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      repo="${2:?missing value for --repo}"
      shift 2
      ;;
    --version)
      version="${2:?missing value for --version}"
      shift 2
      ;;
    --skills-dir)
      skills_dir="${2:?missing value for --skills-dir}"
      shift 2
      ;;
    --bin-dir)
      bin_dir="${2:?missing value for --bin-dir}"
      shift 2
      ;;
    --from-archive)
      from_archive="${2:?missing value for --from-archive}"
      shift 2
      ;;
    --yes)
      assume_yes=1
      shift
      ;;
    --no-shims)
      install_shims=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

default_skills_dir() {
  echo "$HOME/skills"
}

prompt_skills_dir() {
  local default="$1"
  if [[ -n "$skills_dir" ]]; then
    return
  fi
  if [[ "$assume_yes" -eq 1 ]]; then
    skills_dir="$default"
    return
  fi
  if [[ ! -t 0 ]]; then
    echo "no terminal available; pass --skills-dir DIR or --yes" >&2
    exit 2
  fi
  printf 'Install skills into which directory? [%s] ' "$default" >&2
  read -r skills_dir
  skills_dir="${skills_dir:-$default}"
}

confirm_install_plan() {
  local package_dir="$1"
  local dest="$2"
  local shim_path="$3"

  echo "Burpvalve install plan" >&2
  echo >&2
  echo "Package source: $package_dir" >&2
  echo "Skills directory: $skills_dir" >&2
  echo "Skill destination: $dest" >&2
  if [[ -e "$dest" ]]; then
    echo "Existing skill: replace $dest" >&2
  else
    echo "Existing skill: none" >&2
  fi
  if [[ "$install_shims" -eq 1 ]]; then
    echo "Command shim: $shim_path -> $dest/scripts/bin/burpvalve" >&2
    if [[ -e "$shim_path" || -L "$shim_path" ]]; then
      echo "Existing shim: replace $shim_path" >&2
    else
      echo "Existing shim: none" >&2
    fi
  else
    echo "Command shim: skipped (--no-shims)" >&2
  fi
  echo >&2

  if [[ "$assume_yes" -eq 1 ]]; then
    echo "--yes supplied; applying install plan." >&2
    return
  fi

  if [[ ! -t 0 ]]; then
    echo "no terminal available; pass --yes to apply this install plan" >&2
    exit 2
  fi

  echo "Default is No." >&2
  printf 'Apply these changes? [y/N] ' >&2
  local answer
  read -r answer
  case "$answer" in
    y|Y|yes|YES|Yes)
      ;;
    *)
      echo "install cancelled; no files changed" >&2
      exit 2
      ;;
  esac
}

download() {
  local url="$1"
  local out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  else
    echo "curl or wget is required to download release assets" >&2
    exit 1
  fi
}

download_release_assets_with_gh() {
  local repo="$1"
  local version="$2"
  local asset="$3"
  local out_dir="$4"

  if ! command -v gh >/dev/null 2>&1; then
    return 1
  fi

  rm -f "$out_dir/$asset" "$out_dir/checksums.txt"

  local args=(release download --repo "$repo" --dir "$out_dir" --clobber --pattern "$asset" --pattern checksums.txt)
  if [[ "$version" != "latest" ]]; then
    args=(release download "$version" --repo "$repo" --dir "$out_dir" --clobber --pattern "$asset" --pattern checksums.txt)
  fi

  if gh "${args[@]}" >/dev/null 2>&1 && [[ -f "$out_dir/$asset" && -f "$out_dir/checksums.txt" ]]; then
    return 0
  fi

  return 1
}

download_release_assets_direct() {
  local repo="$1"
  local version="$2"
  local asset="$3"
  local out_dir="$4"
  local base_url

  if [[ "$version" == "latest" ]]; then
    base_url="https://github.com/$repo/releases/latest/download"
  else
    base_url="https://github.com/$repo/releases/download/$version"
  fi

  download "$base_url/$asset" "$out_dir/$asset" &&
    download "$base_url/checksums.txt" "$out_dir/checksums.txt"
}

download_release_assets() {
  local repo="$1"
  local version="$2"
  local asset="$3"
  local out_dir="$4"

  if download_release_assets_with_gh "$repo" "$version" "$asset" "$out_dir"; then
    return
  fi

  if download_release_assets_direct "$repo" "$version" "$asset" "$out_dir"; then
    return
  fi

  echo "could not download $asset from $repo release $version" >&2
  echo "make sure the release exists, the repository is public or accessible to this shell, and network access is available" >&2
  exit 1
}

sha256_file() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
  else
    shasum -a 256 "$path" | awk '{print $1}'
  fi
}

verify_checksum() {
  local archive="$1"
  local checksums="$2"
  local asset
  asset="$(basename "$archive")"
  local expected
  expected="$(awk -v asset="$asset" '$2 == asset {print $1}' "$checksums")"
  if [[ -z "$expected" ]]; then
    echo "checksum for $asset not found in $(basename "$checksums")" >&2
    exit 1
  fi
  local actual
  actual="$(sha256_file "$archive")"
  if [[ "$actual" != "$expected" ]]; then
    echo "checksum mismatch for $asset" >&2
    echo "expected: $expected" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi
}

goos="$(detect_os)"
goarch="$(detect_arch)"
asset="burpvalve_${goos}_${goarch}.tar.gz"
tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

prompt_skills_dir "$(default_skills_dir)"

if [[ -n "$from_archive" ]]; then
  archive="$from_archive"
  if [[ ! -f "$archive" ]]; then
    echo "archive not found: $archive" >&2
    exit 1
  fi
else
  archive="$tmp_dir/$asset"
  checksums="$tmp_dir/checksums.txt"
  download_release_assets "$repo" "$version" "$asset" "$tmp_dir"
  verify_checksum "$archive" "$checksums"
fi

extract_dir="$tmp_dir/extract"
mkdir -p "$extract_dir"
tar -xzf "$archive" -C "$extract_dir"

package_dir="$extract_dir/burpvalve"
if [[ ! -x "$package_dir/scripts/bin/burpvalve" ]]; then
  echo "release package is missing required scripts/bin/burpvalve binary" >&2
  exit 1
fi

dest="$skills_dir/burpvalve"
staging="$skills_dir/.burpvalve.install.$$"
backup="$skills_dir/.burpvalve.backup.$$"
shim_path="$bin_dir/burpvalve"

confirm_install_plan "$package_dir" "$dest" "$shim_path"

mkdir -p "$skills_dir"
rm -rf "$staging" "$backup"
cp -R "$package_dir" "$staging"

if [[ -e "$dest" ]]; then
  mv "$dest" "$backup"
fi
mv "$staging" "$dest"
rm -rf "$backup"

if [[ "$install_shims" -eq 1 ]]; then
  mkdir -p "$bin_dir"
  ln -sfn "$dest/scripts/bin/burpvalve" "$shim_path"
fi

echo "Installed burpvalve skill to $dest"
if [[ "$install_shims" -eq 1 ]]; then
  echo "Installed command shims to $bin_dir"
  echo "Verify with: $shim_path --version"
  case ":$PATH:" in
    *":$bin_dir:"*)
      resolved="$(command -v burpvalve || true)"
      if [[ -n "$resolved" ]]; then
        echo "burpvalve is on current PATH at $resolved"
      else
        echo "Current PATH includes $bin_dir, but burpvalve was not resolved by this shell"
      fi
      ;;
    *)
      echo "Current PATH does not include $bin_dir"
      echo "Add it for this shell with: export PATH=\"$bin_dir:\$PATH\""
      ;;
  esac
fi
