# 14 - Project Readiness Setup Pattern

> Plan for a Matt Pocock-style setup skill that makes a repository ready for agentic work before implementation begins. This is a scaffold/operating-contract plan, not the implementation of the skill itself.

> Current status (2026-06-21): this file is historical planning evidence. The
> current product and CLI name is `burpvalve`; use `README.md`,
> `docs/ARCHITECTURE.md`, `docs/release-install.md`, and
> `docs/CHANGELOG.md` as the current operational documentation. Older references here to
> setup-era command names, old commit-gate names, earlier working titles, or
> project registry setup describe earlier planning constraints, not the current
> CLI surface.

## Status

**Planning target:** create a reusable setup workflow, likely a Codex/Hermes/JSM skill, that initializes a project with the minimum substrate agents need to work safely:

- beads issue tracking;
- NTM project/session registration;
- `AGENTS.md` as the repo-local operating contract;
- `CLAUDE.md` symlinked to `AGENTS.md`;
- standard `/docs`, `/plans`, `/log`, and `/backpressure` directories;
- stub backpressure files that explain what each gate is for without pretending rules already exist.
- a pre-commit backpressure verifier that blocks commits until every feature/backpressure-condition pair has an explicit pass, not-applicable, or failure message with evidence.

This pattern is inspired by `mattpocock/skills`' setup skill: configure the local project once, then every later workflow can rely on explicit conventions instead of rediscovering them.

## Why This Exists

Agent-ready projects fail in predictable ways when they start from an empty repo:

- work is tracked in chat instead of a durable issue graph;
- NTM sessions use names that do not match project directories;
- agents look for `CLAUDE.md`, `AGENTS.md`, docs, or plans and find different answers;
- safety rules live in one model's memory instead of repo files;
- backpressure is discussed only after an agent has already overproduced bad work;
- agents call work "done" without evidence, gates, logs, or review artifacts.

The setup skill should make the first five minutes of a project boring and repeatable. It does not decide product strategy. It creates the working substrate.

## Product Shape

Create a user-invoked setup skill, tentatively:

```text
setup-command-project
```

It should be run once near project creation, and rerun safely as an idempotent repair/check command.

The skill should support three modes:

| Mode | Purpose | Mutation |
|---|---|---|
| `check` | Inspect current readiness and report gaps. | None |
| `init` | Create missing files/directories and initialize beads/NTM. | Yes |
| `repair` | Fix drift: missing symlink, missing dirs, stale stubs, broken beads config. | Yes, with confirmation for overwrites |

It should also install or configure a commit-time verifier, likely as a small Go CLI with a thin Git hook wrapper:

```text
burpvalve
```

## Implementation Source Layout

Use a source-first implementation model:

```text
<workspace>/burpvalve/
  cmd/
    setup-command-project/
    burpvalve/
  internal/
    scaffold/
    attestations/
    gitindex/
    lintconfig/
  templates/
    AGENTS.md.tmpl
    backpressure/
    githooks/
  fixtures/
  docs/
  plans/
  skill/
    SKILL.md
    scripts/bin/burpvalve
```

The Go code should be the deterministic engine. It should inspect the target repo, create missing files, insert managed text non-destructively when absent, preserve existing user content, install or wrap the pre-commit hook, initialize/verify beads, and build/copy `burpvalve` into the target repo.

The skill should be the agent-facing entrypoint and procedural wrapper. Canonical JSM source should live at:

```text
<workspace>/jsm-skills/skills/setup-command-project/
```

The skill may include a script that invokes the Go setup tool:

```bash
burpvalve setup --json
```

Do not make a binary-only skill package the source of truth. Prefer bundling Go source or invoking the local `<workspace>/burpvalve` source so builds are inspectable, reproducible, and portable across Linux/macOS and arm64/amd64. A compiled binary may be used as a local cache, but source and version must remain visible.

Binary size should be treated as a product constraint. These CLIs should mostly use the Go standard library, so a stripped single-platform binary should stay small. Build with `-trimpath -ldflags "-s -w"` for release/local-cache artifacts. Success means each compiled `setup-command-project` and `burpvalve` artifact is no larger than 12 MiB. A build larger than 8 MiB should print a size warning and list the largest linked dependencies; a build larger than 12 MiB should fail unless the implementation includes a documented exception explaining why the extra dependency is necessary. Do not commit multi-platform binary artifacts into target repos. If the setup process installs a binary into a target repo, prefer an ignored local path such as `.git/backpressure/bin/` or a user cache and have the hook call that path.

The Git hook should not contain complicated orchestration logic. The hook should call the CLI, and the CLI should own feature detection, condition loading, attestation prompts, evidence capture, and exit codes.

The CLI does not need to integrate with native subagents directly. Its job is to block the commit and ask the committing agent to attest that it spawned the required subagents and that each feature/condition pair passed. If the agent cannot truthfully attest that, the CLI exits non-zero and prints the exact verifier work the agent must do before trying the commit again.

Default behavior should be conservative:

- If files already exist, do not overwrite them.
- If a generated stub exists and has user content below its description, preserve it.
- If `CLAUDE.md` exists and is not a symlink, stop and ask before changing it.
- If `br init` has already run, do not reinitialize.
- If NTM cannot be verified, write a clear follow-up note rather than faking readiness.
- If a commit contains more than one feature or more than one bug fix, refuse and ask the agent to split the work.

## Generated Tree

The desired project root after setup:

```text
.
|-- .beads/
|-- AGENTS.md
|-- CLAUDE.md -> AGENTS.md
|-- .githooks/
|   `-- pre-commit
|-- tools/
|   `-- burpvalve/
|-- docs/
|   `-- README.md
|-- plans/
|   `-- README.md
|-- log/
|   |-- README.md
|   `-- backpressure/
|       `-- failed/
|           `-- README.md
`-- backpressure/
    |-- manifest.yaml
    |-- attestations/
    |   `-- README.md
    |-- README.md
    |-- lint-rules.md
    |-- dry.md
    |-- anti-reward-hacking.md
    |-- one-function-one-test.md
    |-- definition-of-done.md
    |-- evidence-log.md
    |-- scope-control.md
    |-- destructive-operations.md
    |-- data-integrity.md
    |-- security-boundaries.md
    |-- visual-regression.md
    |-- performance-budget.md
    `-- autonomy-boundary.md
```

The setup skill should allow project-specific additions, but this is the baseline.

## File Contracts

### `AGENTS.md`

`AGENTS.md` is the canonical repo-local operating contract for all coding agents.

It should include:

- project purpose in 2-4 sentences;
- quickstart commands;
- build/test/lint commands, even if marked unknown;
- beads workflow rules;
- NTM session naming rules;
- atomic feature/commit rules;
- backpressure directory pointer;
- definition of done;
- where docs/plans/logs live;
- how agents should handle uncertainty;
- how to coordinate file edits.

Minimum content:

```md
# Agent Operating Contract

## Project Purpose

TBD. Replace this with the concrete purpose of the project.

## Agent Startup

1. Read this file.
2. Run `br ready --json` before inventing work.
3. Check `/backpressure/README.md` before calling work done.
4. Record durable decisions in `/docs/` or `/plans/`, not chat.

## Commands

- Build: TBD
- Test: TBD
- Lint: TBD
- Run locally: TBD

## Beads

- Use `br ready --json` to find work.
- Use `br update <id> --status in_progress` when claiming work.
- Use `br close <id> --reason "..."`
- Run `br sync --flush-only` before committing `.beads/` changes.
- `br dep cycles` must be empty.

## Atomic Work And Commits

- One feature, one bead, one commit.
- One bug, one fix, one commit.
- Do not combine two features, two fixes, or a feature plus a fix in the same commit.
- All plans must include a single commit bead for each atomic feature or bug fix. No multi-feature or multi-fix commit beads.
- If staged changes contain multiple atomic units, split the commit before running the backpressure verifier.

## NTM

NTM session name should match the project directory basename unless there is an explicit label, e.g. `<project>` or `<project>--backend`.

## Backpressure

Backpressure rules live in `/backpressure/`. A task is not done because an agent says it is done; it is done when the relevant checks have accepted the artifact or the blocker is explicit.

Before committing feature work, run the backpressure verifier. The verifier must confirm every active backpressure condition for every changed feature, or attach a failure/blocker message for each unmet condition.

## Definition Of Done

- Relevant checks run and results recorded.
- Backpressure verifier has a complete feature x condition matrix.
- Bead closed or blocker recorded.
- Files committed when appropriate.
- Remaining risks or follow-ups named.
```

### `CLAUDE.md`

`CLAUDE.md` should be a symlink to `AGENTS.md`:

```bash
ln -s AGENTS.md CLAUDE.md
```

Rationale:

- Claude Code reliably reads `CLAUDE.md`.
- Other agents commonly read `AGENTS.md`.
- One canonical file avoids drift.

If symlinks are not supported in the environment, the setup skill may copy `AGENTS.md` to `CLAUDE.md`, but must add a warning at the top of both files that the copy can drift.

### `/docs/`

`docs/` stores durable project knowledge:

- architecture notes;
- domain glossary;
- ADRs or decision notes;
- operational runbooks;
- external research summaries that should survive chat context.

Initial `docs/README.md`:

```md
# Docs

Durable project knowledge lives here: architecture, domain vocabulary, decisions, runbooks, and research summaries. Do not use this directory as a task list; actionable work belongs in beads.
```

### `/plans/`

`plans/` stores strategic and implementation plans.

Initial `plans/README.md`:

```md
# Plans

Plans live here. A plan explains what should be built and why. Actionable work from a plan should be converted into beads with dependencies, so agents can execute without re-reading the whole plan.
```

### `/log/`

`log/` stores human/agent work logs, debugging notes, and post-run summaries that are useful but not canonical architecture.

Initial `log/README.md`:

```md
# Log

Use this directory for dated work logs, investigation notes, run summaries, and debugging transcripts. Promote durable decisions to `/docs/` or `/plans/`; do not let logs become the canonical source of truth.
```

Initial `log/backpressure/failed/README.md`:

```md
# Failed Backpressure Attempts

Local blocked-attempt reports live here. These files help an agent recover from a failed pre-commit check, but they are not passing commit attestations and should not be treated as evidence that a commit is ready.
```

## Backpressure Directory

The setup skill creates blank backpressure files with descriptions only. These are placeholders for project-specific rules and gates. The point is to reserve the categories early without inventing fake enforcement.

Backpressure files are also commit-time conditions. Each active file becomes a verifier condition unless `backpressure/manifest.yaml` marks it disabled or profile-specific.

### `backpressure/manifest.yaml`

```yaml
# Backpressure condition registry.
# Each enabled condition is checked against every changed feature before commit.
conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
  - id: dry
    path: backpressure/dry.md
    enabled: true
  - id: anti-reward-hacking
    path: backpressure/anti-reward-hacking.md
    enabled: true
  - id: one-function-one-test
    path: backpressure/one-function-one-test.md
    enabled: true
  - id: definition-of-done
    path: backpressure/definition-of-done.md
    enabled: true
  - id: evidence-log
    path: backpressure/evidence-log.md
    enabled: true
  - id: scope-control
    path: backpressure/scope-control.md
    enabled: true
  - id: destructive-operations
    path: backpressure/destructive-operations.md
    enabled: true
  - id: data-integrity
    path: backpressure/data-integrity.md
    enabled: true
  - id: security-boundaries
    path: backpressure/security-boundaries.md
    enabled: true
  - id: visual-regression
    path: backpressure/visual-regression.md
    enabled: true
  - id: performance-budget
    path: backpressure/performance-budget.md
    enabled: true
  - id: autonomy-boundary
    path: backpressure/autonomy-boundary.md
    enabled: true

# Executable lint/format/static-analysis commands.
# These are separate from the `lint-rules` backpressure attestation.
lint_commands:
  - id: example-lint
    command: "TBD"
    required: false
    paths: ["."]
    timeout_seconds: 120
```

### `backpressure/README.md`

```md
# Backpressure

Backpressure is the set of checks, gates, tests, reviews, and evidence requirements that prevent agents from self-certifying work. Fill these files with project-specific rules as the project matures.

A task is complete only when the relevant backpressure files say what must pass, and the agent records evidence that it passed or names the blocker clearly.
```

### `backpressure/attestations/README.md`

```md
# Commit Backpressure Attestations

Tracked passing commit attestations live here. Each successful feature or bug-fix commit must include one JSON artifact binding the atomic work item, staged payload hash, enabled backpressure conditions, subagent confirmations, verdicts, and messages.

Do not put blocked-attempt reports here. Blocked attempts belong in `/log/backpressure/failed/`.
```

### `backpressure/lint-rules.md`

```md
# Lint Rules

Purpose: project-specific formatting, linting, static analysis, and style gates. Add exact commands and failure-handling rules here.

Suggested policy candidates to enable when the project has tooling for them:

- cap nested `if`/loop depth;
- prefer early returns over deep nesting;
- forbid unexplained magic numbers;
- require descriptive function, method, variable, and package names;
- set minimum identifier length, with explicit exceptions for idiomatic short names such as loop indexes or coordinates;
- cap function length and cyclomatic/cognitive complexity;
- require errors to be handled explicitly;
- reject dead code, unused exports, and broad catch-all exception handling;
- require exact lint/format/test commands before a rule becomes enforced.

Setup should create this as a policy wishlist, not as fake enforcement. A rule is enforced only after this file names the command or analyzer that checks it.

Important distinction:

- `lint-rules` as a backpressure condition asks a subagent to verify that applicable lint/style rules were considered for the feature.
- `burpvalve lint` runs executable lint/format/static-analysis commands declared by the project.

The first is an attestation. The second is a command runner. A commit should satisfy both when executable lint commands exist.
```

### `backpressure/dry.md`

```md
# DRY - Do Not Repeat Yourself

Purpose: prevent duplicate logic, copy-pasted branches, repeated validation, repeated API/client code, repeated prompt text, and parallel implementations of the same behavior.

The DRY subagent should check the staged feature for:

- copied code blocks that should be extracted or shared;
- duplicated conditionals, loops, validation rules, error handling, or mapping logic;
- repeated constants, strings, prompts, or magic numbers that need names or central definitions;
- new helpers that duplicate existing helpers;
- nearly identical tests that should use tables, fixtures, or shared builders;
- abstractions added too early that hide duplication instead of removing it.

The subagent should pass only when repetition is intentional, local, and justified, or when the implementation removes meaningful duplication without creating a worse abstraction.

If the condition is not met, attach a message naming the duplicated areas, the preferred extraction or consolidation, and whether the commit should be split before refactoring.
```

### `backpressure/anti-reward-hacking.md`

```md
# Anti-Reward-Hacking

Purpose: prevent agents from optimizing for superficial success signals while violating the real goal. Add examples of forbidden shortcuts, fake completion patterns, metric gaming, and required evidence.
```

### `backpressure/one-function-one-test.md`

```md
# One Function One Test

Purpose: define when new or changed behavior requires direct test coverage. Use this to prevent broad implementation changes with no nearby behavioral test.
```

This file should not literally require one test for every trivial helper forever. It exists to force the project to state its test granularity rule.

### `backpressure/definition-of-done.md`

```md
# Definition Of Done

Purpose: define what "done" means in this project: checks, review, logs, commits, beads, PRs, deployment evidence, and known-risk notes.
```

### `backpressure/evidence-log.md`

```md
# Evidence Log

Purpose: define what evidence agents must record before closing work: command output, screenshots, preview URLs, curl results, CI links, benchmark output, or manual verification notes.
```

### `backpressure/scope-control.md`

```md
# Scope Control

Purpose: prevent agents from expanding tasks without approval. Add rules for when to create a new bead, when to stop, and how to report adjacent issues.
```

### `backpressure/destructive-operations.md`

```md
# Destructive Operations

Purpose: define gates for risky commands and irreversible changes: deletes, resets, force pushes, data migrations, external sends, spend changes, deploys, and production mutations.
```

### `backpressure/data-integrity.md`

```md
# Data Integrity

Purpose: define invariants for persisted data, migrations, idempotency, dedupe, rollback, backups, and audit trails.
```

### `backpressure/security-boundaries.md`

```md
# Security Boundaries

Purpose: define auth, secrets, tenant isolation, PII, permission, sandbox, and external API safety rules.
```

### `backpressure/visual-regression.md`

```md
# Visual Regression

Purpose: define screenshot, browser, responsive layout, canvas, and visual-diff checks for UI work.
```

### `backpressure/performance-budget.md`

```md
# Performance Budget

Purpose: define latency, memory, token, cost, bundle-size, throughput, or benchmark thresholds that should push back on changes.
```

### `backpressure/autonomy-boundary.md`

```md
# Autonomy Boundary

Purpose: prevent unnecessary human approval loops. Define what the agent is already authorized to do, what must be handled by existing policy gates, and the narrow cases where work must stop with a blocker because it is outside standing authority.
```

## Pre-Commit Backpressure Gate

This setup pattern should install a hard pre-commit gate. The gate must block the commit unless every changed feature has been checked against every enabled backpressure condition.

The recommended implementation is:

1. `.githooks/pre-commit` is a thin shell wrapper.
2. The wrapper calls a compiled or `go run` CLI, tentatively `burpvalve`.
3. The CLI computes a feature x condition matrix.
4. The CLI asks the committing agent to attest that each matrix cell was checked by a dedicated subagent.
5. The CLI requires a pass, not-applicable reason, or failure/blocker message for every matrix cell.
6. The CLI writes a tracked attestation artifact under `backpressure/attestations/` for successful commits.
7. The CLI writes a local blocked-attempt report under `log/backpressure/failed/` when the commit cannot pass.
8. The CLI exits non-zero until the matrix is complete, all cells are pass or not-applicable, and the attestation artifact is staged.

Example hook:

```bash
#!/usr/bin/env bash
set -euo pipefail

go run ./tools/burpvalve commit
go run ./tools/burpvalve lint
```

The actual hook may call a built binary for speed:

```bash
./bin/burpvalve commit
./bin/burpvalve lint
```

The setup skill should configure Git to use the repo-local hook path:

```bash
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit
```

### Hook Interactivity Contract

A pre-commit hook can execute a CLI, but interactive prompting is not guaranteed in every environment. Terminal `git commit` usually has a controlling TTY available. GUI clients, IDE commits, automation, and CI often do not.

The hook/CLI should therefore follow this rule:

- If `/dev/tty` is available, `burpvalve commit` may open `/dev/tty` for prompts.
- If no TTY is available, pre-commit mode must fail closed with a clear message and instructions for running the verifier manually.
- CI mode must never prompt.

The CLI should start with an entry message before any questions:

```text
Backpressure pre-commit gate

This commit is blocked until every enabled backpressure condition has a subagent attestation for the staged atomic feature or bug fix.

If you have not spawned the required verifier subagents yet, answer "no" or stop now, run those subagents according to /backpressure/, then retry the commit.

This CLI will generate backpressure/attestations/<staged-payload-hash>.json for passing checks. Stage that file and rerun commit.
```

### Hook Composition

Setup should install one repo-local pre-commit dispatcher at `.githooks/pre-commit`.

The dispatcher should run:

1. `burpvalve commit`;
2. project lint/format/static-analysis commands declared in `backpressure/lint-rules.md` or `backpressure/manifest.yaml`;
3. any existing project-specific hook commands that the setup skill preserved.

Because the shell uses `set -e`, lint mode should run only after the backpressure attestation gate succeeds. If pre-commit mode generates a new attestation artifact and exits non-zero so the agent can stage it, lint mode should not run during that attempt. This avoids showing lint noise before the hard attestation gate is satisfied.

Setup should not invent language-specific linters for a project with unknown tooling. It may seed recommended lint policies in `backpressure/lint-rules.md`, but enforcement begins only when the project declares exact commands or analyzers. If an existing pre-commit hook is present, setup should preserve it and either wrap it or ask before replacing it.

### Tracked Attestation Artifact

Successful commits should include a tracked attestation artifact:

```text
backpressure/attestations/<staged-payload-hash>.json
```

This file is part of the commit. It lets reviewers and CI inspect what the committing agent claimed, which subagent checks were run, which conditions were marked not applicable, and why.

The CLI should generate the artifact before commit and then verify it is staged. It should not silently stage files by default. Safer default behavior:

1. Generate or update `backpressure/attestations/<staged-payload-hash>.json`.
2. Exit non-zero with a message telling the agent to stage that file.
3. On the next commit attempt, verify the staged attestation file matches the staged payload and condition files.
4. Allow the commit only when the artifact is staged and valid.

The CLI may support an explicit opt-in flag such as `--stage-attestation`, but automatic staging should not be the default because hidden index mutation makes review harder.

The staged payload hash must exclude generated attestation and blocked-attempt artifacts so the attestation does not hash itself. Define it as a stable hash of staged paths excluding:

- `backpressure/attestations/**`;
- `log/backpressure/failed/**`;
- other generated verifier outputs declared by the tool.

This avoids self-referential churn where staging the attestation changes the hash that names or validates the attestation.

The attestation is an accountability record, not cryptographic proof that a subagent actually ran. The CLI records what the committing agent claimed, binds that claim to the staged payload and condition text, and makes missing or contradictory claims reviewable by humans and CI.

Blocked commits should write a local failure report:

```text
log/backpressure/failed/<timestamp>-blocked.json
```

The blocked report is for agent recovery. It should not be required in the commit because failed/unknown attestations should not pass the gate.

### Feature Detection

The verifier must identify changed features before building the matrix.

Preferred sources, in order:

1. Explicit CLI flag, e.g. `--feature <bead-id-or-name>`.
2. Open or in-progress bead referenced by the branch name, staged changes, or commit message draft.
3. `br list --json` / `br ready --json` metadata, if the bead names the feature.
4. A repo-local feature manifest, if one exists.
5. Staged diff clustering by directory or package.
6. Interactive agent prompt.

If the verifier cannot identify features deterministically, it must block and ask for explicit feature labels. It must not collapse multiple changed features into `misc` just to allow a commit.

If the verifier identifies more than one atomic feature or more than one atomic bug fix, it should block by default and ask the agent to split the staged changes. The feature x condition matrix still supports multiple features because the tool may be used in audit or repair mode, but ordinary feature commits should be one atomic unit.

### Matrix Rule

For each commit, let:

- `F` be the set of changed features.
- `C` be the set of enabled backpressure conditions.

The verifier must produce `|F| x |C|` cells.

If there are 2 changed features and 10 enabled conditions, the verifier owes 20 confirmations.

Each cell must end in exactly one of these states:

| State | Commit allowed? | Meaning | Required attachment |
|---|---:|---|---|
| `pass` | Yes | A dedicated subagent checked this condition for this feature and found it satisfied. | Evidence summary plus source agent |
| `not_applicable` | Yes | A dedicated subagent checked this condition and found it genuinely does not apply. | Reason this condition does not apply to this feature |
| `fail` | No | A dedicated subagent checked this condition and found the feature does not satisfy it. | Failure message, blocker, and recommended next action |
| `unknown` | No | The condition was not checked, the subagent output was missing/malformed, or evidence is insufficient. | Missing evidence message and verifier notes |

No cell may be inferred from another cell. If `lint-rules` passes for Feature A, it does not automatically pass for Feature B.

Do not treat "no" as a verdict. The CLI should ask two separate questions:

1. Did a dedicated subagent check this condition for this feature?
2. If yes, what was the verdict?

If the answer to the first question is no, the cell is `unknown`, the commit blocks, and the attestation message must explain why the subagent check did not happen.

### Subagent Attestation

The hard condition is: every feature/condition pair must be independently verified before feature work can commit.

For feature commits, strict mode is the default. The active coding agent should launch one verifier subagent per matrix cell unless there is a valid attestation artifact for that exact feature/condition pair and staged diff.

In audit or repair mode, 2 changed features with 10 enabled conditions means 20 verifier subagents. Ordinary feature commits should avoid this by containing one atomic feature or bug fix.

The expected normal case is one atomic feature per commit, so the common case with 10 enabled conditions is 10 verifier subagents.

The Go CLI should be a blocking checklist and evidence ledger. It does not need to spawn subagents itself. It should expose a simple contract:

```text
burpvalve commit
```

The CLI should:

1. detect the staged feature or bug fix;
2. load enabled conditions from `backpressure/manifest.yaml`;
3. print one checklist row per feature/condition pair;
4. ask the committing agent whether a dedicated subagent checked that row;
5. ask for `pass`, `not_applicable`, `fail`, or `unknown`;
6. require an explanatory message for `not_applicable`, `fail`, or `unknown`;
7. write a tracked attestation artifact for complete pass/not-applicable matrices;
8. write a local blocked-attempt report for `fail`, `unknown`, missing subagent confirmation, or multiple atomic units;
9. exit non-zero unless the tracked attestation artifact is staged and valid.

The question sequence should be fixed:

1. Entry banner: explain the gate, artifact path, and fail-closed behavior.
2. Atomicity confirmation: show detected feature/bug bead and ask whether the staged diff contains exactly one atomic feature or one atomic bug fix.
3. If atomicity is false, block and ask the agent to split the commit.
4. For each enabled condition, in manifest order:
   - show feature id/name;
   - show condition id and condition file path;
   - ask whether a dedicated subagent checked this exact condition for this exact feature;
   - if no, mark `unknown`, require a message explaining why, write a blocked-attempt report, and block;
   - if yes, ask for verdict: `pass`, `not_applicable`, `fail`, or `unknown`;
   - if `not_applicable`, require the reason;
   - if `fail` or `unknown`, require blocker, evidence, and next action, then block;
   - if `pass`, ask for concise evidence summary.
5. Summary: print the full matrix before writing artifacts.
6. Artifact step: write the tracked passing attestation or local blocked-attempt report.
7. Staging check: if the passing attestation is not staged, block and tell the agent the exact `git add` command to run.

Example interactive prompt:

```text
Feature: checkout discounts
Condition: anti-reward-hacking

Confirm: did you spawn a dedicated subagent to check this condition for this feature? [y/N]
Verdict: pass | not_applicable | fail | unknown
Message required unless verdict is pass:
```

If the agent cannot answer `y` truthfully, it must stop the commit, spawn the needed subagent according to the relevant file in `/backpressure/`, then retry the commit.

Model selection should be configurable by condition. Examples:

```yaml
models:
  default: sonnet
  high_reasoning: gpt-5.4
condition_models:
  dry: high_reasoning
  anti-reward-hacking: high_reasoning
  security-boundaries: high_reasoning
  data-integrity: high_reasoning
```

The plan should not hard-code provider availability. `sonnet`, `gpt-5.4`, or any other model name should be treated as local model aliases that the active agent runtime or project config resolves. The CLI may print the preferred model alias for a condition, but the committing agent owns spawning the actual subagent.

Each sub-agent prompt must include:

- the feature name and source bead, if available;
- the condition file contents;
- the staged diff or relevant file list;
- exact required output schema;
- instruction to report `pass`, `not_applicable`, `fail`, or `unknown`;
- instruction to attach a message when the condition is not met.

Expected subagent result that the committing agent should summarize into the CLI:

```json
{
  "feature": "checkout discounts",
  "condition": "anti-reward-hacking",
  "status": "fail",
  "agent": "sonnet",
  "message": "The implementation updates the success metric without proving the discount was applied to the persisted order.",
  "evidence": [
    "staged diff: internal/checkout/discounts.go",
    "missing test for persisted order total"
  ],
  "next_action": "Add a test proving order total persistence and rerun verifier."
}
```

The CLI should aggregate successful results into a tracked commit artifact:

```text
backpressure/attestations/<staged-payload-hash>.json
```

The CLI should aggregate blocked attempts into local recovery artifacts:

```text
log/backpressure/failed/<timestamp>-blocked.json
```

### Evidence Binding, Not A Backpressure Category

The verifier should bind attestations to the staged diff so an agent cannot reuse an old "yes" after changing files.

This should not be a separate backpressure file. It is part of the CLI's integrity check.

Minimum binding fields:

- feature id or bug id;
- condition id;
- staged payload hash;
- condition file hash;
- subagent model alias, if known;
- verdict;
- message;
- timestamp.

If the staged diff or condition file changes, previous attestations for that cell are stale and the CLI should ask again.

Tracked attestation artifacts should use an explicit schema, for example:

```json
{
  "schema_version": 1,
  "tool": "burpvalve",
  "tool_version": "0.1.0",
  "staged_payload_hash": "sha256:...",
  "manifest_hash": "sha256:...",
  "condition_order": [
    "lint-rules",
    "dry",
    "anti-reward-hacking"
  ],
  "generated_by": {
    "agent": "codex",
    "model": "gpt-5"
  },
  "git_head_before_commit": "abc123",
  "created_at": "2026-06-20T12:00:00Z",
  "feature": {
    "id": "br-123",
    "kind": "feature",
    "name": "checkout discounts"
  },
  "atomicity": {
    "one_feature_or_fix": true,
    "message": "Staged changes map only to br-123."
  },
  "conditions": [
    {
      "condition_id": "anti-reward-hacking",
      "condition_file": "backpressure/anti-reward-hacking.md",
      "condition_file_hash": "sha256:...",
      "subagent_confirmed": true,
      "subagent_model": "sonnet",
      "verdict": "pass",
      "message": "",
      "evidence": [
        "internal/checkout/discounts.go",
        "subagent found persisted order total covered by TestDiscountPersistsOrderTotal"
      ],
      "next_action": ""
    }
  ]
}
```

For a passing commit, every condition entry must have `subagent_confirmed: true` and `verdict` equal to `pass` or `not_applicable`. For `not_applicable`, `message` is mandatory. For `fail`, `unknown`, or `subagent_confirmed: false`, the artifact may be written as a blocked-attempt report, but it must not be accepted as the tracked commit attestation.

Reviewer discovery rule:

- If there is no tracked attestation artifact, the commit failed the backpressure protocol.
- If a condition has `subagent_confirmed: false`, the commit failed the backpressure protocol.
- If a condition has `subagent_confirmed: false` without a message, the agent failed to explain the missing check.
- If a condition is `not_applicable` without a message, the agent failed to justify the exemption.
- If a condition is `fail` or `unknown` in a tracked passing artifact, the commit should be rejected even if the hook was bypassed.

### CI Verification

CI should be non-interactive. It should not ask an agent questions and should not spawn subagents.

CI should run a command such as:

```bash
burpvalve ci
```

In CI mode, the CLI should:

1. find the tracked attestation artifact for the commit or tree;
2. verify it is committed, not only present in the working tree;
3. recompute the committed payload hash used by the artifact;
4. recompute every enabled condition file hash;
5. recompute the manifest hash and condition order;
6. verify there is exactly one atomic feature or bug fix unless running audit/repair mode;
7. verify every enabled condition has one cell;
8. verify every accepted cell has `subagent_confirmed: true`;
9. verify every accepted cell has verdict `pass` or `not_applicable`;
10. verify every `not_applicable` cell includes a message;
11. fail if any `fail`, `unknown`, missing message, missing cell, stale hash, or multi-feature normal commit is found.

This makes reviewer discovery mechanical: a reviewer or CI job can inspect one committed JSON artifact and know whether the commit had a complete backpressure attestation.

### NTM Role

NTM remains useful for the setup pattern, but its role should be limited:

- registering or resolving the project/session;
- running a small top-level orchestrator swarm, usually 2-10 agents;
- supervising long-running implementation work;
- giving humans a tmux dashboard for the project;
- coordinating beads, locks, mail, and handoffs when multiple top-level agents are active.

NTM should not be the default mechanism for spawning one verifier per feature/condition cell. If NTM is used for backpressure, it should usually run one top-level reviewer/coordinator pane that asks the active agent to use native subagents, or it should run a small fixed number of reviewers that batch cells.

If a project explicitly uses NTM for backpressure coordination anyway, it must still obey the NTM evidence rule: verify state transitions with `ntm --robot-snapshot` and attention/tail evidence. A spawn/send exit code alone is not sufficient.

### Non-Interactive Commits

Pre-commit hooks run in environments where interactive prompting may be unavailable.

Rules:

- If `/dev/tty` is unavailable and required confirmations are missing, fail closed with instructions for running the verifier in an interactive terminal.
- If the agent cannot confirm that subagents checked every required condition, fail closed.
- If the user explicitly bypasses hooks with `--no-verify`, the repo should require CI or a later merge gate to run the same verifier.
- CI should run `burpvalve ci` so this gate is not only local.

### Failure Message Requirement

When a condition is not met, the verifier must attach a message. A bare failure is invalid.

The message must name:

- feature;
- condition;
- why it failed or why evidence is missing;
- files or commands involved;
- next action;
- whether a human decision is required.

The message should be written to the summary artifact and, when a bead exists, posted back to the bead or Agent Mail thread.

## Beads Initialization

The setup skill should initialize local-first issue tracking:

```bash
br init
br doctor
br config --list
br dep cycles
br sync --flush-only
```

Important rules:

- `br` does not run git commands; the setup skill must say exactly what changed.
- The initial `.beads/` should be committed with the scaffold.
- Use `br ready --json` and `br list --json` in agent-facing docs.
- Never run bare `bv`; use robot flags such as `bv --robot-triage`.
- If the repo already has `.beads/`, run `br doctor` instead of `br init`.

Initial beads to create, if the user wants the setup skill to seed work:

| Bead | Type | Purpose |
|---|---|---|
| Fill project purpose in `AGENTS.md` | task | Replace TBD project purpose and commands. |
| Define first backpressure gates | task | Fill `definition-of-done.md`, `lint-rules.md`, and `evidence-log.md`. |
| Add first real plan | task | Create a project-specific plan in `/plans/`. |
| Add initial architecture doc | task | Create `/docs/architecture.md` or equivalent if the project has code. |

The setup skill should ask before creating these beads. Some repos are intentionally empty scaffolds and should not get fake work items.

## NTM Registration

NTM project identity should be deterministic.

Project readiness should verify:

1. The project lives under the configured NTM projects base, or the user explicitly supplies a path.
2. The NTM session/project name equals the repo directory basename.
3. Labels use `--`, e.g. `<project>--frontend`, and do not become a different project identity.
4. Robot mode can see the project/session after setup.

Suggested setup flow:

```bash
basename "$PWD"
ntm --robot-capabilities
ntm quick "$(basename "$PWD")"
ntm --robot-snapshot
```

If `ntm quick` would create or mutate state unexpectedly, the setup skill should run a check mode first and tell the user the command it intends to run.

The resulting `AGENTS.md` should include:

```md
## NTM Session Naming

Use the repo basename as the base session name. Use labels only for variants:

- Base session: `<repo-name>`
- Labeled session: `<repo-name>--frontend`
- Labeled session: `<repo-name>--backend`

Before state-changing NTM commands, check `ntm --robot-capabilities` and verify state with `ntm --robot-snapshot`.
```

## Setup Workflow

### Phase 1 - Inspect

1. Confirm current directory is the target project root.
2. Detect git repo state.
3. Detect existing `.beads/`, `AGENTS.md`, `CLAUDE.md`, `docs/`, `plans/`, `log/`, and `backpressure/`.
4. Detect NTM availability via `ntm --robot-capabilities`.
5. Print a readiness report with planned changes.

### Phase 2 - Scaffold

1. Create missing directories.
2. Create missing README/stub files.
3. Create `AGENTS.md` if absent.
4. Create `CLAUDE.md` symlink to `AGENTS.md` if safe.
5. Initialize or verify beads.
6. Register/verify NTM project identity.
7. Optionally add project registry entry.
8. Install the pre-commit backpressure hook and verifier CLI scaffold.

### Phase 3 - Seed

Optional, user-confirmed:

1. Create initial beads for filling TBD content.
2. Create `/docs/architecture.md` if the project already has code.
3. Create `/plans/initial-plan.md` if the user has supplied a project direction.
4. Add project-specific commands to `AGENTS.md`.

### Phase 4 - Verify

Required checks:

```bash
test -f AGENTS.md
test -L CLAUDE.md || test -f CLAUDE.md
test -d docs
test -d plans
test -d log
test -d backpressure
test -d backpressure/attestations
br doctor
br dep cycles
ntm --robot-capabilities
git config --get core.hooksPath
test -x .githooks/pre-commit
go test ./tools/burpvalve/...
```

Optional checks:

```bash
br ready --json
bv --robot-triage
ntm --robot-snapshot
git status --short
```

## Idempotency Rules

The setup skill must be safe to run repeatedly.

| Existing state | Behavior |
|---|---|
| `AGENTS.md` exists | Do not overwrite; append missing standard sections only with explicit confirmation. |
| `CLAUDE.md` symlink correct | Leave as-is. |
| `CLAUDE.md` regular file exists | Compare with `AGENTS.md`; ask before replacing. |
| `.beads/` exists | Run `br doctor`; do not `br init`. |
| `backpressure/*.md` contains user content | Preserve content; do not reset to stub. |
| `backpressure/attestations/*.json` exists | Preserve historical artifacts; do not rewrite except for the current staged payload. |
| `.githooks/pre-commit` exists | Do not overwrite; compare and ask before replacing. |
| `tools/burpvalve/` exists | Run its tests/check mode; do not regenerate. |
| NTM project/session exists | Verify snapshot; do not respawn unless asked. |

## Anti-Patterns

- Creating a beautiful scaffold but no beads.
- Creating beads with vague titles and no acceptance criteria.
- Copying `AGENTS.md` into `CLAUDE.md` without drift warning.
- Filling backpressure stubs with generic rules that are not project-specific.
- Calling NTM registration done without a robot snapshot.
- Letting the pre-commit hook become a reminder instead of an enforced gate.
- Assuming pre-commit prompts always work without checking for `/dev/tty`.
- Letting one commit contain multiple features, multiple bug fixes, or a feature plus an unrelated fix.
- Treating one verifier result as covering multiple changed features.
- Allowing `unknown` backpressure cells to pass because the commit is small.
- Claiming a subagent checked a condition without attaching a verdict or message.
- Allowing a commit to pass without a staged tracked attestation artifact.
- Silently staging the attestation artifact without telling the agent.
- Treating a local blocked-attempt report as a passing commit attestation.
- Enforcing lint rules that are only wishlist text and have no command/analyzer.
- Passing the DRY condition when the feature introduces unreviewed duplicate logic or copy-pasted behavior.
- Creating broad abstractions merely to satisfy DRY when local duplication would be clearer.
- Using NTM panes as the default fanout mechanism for dozens of short verifier cells.
- Building the Go CLI as if it owns model invocation instead of making it a blocking attestation gate.
- Treating `AGENTS.md` as a task backlog.
- Treating `/log/` as canonical architecture.
- Creating a project registry entry without making ambiguity/identity explicit.
- Re-running setup and overwriting project-specific rules.

## Acceptance Criteria

The setup pattern is successful when:

- A fresh repo can be made agent-ready in one run.
- A fresh agent can read `AGENTS.md` and know where work, docs, logs, and gates live.
- `CLAUDE.md` points to the same contract as `AGENTS.md`.
- `br doctor` passes or reports a clear blocker.
- `br dep cycles` returns no cycles.
- NTM project/session identity is documented and verifiable.
- Backpressure files exist as named categories with descriptions, not fake completed rules.
- Atomic work rules are present: one feature or one bug fix per bead and per commit.
- Compiled Go CLI artifacts are source-backed, stripped, and within the documented size budget, with no multi-platform binaries committed into target repos.
- The pre-commit hook blocks commits until the feature x condition matrix is complete.
- The CLI has a clear entry banner and fixed question sequence.
- The hook handles TTY and non-TTY commit contexts safely.
- For feature work, every enabled backpressure condition has a per-feature subagent attestation and evidence artifact.
- DRY is an enabled backpressure condition with a dedicated subagent check for duplicated logic and unjustified repetition.
- Passing commits include a staged tracked attestation artifact under `backpressure/attestations/`.
- Reviewers can detect missing attestations, missing subagent confirmations, and unjustified not-applicable verdicts from the committed artifact.
- CI can verify committed attestation artifacts without interactive prompts or subagent spawning.
- Lint rules are seeded as policy candidates, and only exact declared commands/analyzers are enforced.
- Unmet backpressure conditions include an attached message with blocker and next action.
- The setup is idempotent and safe against overwriting existing project knowledge.
- The user can see exactly what was created and what still needs filling.

## Implementation Backlog

### PRS-00 - Create Implementation Source Repo

Create `<workspace>/burpvalve` as the durable implementation repo.

Responsibilities:

- initialize a normal Git/Go project;
- define `cmd/setup-command-project` and `cmd/burpvalve`;
- create `internal/scaffold`, `internal/attestations`, `internal/gitindex`, and `internal/lintconfig`;
- create `templates/`, `fixtures/`, `docs/`, and `plans/`;
- add build targets that use `-trimpath -ldflags "-s -w"` and enforce the 8 MiB warning / 12 MiB failure binary size budget;
- include a local `skill/` folder only as the source for the skill wrapper, not as the only source of truth for Go code.

### PRS-00A - Create Skill Package Wrapper

Create the canonical JSM skill source at `<workspace>/jsm-skills/skills/setup-command-project`.

Responsibilities:

- provide `SKILL.md` as the agent-facing entrypoint;
- include a packaged `scripts/bin/burpvalve` command that runs setup and commit-gate subcommands;
- document that the Go source lives in `<workspace>/burpvalve`;
- avoid binary-only distribution as the source of truth;
- validate with `cd <workspace>/jsm-skills && jsm validate skills/setup-command-project`.

### PRS-01 - Define Scaffold Templates

Create the template files for:

- `AGENTS.md`;
- `docs/README.md`;
- `plans/README.md`;
- `log/README.md`;
- all `backpressure/*.md` stubs;
- `backpressure/attestations/README.md`;
- `log/backpressure/failed/README.md`.

Templates should use placeholders sparingly and avoid fake specificity.

### PRS-02 - Implement Readiness Inspector

Build a check-only command/workflow that reports:

- existing scaffold files;
- missing scaffold files;
- symlink status;
- beads status;
- NTM availability;
- registry status;
- git dirty status.

### PRS-03 - Implement Safe Scaffolder

Create missing files/directories without overwriting existing content.

Must handle:

- symlink creation;
- regular-file `CLAUDE.md` conflict;
- existing `.beads/`;
- partial `backpressure/` directory;
- no git repo.

### PRS-04 - Implement Beads Initialization

Run `br init` only when needed, then verify with `br doctor`, `br config --list`, and `br dep cycles`.

Optionally seed initial beads after confirmation.

### PRS-05 - Implement NTM Registration/Verification

Use `ntm --robot-capabilities` before any NTM action.

Define exactly what "registered" means:

- project basename is valid;
- `ntm quick <basename>` has been run or existing session/project is visible;
- `ntm --robot-snapshot` can verify the session/project when active.

### PRS-07 - Add Drift Repair Mode

Detect and repair common drift:

- missing `CLAUDE.md` symlink;
- added `CLAUDE.md` regular file;
- missing `backpressure/README.md`;
- missing standard backpressure category file;
- beads present but unsynced;
- `AGENTS.md` missing beads or NTM sections.

### PRS-08 - Self-Test The Setup Skill

Create fixture repos:

- empty directory;
- git repo without scaffold;
- repo with existing `AGENTS.md`;
- repo with regular `CLAUDE.md`;
- repo with existing `.beads/`;
- repo with partial `backpressure/`.

Verify:

- check mode is non-mutating;
- init mode creates expected files;
- rerun is idempotent;
- no existing user content is overwritten;
- symlink behavior is correct.

### PRS-09 - Implement Pre-Commit Backpressure CLI

Build `tools/burpvalve` in Go.

Responsibilities:

- load `backpressure/manifest.yaml`;
- detect changed features;
- build the feature x condition matrix;
- enforce one atomic feature or bug fix per normal commit;
- prompt the committing agent for direct subagent attestations;
- require a verdict for every feature/condition cell;
- require explanatory messages for `not_applicable`, `fail`, and `unknown`;
- write `backpressure/attestations/<staged-payload-hash>.json` for passing matrices;
- write `log/backpressure/failed/<timestamp>-blocked.json` for blocked attempts;
- require the tracked attestation artifact to be staged;
- print the entry banner before asking any questions;
- use `/dev/tty` for interactive prompting when available;
- fail closed with manual instructions when `/dev/tty` is unavailable;
- exit non-zero unless all cells are `pass` or `not_applicable` and the attestation artifact is staged.

### PRS-10 - Install Git Hook And CI Gate

Create `.githooks/pre-commit` as a thin wrapper around `burpvalve`.

Also add a CI command that runs the same verifier in non-interactive artifact-verification mode, so `git commit --no-verify` cannot permanently bypass the gate before merge.

Requirements:

- pre-commit mode may prompt the committing agent;
- pre-commit mode must check for `/dev/tty` before prompting;
- CI mode must not prompt or spawn subagents;
- CI mode must verify the committed attestation artifact, staged/committed payload hash, condition file hashes, and accepted verdicts;
- hook mode should fail once after generating a new attestation artifact, instructing the agent to stage it and retry.

### PRS-11 - Implement Attestation Prompt Flow

Teach `burpvalve` to ask the committing agent the right blocking questions.

Requirements:

- print one prompt per feature/condition cell;
- follow the fixed sequence: entry banner, atomicity, per-condition subagent confirmation, verdict, message/evidence, summary, artifact write, staging check;
- ask whether a dedicated subagent checked the cell;
- refuse a `pass` if the agent did not confirm a subagent checked it;
- collect `pass`, `not_applicable`, `fail`, or `unknown`;
- require a message for every non-pass verdict;
- require a message for `subagent_confirmed: false`;
- distinguish `subagent_confirmed: false` from `not_applicable`;
- fail closed on missing, malformed, contradictory, or stale attestations.

### PRS-12 - Define Optional NTM Top-Level Bridge

Keep NTM support as a top-level orchestration bridge, not the primary per-cell fanout path.

Requirements:

- use NTM to register/resolve the project session;
- optionally run a small reviewer/coordinator swarm of 2-10 agents;
- allow NTM reviewers to batch multiple backpressure cells when that is more appropriate than one pane per cell;
- verify any NTM state-changing action with `ntm --robot-snapshot` and attention/tail evidence;
- document that native agent subagents remain the preferred route for high-cardinality verifier cells.

### PRS-13 - Define Attestation Artifact Schema

Define and test the JSON schema for matrix artifacts and individual attestation records.

The schema must preserve:

- tool name and version;
- manifest hash;
- condition order;
- generator identity;
- Git HEAD before commit;
- feature identity;
- condition identity;
- source bead or diff cluster;
- staged payload hash;
- condition file hash;
- subagent model alias, if known;
- whether a dedicated subagent was confirmed;
- status;
- evidence;
- failure message;
- next action;
- timestamp.

This schema is not another backpressure condition. It exists so the CLI can tell whether the agent's attestation matches the current staged diff and current condition text.

### PRS-14 - Implement Lint Command Wiring

Teach setup and `burpvalve lint` to run project-declared lint/format/static-analysis checks.

Requirements:

- setup seeds lint policy candidates in `backpressure/lint-rules.md`;
- setup does not enforce lint wishlist items without commands/analyzers;
- projects may declare exact lint commands in `backpressure/lint-rules.md` or `backpressure/manifest.yaml`;
- declared lint commands should include `id`, `command`, `required`, `paths`, and `timeout_seconds`;
- `.githooks/pre-commit` runs lint mode after the backpressure attestation gate;
- lint mode prints skipped wishlist rules separately from enforced command failures;
- lint mode fails closed only for declared commands/analyzers that actually fail.

## Backlog Dependencies

The implementation backlog should convert into beads with these dependency edges:

| Bead | Depends on | Unblocks | Success criteria |
|---|---|---|---|
| PRS-00 | None | PRS-00A, PRS-01, PRS-02, PRS-09 | `<workspace>/burpvalve` exists with Go module, command skeletons, internal packages, templates, fixtures, docs, plans, and build targets that enforce the documented binary size budget. |
| PRS-00A | PRS-00 | PRS-03, PRS-10 | JSM skill wrapper exists under `<workspace>/jsm-skills/skills/setup-command-project` and validates locally. |
| PRS-01 | PRS-00 | PRS-03, PRS-07 | Template files exist and contain no fake project-specific rules. |
| PRS-02 | PRS-00 | PRS-03, PRS-04, PRS-05, PRS-10 | Inspector reports scaffold, git, beads, NTM, hook, and verifier status without mutation. |
| PRS-03 | PRS-01, PRS-02 | PRS-04, PRS-10 | Missing scaffold files are created without overwriting user content. |
| PRS-04 | PRS-02, PRS-03 | PRS-08 | Beads initializes or verifies idempotently; `br doctor` and `br dep cycles` are handled. |
| PRS-05 | PRS-02 | PRS-08, PRS-12 | NTM project/session identity is verified or a clear blocker is reported. |
| PRS-07 | PRS-01, PRS-02, PRS-03 | PRS-08 | Repair mode fixes missing standard scaffold pieces without clobbering user content. |
| PRS-08 | PRS-03, PRS-04, PRS-05, PRS-07 | PRS-09 | Fixture repos prove init/check/repair are idempotent. |
| PRS-09 | PRS-08, PRS-13 | PRS-10, PRS-11 | CLI loads manifest, detects atomic feature, builds matrix, writes tracked passing artifacts, writes local blocked reports, and exits correctly. |
| PRS-10 | PRS-00A, PRS-09 | PRS-11, PRS-14 | Git hook calls the CLI, handles TTY/non-TTY contexts correctly, blocks incomplete attestations, and requires the tracked attestation artifact to be staged. |
| PRS-11 | PRS-09, PRS-10, PRS-13 | PRS-12 | Prompt flow shows the entry banner, follows the fixed question sequence, records subagent confirmations, separates missing subagent checks from not-applicable verdicts, and refuses missing/non-pass cells. |
| PRS-12 | PRS-05, PRS-11 | None | Optional NTM bridge is documented as top-level coordination only. |
| PRS-13 | PRS-09 | PRS-10, PRS-11 | Attestation artifacts bind feature, condition, staged diff, condition file, subagent confirmation, verdict, message, and timestamp. |
| PRS-14 | PRS-01, PRS-10 | None | Lint mode runs only declared commands/analyzers, reports skipped wishlist rules, and fails only on real command failures. |

## Relationship To Existing Plans

This plan supports:

- `docs/07-agent-backpressure.md` by making repo-local backpressure a default scaffold.
- `docs/08-firstmate-repo-research.md` by adopting the idea that `AGENTS.md` is the project operating contract.
- `docs/09-matt-pocock-skills-review.md` by copying the setup-skill pattern.
- `plans/03-hermes-agent-platform.md` by giving Hermes-ready repos a standard substrate.
- `plans/11-agentic-work-backpressure-protocol.md` by creating places where artifact gates and evidence rules can live.

## Open Questions

1. Should this become a JSM skill, a Hermes built-in command, an NTM workflow, or all three?
2. Should the default backpressure files be this broad, or should UI/backend/data/trading projects get different profiles?
3. Should setup create initial beads by default, or only after explicit confirmation?
4. Should `AGENTS.md` include the full generated content, or should it link to smaller files under `/docs/` and `/backpressure/`?
5. Should project registry integration be required for every repo under the local projects workspace, or optional until the registry stabilizes?
6. Should NTM "registration" create a session immediately, or only verify that the project name/path is compatible with NTM?
