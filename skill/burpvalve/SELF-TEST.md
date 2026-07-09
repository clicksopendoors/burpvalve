# Self-Test

Validate the source skill package:

```bash
jsm validate skill/burpvalve
```

Build a local release package:

```bash
VERSION=dev ./scripts/package-skill.sh
```

Install that package into a temporary skills directory:

```bash
tmp="$(mktemp -d)"
bash ./install.sh --from-archive dist/burpvalve_$(go env GOOS)_$(go env GOARCH).tar.gz --skills-dir "$tmp/skills" --bin-dir "$tmp/bin" --yes
"$tmp/skills/burpvalve/scripts/bin/burpvalve" --version
"$tmp/bin/burpvalve" --version
test -f "$tmp/skills/burpvalve/INSTALL.md"
test -f "$tmp/skills/burpvalve/references/deterministic-backpressure.md"
"$tmp/bin/burpvalve" -h
"$tmp/bin/burpvalve" init -h
"$tmp/bin/burpvalve" setup --json
"$tmp/bin/burpvalve" config --json
"$tmp/bin/burpvalve" prompts list
"$tmp/bin/burpvalve" prompts show commit-choreography --var bead=bd-test
"$tmp/bin/burpvalve" prompts show verifier-bootstrap --var agent=Verifier --var project_key=/example/project --var orchestrator=Coordinator
"$tmp/bin/burpvalve" completion --color never
"$tmp/bin/burpvalve" --robots -h
"$tmp/bin/burpvalve" init --robots -h
```

Run from a target repository:

```bash
PATH="$tmp/bin:$PATH" burpvalve setup --json
PATH="$tmp/bin:$PATH" burpvalve config --json
target="$(mktemp -d)"
PATH="$tmp/bin:$PATH" burpvalve init --target "$target" --force --json --no-beads --no-ntm --no-agents --no-claude
test ! -e "$target/bin/burpvalve"
partial="$(mktemp -d)"
PATH="$tmp/bin:$PATH" burpvalve init --target "$partial" --force --json log attestations
repair_target="$(mktemp -d)"
printf '# Local Notes\n' > "$repair_target/AGENTS.md"
PATH="$tmp/bin:$PATH" burpvalve repair --target "$repair_target" --force --json AGENTS.md
robot_target="$(mktemp -d)"
printf '{"target":"%s","confirm":true,"skip":{"beads":true,"ntm":true,"bin":true}}\n' "$robot_target" \
  | PATH="$tmp/bin:$PATH" burpvalve init --robots
```

Expected result:

- validation passes;
- the package includes `INSTALL.md`;
- the package includes `references/deterministic-backpressure.md`;
- the package includes `scripts/bin/burpvalve`;
- `burpvalve -h` shows description, quick start, shell integration, usage, and commands;
- command help such as `burpvalve init -h` explains what the command does and which flags are available;
- `burpvalve --robots -h` and command robot help return structured JSON;
- `burpvalve config --json` reports merged global/project defaults;
- `burpvalve prompts list` includes canonical prompt-bank entries;
- `burpvalve prompts show` renders required-variable prompts without needing copied prompt bodies in the skill;
- `burpvalve completion --color never` prints setup guidance, not a raw completion script;
- `burpvalve setup` invokes the copied user-bin command;
- default `burpvalve init` does not install repo-local `bin/burpvalve`;
- partial `burpvalve init` reports skipped components and still creates the backpressure scaffold;
- targeted `burpvalve init log attestations` creates only those scaffold pieces;
- targeted `burpvalve repair AGENTS.md` appends missing sections without creating unrelated pieces;
- `burpvalve init --robots` accepts JSON input with `confirm: true` and does not open a TUI;
- the output is a readiness report for the current target repo.
