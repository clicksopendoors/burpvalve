# Install Burpvalve From A Skill-Only Copy

Use this file when the `burpvalve` skill is present but the executable is not.
The normal release package already includes:

```text
scripts/bin/burpvalve
```

If that file exists and is executable in an installed skill package, install
only the command shim:

```bash
skills_dir="${BURPVALVE_SKILLS_DIR:-$HOME/skills}"
skill_dir="$skills_dir/burpvalve"
bin_dir="$HOME/.local/bin"

mkdir -p "$bin_dir"
ln -sfn "$skill_dir/scripts/bin/burpvalve" "$bin_dir/burpvalve"
export PATH="$bin_dir:$PATH"
burpvalve -h
burpvalve completion --color never
burpvalve completion verify --json
```

## Bootstrap Missing Binary From GitHub

If `scripts/bin/burpvalve` is missing, do not try to invent the binary inside the
skill. Install the compiled release from the Burpvalve GitHub repository.

Use a pinned release tag. Burpvalve publishes release archives for Linux and
macOS on amd64 and arm64.

```bash
repo="${BURPVALVE_REPO:-clicksopendoors/burpvalve}"
version="${BURPVALVE_VERSION:-v0.2.0}"
skills_dir="${BURPVALVE_SKILLS_DIR:-$HOME/skills}"
bin_dir="${BURPVALVE_BIN_DIR:-$HOME/.local/bin}"
tmp="$(mktemp -d)"

curl -fsSL "https://raw.githubusercontent.com/$repo/${version}/install.sh" \
  -o "$tmp/install-burpvalve.sh"

less "$tmp/install-burpvalve.sh"
chmod +x "$tmp/install-burpvalve.sh"

bash "$tmp/install-burpvalve.sh" \
  --repo "$repo" \
  --version "$version" \
  --skills-dir "$skills_dir" \
  --bin-dir "$bin_dir" \
  --yes

export PATH="$bin_dir:$PATH"
burpvalve -h
burpvalve config --json
burpvalve completion --color never
burpvalve completion verify --json
burpvalve setup
```

That installer downloads the platform-specific release archive, verifies the
checksum, installs the full skill package, and creates:

```text
$bin_dir/burpvalve -> $skills_dir/burpvalve/scripts/bin/burpvalve
```

The installer prints an install plan before touching `skills_dir` or `bin_dir`.
It names the skill destination, command shim, and any existing files it will
replace. Without `--yes`, it asks for final confirmation and defaults to No.
Use `--yes` only when the selected directories are already the intended
locations.

Do not use Go's module installer for Burpvalve. The module path is
intentionally `burpvalve`, so public installs use the release package and
installer.

## If Raw GitHub Access Is Blocked

Download `install.sh` and a release archive through the GitHub UI or `gh`, then
install from the local archive:

```bash
repo="${BURPVALVE_REPO:-clicksopendoors/burpvalve}"
version="${BURPVALVE_VERSION:-v0.2.0}"
skills_dir="${BURPVALVE_SKILLS_DIR:-$HOME/skills}"
bin_dir="${BURPVALVE_BIN_DIR:-$HOME/.local/bin}"
tmp="$(mktemp -d)"

gh api "repos/$repo/contents/install.sh?ref=$version" --jq .content | base64 -d \
  > "$tmp/install-burpvalve.sh"

gh release download "$version" \
  --repo "$repo" \
  --dir "$tmp" \
  --pattern "burpvalve_$(go env GOOS)_$(go env GOARCH).tar.gz" \
  --pattern checksums.txt
chmod +x "$tmp/install-burpvalve.sh"

bash "$tmp/install-burpvalve.sh" \
  --from-archive "$tmp/burpvalve_$(go env GOOS)_$(go env GOARCH).tar.gz" \
  --skills-dir "$skills_dir" \
  --bin-dir "$bin_dir" \
  --yes
```

The installer prefers `gh release download` when it downloads assets itself,
then falls back to direct public GitHub release URLs.

## Verify

```bash
command -v burpvalve
burpvalve --version
burpvalve -h
burpvalve config --json
burpvalve completion --color never
burpvalve completion verify --json
test -x "$skills_dir/burpvalve/scripts/bin/burpvalve"
```

Expected result:

- `command -v burpvalve` points inside `bin_dir`.
- `bin_dir/burpvalve` is a symlink to the skill binary.
- `skills_dir/burpvalve/scripts/bin/burpvalve` exists and is executable.
- `burpvalve -h` prints the CLI help.
- `burpvalve completion --color never` prints shell-specific setup guidance
  instead of dumping a raw completion script.
- `burpvalve completion verify --json` reports whether the completion file,
  shell startup wiring, command path, and repo-local fallback are present.
- `burpvalve config --json` prints global/project config paths and merged
  defaults.

## Development Checkout

Only build from source when intentionally working in the Burpvalve repository:

```bash
make build
VERSION=dev ./scripts/package-skill.sh
bash ./install.sh \
  --from-archive "dist/burpvalve_$(go env GOOS)_$(go env GOARCH).tar.gz" \
  --skills-dir "$HOME/skills" \
  --bin-dir "$HOME/.local/bin" \
  --yes
```
