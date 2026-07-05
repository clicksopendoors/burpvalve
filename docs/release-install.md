# Release And Install

Release packages are built from source and include the skill instructions plus
the platform-specific `burpvalve` binary:

```text
burpvalve/
  SKILL.md
  SELF-TEST.md
  VERSION
  scripts/
    bin/
      burpvalve
```

## Build A Local Package

```bash
VERSION=dev ./scripts/package-skill.sh
```

This writes:

```text
dist/burpvalve_<goos>_<goarch>.tar.gz
dist/burpvalve_<goos>_<goarch>.tar.gz.sha256
```

## Install From GitHub

Public installs should use a pinned release tag. Burpvalve publishes release
archives for Linux and macOS on amd64 and arm64.

Download the installer for the pinned release, inspect it, then run it:

```bash
version="v0.2.0"
tmp="$(mktemp -d)"

curl -fsSL "https://raw.githubusercontent.com/clicksopendoors/burpvalve/${version}/install.sh" \
  -o "$tmp/install-burpvalve.sh"

less "$tmp/install-burpvalve.sh"
chmod +x "$tmp/install-burpvalve.sh"

"$tmp/install-burpvalve.sh" \
  --repo clicksopendoors/burpvalve \
  --version "$version" \
  --skills-dir "$HOME/skills" \
  --bin-dir "$HOME/.local/bin"
```

If you accept the tradeoff of piping a remote installer into a shell, use the
same pinned tag:

```bash
version="v0.2.0"
curl -fsSL "https://raw.githubusercontent.com/clicksopendoors/burpvalve/${version}/install.sh" | \
  bash -s -- \
    --repo clicksopendoors/burpvalve \
    --version "$version" \
    --skills-dir "$HOME/skills" \
    --bin-dir "$HOME/.local/bin" \
    --yes
```

The downloaded-file path is safer because you can read `install.sh` first and
let the installer ask for final confirmation. Press Enter to accept the
recommended skills directory default:

```text
~/skills
```

The final confirmation defaults to No. Pass `--yes` only for noninteractive
installs where `--skills-dir` and `--bin-dir` are already explicit.

Pinned install:

```bash
bash install.sh --repo clicksopendoors/burpvalve --version v0.2.0
```

Local archive install:

```bash
bash install.sh --from-archive dist/burpvalve_$(go env GOOS)_$(go env GOARCH).tar.gz
```

If raw GitHub URLs are blocked in your environment, download `install.sh` and a
release archive through the GitHub UI or `gh release download`, then install
with `--from-archive`.

Do not use Go's module installer for Burpvalve. The module path is
intentionally `burpvalve`, so public installs use the release package and
installer instead.

## Verify

```bash
burpvalve --version
burpvalve -h
burpvalve init -h
burpvalve setup --json
jsm validate "$HOME/skills/burpvalve"
```

If you installed with `--skills-dir`, validate the `burpvalve` folder under that
directory instead.

From a target repo:

```bash
burpvalve setup --json
```

Partial init can skip optional integration pieces while keeping the rest of the
scaffold behavior unchanged:

```bash
burpvalve init --no-beads --no-ntm --no-agents --no-claude
```
