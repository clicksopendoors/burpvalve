---
name: burpvalve
description: >-
  Set up and operate Burpvalve repo backpressure: install/check/repair scaffold,
  use agent-safe JSON or robot flows, and run commit gates without fabricating
  verification evidence.
category: other
tags:
  - ctx-cli
  - ctx-docs
  - tool-git
license: MIT
distribution: public
---

# burpvalve

> Core rule: humans can use the guided terminal flow; agents should use JSON or
> robot mode and provide explicit evidence.

Use this skill when a repository needs Burpvalve installed, checked, repaired,
or used as a commit gate. Burpvalve gives each agent work unit a local
operating contract, standard project folders, optional coordination tools, and
a commit-time backpressure layer that refuses weak staged work before it
reaches a human.

Release packages include this skill and one platform-specific binary:

```text
scripts/bin/burpvalve
```

If this skill is present without that binary, read `INSTALL.md`. It explains how
to fetch the official installer, install the compiled release package, and put
the `burpvalve` command on `PATH`.

Bootstrap order for skill-only installs:

1. Check whether `scripts/bin/burpvalve` exists in the installed skill package.
2. If it exists, copy or repair the user-bin command described in `INSTALL.md`.
3. If it is missing, install a compiled release package with `INSTALL.md`.
4. Verify `burpvalve -h`, `burpvalve config --json`, and
   `burpvalve prompts list` before relying on prompt-bank references below.

## Command Model

| Command | Use It For | Mutates Files |
|---|---|---|
| `burpvalve setup` | Inspect a repo and report missing pieces. | No |
| `burpvalve config` / `burpvalve config show` | Show global and project config paths, merged defaults, and value sources. | No |
| `burpvalve config init` | Create or update global or project config. | Yes, after guided confirmation, `--force`, or robot confirmation |
| `burpvalve completion` | Install shell completions and optional PATH wiring. | Yes, with installer flow or flags |
| `burpvalve init` | Add the standard scaffold and hook wiring. | Yes |
| `burpvalve repair` | Restore missing generated pieces without overwriting project knowledge. | Yes |
| `burpvalve commit` | Check the staged payload and write/pass attestation evidence. | Writes attestation or blocked report |
| `burpvalve gate run` | Execute a prepared, hash-bound local gate ceremony through commit and journaled handoff. | Yes, with explicit confirmation or robot confirmation |
| `burpvalve explain` | Translate setup, lint, commit, attestation, or blocked-report JSON into recovery steps. | No |
| `burpvalve prompts` | List or render canonical orchestrator prompt templates. | No, unless `prompts show --write` exports a local copy |
| `burpvalve verifier prompts` | Generate read-only verifier handoff packets for the staged payload. | No |
| `burpvalve beads preflight` | Plan Beads delivery closure without mutating Beads or Git. | No |
| `burpvalve lint` | Run executable commands declared in `backpressure/manifest.yaml`. | No |
| `burpvalve ci` | Validate attestation evidence in automation. | No |

## Human vs Agent Use

| Situation | Prefer | Why |
|---|---|---|
| Human exploring the tool | `burpvalve -h`, `burpvalve init`, `burpvalve repair`, `burpvalve completion` | The Bubble Tea flow asks readable questions and confirms before mutation. |
| Agent inspecting a repo | `burpvalve setup --json`, `burpvalve config --json` | Machine-readable output is easier to reason about and quote back. |
| Agent applying a known mutation | `burpvalve init --force --json ...` or `burpvalve repair --force --json ...` | `--force` skips terminal questions and applies explicit flags/targets. |
| Agent applying structured input | `printf '{...}' \| burpvalve init --robots` | Robot mode accepts JSON input and never opens a TUI. |
| Agent recovering from a blocked result | `burpvalve setup --json \| burpvalve explain --json -` or `burpvalve explain --json log/backpressure/failed/<file>.json` | Explain reads structured facts and returns stable blockers plus next steps without scraping human prose. |
| Agent needing reusable workflow prompts | `burpvalve prompts list` and `burpvalve prompts show <name>` | Prompt-bank entries are versioned in the binary and avoid stale copied workflow prose. |
| Agent preparing verifier cells | `burpvalve verifier prompts --feature <id> --json` | Prompt packets include staged paths, condition policy, and the response schema for read-only verifiers. |
| Agent running prepared gate mechanics | `burpvalve gate run --handoff <file> --dry-run --json`, then `--yes --json` only if the handoff and state match | Gate run refuses stale hashes, missing verifier evidence, dirty index, and stale `HEAD`; it does not fabricate verdicts. |
| Commit hook or CI | `burpvalve commit --feature ... --responses ...`, `burpvalve lint`, `burpvalve ci` | Hooks and CI need deterministic inputs and outputs. |

Mutating commands default to human safety. In an interactive terminal they ask
questions and then ask for final confirmation, defaulting to No. In automation,
use `--force` with explicit flags/targets, or use `--robots` with stdin JSON
that includes `"confirm": true`.

## Quick Start For Agents

From the target repository:

```bash
burpvalve setup --json
burpvalve config --json
burpvalve init -h
burpvalve prompts list
burpvalve lint --json | burpvalve explain --json -
```

Use `burpvalve config --json` to see effective defaults and whether each value
came from global config or project config. Config does not mutate a repo by
itself; it only changes future prompts, default selections, and robot facts when
a Burpvalve command is run. Humans can run `burpvalve config init` for a guided
flow. Agents should write config noninteractively with explicit JSON and
confirmation:

```bash
burpvalve config init --project --file .burpvalve.seed.json --force --json
printf '{"scope":"project","confirm":true,"config":{"schema_version":1,"defaults":{"shell":"zsh"}}}' \
  | burpvalve config init --robots
```

If the repo should receive the standard scaffold:

```bash
burpvalve init --force --json
```

If the repo should skip optional integrations:

```bash
burpvalve init --force --json --no-beads --no-ntm
```

If the user asked for only specific pieces:

```bash
burpvalve init --force --json log attestations
burpvalve repair --force --json AGENTS.md
burpvalve repair --force --json hooks
```

If an orchestrator supplied a prepared gate-run handoff:

```bash
burpvalve gate run --dry-run --handoff log/backpressure/gate-runs/<id>-handoff.json --json
burpvalve gate run --handoff log/backpressure/gate-runs/<id>-handoff.json --yes --json
```

Treat `gate run` as a fail-closed mechanical runner. It still relies on real
verifier responses bound to the current staged payload. In v1 it journals the
push command for the orchestrator instead of pushing by itself.

Robot input example:

```bash
printf '{"target":".","confirm":true,"skip":{"beads":true,"ntm":true}}' \
  | burpvalve init --robots
```

Use `burpvalve --robots -h` and `burpvalve <command> --robots -h` when you need
structured command documentation.

## Canonical Prompt Bank

When the binary is available, use `burpvalve prompts` for reusable orchestrator
and verifier prompts instead of copying long workflow text into chat or local
docs. Prompt names are a stable public API; render them from the current binary
so wording stays aligned with the installed Burpvalve version.

```bash
burpvalve prompts list
burpvalve prompts show commit-choreography --var bead=<bead-id>
burpvalve prompts show verifier-brief --var feature=<feature-id>
burpvalve prompts show verifier-bootstrap \
  --var agent=<agent-name> \
  --var project_key=<project-key> \
  --var orchestrator=<agent-name>
```

Use these entries for the recurring coordination patterns. In this skill, work
unit means the atomic work being checked, feature means the stable CLI/schema
binding for a staged payload, bead means a Beads/br tracker issue, and a seal
is a Burpvalve attestation.

| Prompt | Use |
|---|---|
| `commit-choreography` | Paste-safe numbered commit flow for a staged work unit. |
| `verifier-brief` | Rules of engagement for feature x condition verifier cells. |
| `verifier-bootstrap` | Arrival prompt for a fresh verifier joining a coordinated project. |
| `verifier-packet-relay` | Relay instructions for forwarding verifier packets to peer panes. |
| `marching-orders` | Start-of-session brief for a coder taking a work unit. |
| `orchestrator-tick` | Coordinator observe, assign, unblock, and log loop. |
| `bead-conversion` | Convert approved planning material into scoped Beads tracker issues. |

If `burpvalve prompts` is unavailable, do not invent replacements. First repair
the binary install using `INSTALL.md`; until then, keep handoffs explicit and
minimal: feature id or work-unit id, staged payload hash, condition ids,
read-only expectation, response schema, and the Agent Mail thread or equivalent
audit reference.

## Installation And PATH

The normal installed shape is:

```text
~/.local/bin/burpvalve
<skills-dir>/burpvalve/scripts/bin/burpvalve
```

The user-bin command is a copied executable, not a symlink into the skill tree.
Moving or reorganizing the skill directory should not break `burpvalve` on
`PATH`.

After installation, verify the global command:

```bash
command -v burpvalve
burpvalve --version
burpvalve -h
burpvalve completion --color never
burpvalve completion verify --json
```

The git hook expects `burpvalve` on `PATH`. A repo-local `bin/burpvalve` is only
an optional fallback for unusual hook environments. Do not assume it is created
by default. Use `--repo-bin` or `burpvalve repair --force --json bin` only when
the repo explicitly needs that fallback.

Use `burpvalve completion verify --json` for read-only PATH/completion checks.
It reports command origin (`path`, `repo-local`, or `missing`), completion file
status, shell startup wiring, config sources, and next steps.

## Scaffold Pieces

The standard scaffold may include:

- `AGENTS.md`
- one explicit Claude route:
  - ordinary-agent route: `CLAUDE.md` points to `AGENTS.md`
  - orchestrator-skill route: `CLAUDE.md` points Claude to
    `.claude/skills/burpvalve-orchestrator/SKILL.md`
  - no route: Burpvalve leaves `CLAUDE.md` absent or untouched unless
    explicitly targeted later
- `.beads/` when `br` is available and not skipped
- optional `ntm quick` registration when NTM is available and not skipped
- `docs/`
- `plans/`
- `log/`
- `backpressure/`
- `.githooks/pre-commit`
- `tools/burpvalve/` documentation

The optional repo-local fallback is:

- `bin/burpvalve`

Useful target names include `AGENTS.md`, `CLAUDE.md`, `docs`, `plans`, `log`,
`backpressure`, `attestations`, `beads`, `ntm`, `hooks`, `precommit`,
`hooks-path`, `tool-docs`, `orchestrator`, `ORCHESTRATOR.md`, and
`bin/burpvalve`.

Useful skip flags include `--no-beads`, `--no-ntm`, `--no-claude`,
`--no-claude-symlink`, `--no-agents`, `--no-agents-md`, `--no-docs`,
`--no-plans`, `--no-log`, `--no-backpressure`, `--no-attestations`,
`--no-hooks`, `--no-git-hooks`, `--no-precommit`, `--no-hooks-path`,
`--no-tool-docs`, and `--no-bin`.

## Repair Behavior

`burpvalve repair AGENTS.md` appends missing Burpvalve sections to an existing
`AGENTS.md` instead of replacing it.

`burpvalve init` and `burpvalve repair` use an explicit Claude route. The
ordinary-agent route creates or repairs the `CLAUDE.md -> AGENTS.md` symlink.
The orchestrator-skill route creates or repairs a regular `CLAUDE.md` bootstrap
plus `.claude/skills/burpvalve-orchestrator/`. The no-route choice disables
Claude-route writes for that command.

Repair preserves an existing regular `CLAUDE.md` by default. To adopt that file
into a Burpvalve-managed route, pass the explicit adoption selector documented
by `burpvalve repair -h` / `burpvalve --robots repair -h`; adoption imports the
old content into `AGENTS.md` before applying the selected route. Do not assume
every regular `CLAUDE.md` is invalid or automatically replaced.

`ORCHESTRATOR.md` is an optional sidecar for project-local coordinator notes.
It can point to the Claude orchestrator skill when both exist, but `AGENTS.md`
remains the canonical coding-agent contract.

## Commit Gate Rule

Do not fabricate subagent or verifier confirmation.

When Burpvalve asks for verifier cells, answer only with evidence that exists
for the current staged payload and condition. If your runtime can spawn
read-only verifier subagents, do so for the relevant feature/condition cells. If
it cannot, report that limitation and let the gate block or record a blocker.

When `burpvalve verifier prompts` is available, use it to generate the handoff
packet for each verifier cell before spawning or messaging reviewers:

```bash
burpvalve verifier prompts --feature <feature-or-bead-id> --json
burpvalve verifier prompts --feature <feature-or-bead-id> --condition dry
```

The command is read-only. It does not spawn native subagents, send NTM tasks, or
claim verification happened. It only packages the staged paths, condition file,
manifest verifier policy, approved authorization language, and response schema
that real verifier evidence must satisfy.

For peer-pane verifier relay through NTM, use `burpvalve prompts show
verifier-packet-relay` together with the local repo's `docs/ntm-bridge.md`
policy. NTM and Agent Mail are coordination substrates; Burpvalve does not send
mail, spawn panes, monitor panes, or convert terminal wakes into verifier
evidence. Every feature x condition cell still needs a real hash-bound verdict.

For JSON response files, prefer the `verifier` object over the legacy
`subagent_confirmed` boolean:

```json
{
  "condition_id": "lint-rules",
  "verifier_policy": "independent_required",
  "verifier": {
    "kind": "independent_subagent",
    "model": "gpt-5",
    "runtime": "codex-cli",
    "separate_context": true
  },
  "verdict": "pass",
  "evidence": ["go test ./... passed"]
}
```

Use `verifier.kind: "main_agent"`, `"ci"`, or `"human"` only when the
condition's manifest policy allows that evidence type. Legacy
`subagent_confirmed: true` still maps to an independent verifier when
`verifier.kind` is omitted.

For non-interactive commit checks, prefer:

```bash
burpvalve commit --feature <feature-or-work-unit-id> --bead <bead-id> --responses responses.json
burpvalve lint
```

The seal/attestation is bound to the staged payload and condition files. If
staged files change, rerun the commit gate and stage the new attestation it
writes.

For Beads-backed delivery work units, use the dry-run preflight helper before
closing the bead:

```bash
burpvalve beads preflight <bead-id> --json
```

It reads `br show --json` and staged Git paths, then prints the safe order. It
does not close beads, run `br sync`, stage files, commit files, or write
attestations. Close delivery beads only after the staged work unit and verifier
evidence are ready, then run `br sync --flush-only`, stage `.beads/issues.jsonl`
with the payload, and run `burpvalve commit --bead <bead-id>` against that exact
staged payload. If one payload intentionally covers multiple beads, repeat
`--bead` and include `--bead-rationale`; do not combine unrelated work units
just because multiple beads are open.

## Extending Backpressure

When the user asks to add optional backpressure, tune lint rules, define
anti-reward-hacking policy, or make a condition more deterministic, read
`references/deterministic-backpressure.md` before editing project files.

That reference explains how to ask about lint commands, local versus CI
enforcement, anti-reward-hacking strictness, and whether a condition should stay
as written policy or become an executable gate. Do not add developer
dependencies, alter CI, or tighten destructive-operation rules unless the user
approved that scope.

## Validation

For a repo-level smoke check:

```bash
burpvalve setup --json
burpvalve config --json
burpvalve lint
```

For skill/package validation, use `SELF-TEST.md`.
