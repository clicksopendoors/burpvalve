# Issue 17: Guided Lint Setup With Scoped Enforcement

## Status

Planning, revision 2. Design questions resolved 2026-07-02 (see
`docs/decisions-2026-07-02-review-round.md`); revised again the same day after
two independent Codex reviews (Agent Mail messages 2268/2269, both "needs
revision": the original text depended on manifest/config fields that do not
exist and would be silently ignored by the current loader). The Implementation
Contract section at the end is binding. Ready for bead conversion after
`plans/land-working-set.md` completes and this revision is accepted. Do not
implement before the plan is converted into beads.

## Source

GitHub issue #17: "Add guided lint setup with language presets and real
enforcement."

This plan expands issue #17 with one additional requirement: scoped
subdirectory lint commands must be a first-class part of the guided setup flow.
Polyglot repos, monorepos, and mixed-tooling repos need lint commands that apply
to the right directory or package root. Single-language repos should not be
forced through unnecessary subdirectory questions.

## Problem

Burpvalve can already run explicit `lint_commands` from
`backpressure/manifest.yaml`. That is honest, but it leaves a weak default state:
many repos start with `lint_commands: []`, so `burpvalve lint` can only report
that no deterministic lint evidence exists.

The next product step is not to invent fake lint proof. It is to help the user
or agent discover real repo-native lint, format, type-check, test, and static
analysis commands and write the selected commands into the manifest.

The hard part is scoping. A TypeScript command should not accidentally cover Go
services. A Go command should not run from the frontend directory. A Python tool
should not scan generated frontend assets. Burpvalve needs to make scope obvious
when scope matters, and stay quiet when it does not.

## Goals

- Add a guided lint setup flow that proposes executable lint/static-analysis
  commands based on detected repo tooling.
- Make scoped subdirectory command setup first-class for polyglot and monorepo
  projects.
- Ask about polyglot or multi-root setup when detection finds multiple language
  roots, multiple package managers, or separate app/service directories.
- If the user declines polyglot/multi-root setup and the repo is treated as a
  single non-polyglot project, skip subdirectory lint questions and configure
  repo-root commands only.
- Prefer existing project commands over new dependencies or Burpvalve-owned
  analyzers.
- Show the exact manifest diff before writing, with confirmation defaulting to
  No.
- Preserve robot-safe JSON flows for agents and CI.

## Non-Goals

- Do not install new developer dependencies without explicit user confirmation.
- Do not silently edit `package.json`, formatter configs, CI files, hooks, or
  project scripts.
- Do not claim a lint rule is enforced unless a command or analyzer actually
  runs.
- Do not implement every language preset in the first increment.
- Do not make all structural rules mandatory defaults. Structural rules such as
  max function size or max nesting should be opt-in.

## Proposed Command Shape

Primary user-facing command:

```bash
burpvalve lint init
```

Decision: `burpvalve lint init` is the primary standalone command. A later
`burpvalve init` flow may ask whether to configure deterministic lint commands
now, but it should call into the same lint setup model rather than creating a
separate flow.

Avoided alternatives:

- `burpvalve lint configure`
- `burpvalve config init` with a lint setup section
- making lint configuration a required part of `burpvalve init`

## Wizard Flow

### 1. Detection

Inspect repo files and produce a non-mutating detection report:

- language roots: `go.mod`, `Cargo.toml`, `pyproject.toml`,
  `requirements.txt`, `uv.lock`, `package.json`, lockfiles, framework configs;
- package manager roots: npm, pnpm, yarn, bun, cargo, go, uv, poetry, etc.;
- existing scripts: `lint`, `test`, `typecheck`, `check`, `build`, `format`;
- known config files: ESLint, Prettier, TypeScript, Astro, Ruff, Mypy, Pyright,
  Go tooling, Rustfmt, Clippy;
- existing `backpressure/manifest.yaml` `lint_commands`;
- existing CI workflow commands, when discoverable without expensive parsing.

Detection should produce facts, not mutate files.

### 2. Polyglot Or Single-Root Decision

If detection sees multiple language roots, package roots, or likely app/service
directories, the wizard asks a polyglot question:

```text
This repo appears to have multiple tool roots.
Configure scoped lint commands per subdirectory?
```

Recommended default: Yes when multiple roots are detected.

If the user declines and chooses a single-root setup, the wizard should:

- stop asking subdirectory-specific lint questions;
- propose only repo-root commands;
- warn when ignoring detected subdirectories could leave parts of the repo
  unchecked;
- record no scoped command groups unless the user explicitly re-enables them.

If detection sees a single language/tool root, the wizard should not ask about
subdirectory scopes by default.

### 3. Command Selection

For each selected root or scope, show candidate commands:

- Go: `go test ./...`, `go vet ./...`, `gofmt` check.
- Node/TypeScript/React/Astro: existing package scripts such as `lint`,
  `test`, `typecheck`, `check`, `build`; formatter checks only when configured.
- Rust: `cargo fmt --check`, `cargo test`, `cargo clippy`.
- Python: `ruff check`, `pytest`, `mypy`, `pyright` only when dependencies or
  config indicate they exist.
- Generic: `git diff --check`, generated-artifact checks, file-size checks.

Each command asks:

- required or advisory;
- timeout;
- scope paths;
- whether it runs from repo root or a subdirectory;
- whether it should run locally, in CI, or both.

### 4. Structural Rule Selection

Structural rules are opt-in. Examples:

- maximum function size;
- maximum file size;
- maximum nesting depth;
- suspicious TODO/placeholder markers;
- skipped tests or weakened assertions;
- test adjacency expectations;
- language/framework-specific checks.

First increment should treat structural rules as recommendations unless a real
command or built-in analyzer exists and the user explicitly enables it.

### 5. Preview And Confirmation

Before writing, show:

- detected roots;
- selected commands;
- scoped directories;
- required/advisory status;
- timeout values;
- exact manifest diff;
- any recommendations left as prose in `backpressure/lint-rules.md`.

Confirmation defaults to No.

## Manifest Shape

Current manifest entries look like:

```yaml
lint_commands:
  - id: frontend-lint
    command: "cd apps/web && npm run lint"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 120
```

This works, but it hides the execution directory inside the shell command. For
first-class scoped commands, add an optional `run_directory` field. The name is
intentionally command-specific: `target_root` is the repo Burpvalve operates on,
while `run_directory` is the subdirectory where one lint command runs.

```yaml
lint_commands:
  - id: frontend-lint
    run_directory: "apps/web"
    command: "npm run lint"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 120
```

Decision: issue #17 should introduce `run_directory` in the first increment,
while continuing to support existing commands that encode directory changes with
`cd ... &&`.

`run_directory` must be repo-relative and contained inside `target_root`.
Absolute paths and parent traversal are invalid:

```yaml
run_directory: "."          # valid
run_directory: "apps/web"   # valid
run_directory: "/example/web"   # invalid
run_directory: "../web"     # invalid
```

## Robot JSON Flow

Agents need deterministic non-interactive paths:

```bash
burpvalve lint init --json --detect
burpvalve lint init --robots < input.json
burpvalve lint init --force --preset go --write
```

Detection output should include:

- detected languages;
- detected roots;
- detected commands/scripts;
- proposed commands;
- command scope;
- whether each command already exists;
- required/advisory recommendation;
- manifest diff;
- skipped recommendations with reasons.

Robot input should allow:

- selecting root scopes;
- accepting specific commands;
- setting required/advisory;
- setting timeout;
- confirming writes.

No robot path should mutate unless confirmation is explicit.

## First Increment

1. Add read-only lint setup detection.
2. Add `burpvalve lint init --json --detect`.
3. Detect Go and Node/Astro roots first.
4. Ask the polyglot/scoped setup question when multiple roots are detected.
5. Present candidate commands from existing project scripts/config only.
6. Preview manifest changes and require confirmation before writing.
7. Write selected commands to `backpressure/manifest.yaml`.
8. Leave unconfirmed or unavailable recommendations in `backpressure/lint-rules.md`
   as policy candidates, not enforcement.
9. Add tests proving no command is reported as enforced until it is written and
   executable.

## Later Increments

- Add Python and Rust presets.
- Add richer validation and migration guidance for existing `cd ... &&`
  commands after `run_directory` exists.
- Add built-in staged-file analyzers for simple structural rules.
- Add CI integration recommendations.
- Add global preset storage and per-project overrides.
- Add import/export for user-defined lint presets.

## Acceptance Criteria

- The wizard asks about scoped subdirectory commands when a repo appears
  polyglot or multi-root.
- The wizard skips subdirectory lint questions when the user chooses a
  single-root, non-polyglot setup.
- Generated manifest entries are explicit, readable, and easy to remove.
- `run_directory` is validated as a relative path inside the target repo.
- Burpvalve never installs dependencies or edits project scripts/configs without
  explicit confirmation.
- Burpvalve never claims enforcement for wishlist prose.
- `burpvalve lint` reports no-op, advisory, and enforced states distinctly.
- Robot output exposes detection facts and proposed manifest diffs.
- Tests cover single-language repo, polyglot repo, declined scoped setup, and
  accepted scoped setup.

## Concurrency (Goroutine Work Scoped To This Plan)

Decision D9 in `docs/decisions-2026-07-02-review-round.md` approved three
concurrency items and scoped them here, because scoped per-directory lint
commands are the workload that benefits most. Production code currently
contains zero goroutines; `internal/backpressure/lint.go` runs commands
sequentially, and `internal/scaffold/inspect.go` probes external tools
sequentially.

1. **Parallel `lint_commands` execution (primary, ships with the wizard).**
   Run declared commands concurrently with bounded parallelism (errgroup or
   equivalent). Default bound: `min(len(commands), 4)` — deliberately lower
   than `runtime.NumCPU()` because package-manager scripts and monorepo tools
   are memory- and cache-heavy; overridable with `--jobs N`, where `--jobs 1`
   restores exactly today's serial behavior (this equivalence is a required
   test). Invalid `--jobs` (0, negative, non-integer) is rejected with a
   usage error. Requirements:
   - Output and JSON result order remain deterministic: collect results, emit
     in manifest order regardless of completion order.
   - Per-command timeouts keep working exactly as today (each command already
     runs under its own `context.WithTimeout`); a timeout in one parallel
     command must not cancel the others.
   - **`serial: true` scheduling semantics (explicit):** execution happens in
     two phases. Phase 1 runs all non-serial commands with bounded
     parallelism. Phase 2 runs all `serial: true` commands one at a time, in
     manifest order relative to each other. A serial command listed first in
     the manifest therefore runs *after* the parallel batch — this is
     documented behavior, and the wizard says so when it sets `serial: true`.
     Output/JSON ordering is still manifest order regardless of phase.
   - Interleaved stdout/stderr is never shown live; each command's output is
     buffered and printed as a block, in manifest order. Current buffering
     limits apply per command; note in docs that N parallel commands buffer N
     outputs concurrently.
   - Required acceptance tests: `--jobs 1` byte-identical to pre-change
     serial output; deterministic JSON order under randomized completion;
     multiple simultaneous failures all reported; required-vs-optional
     failure handling unchanged; per-command timeout under parallelism;
     `serial: true` two-phase scheduling.
2. **Parallel setup/inspect probes (secondary — separate bead, NOT part of
   the wizard bead set).** The independent external probes in
   `internal/scaffold/inspect.go` (git checks, `br doctor`,
   `ntm --robot-capabilities`) run concurrently. Report structure and check
   ordering in JSON output are unchanged. This lands as its own bead so it
   cannot widen the wizard implementation.
3. **Concurrent attestation scan (tertiary, deferred until needed).**
   `attestations list/latest` may scan evidence directories concurrently once
   real repos accumulate hundreds of records. Do not build this until a repo
   demonstrates the need; file a bead and defer.

Everywhere else the CLI is a sequential prompt/state machine; do not add
goroutines outside these three items.

## Resolved Design Questions

1. Resolved: first-class scoped commands should add `run_directory`, not
   `working_directory`, and not rely only on `cd <dir> && <command>`.
2. Resolved: `run_directory` must not point outside the target repo root.
3. Resolved: `burpvalve lint init` is the primary command. `burpvalve init` may
   optionally delegate to it later.
4. Resolved (2026-07-02): **structural rules are generated recommendations
   only in the first increment.** They live as prose in
   `backpressure/lint-rules.md` until backed by a real external command or
   analyzer the user explicitly enables. Built-in staged-file analyzers stay
   in Later Increments and, when built, must be individually opt-in — never
   defaults. Rationale: Burpvalve's credibility rests on never claiming
   enforcement that does not execute; shipping half-strength built-in
   analyzers early would blur exactly the line issue #8 fixed.
5. Resolved (2026-07-02, sharpened in revision 2): **presets store full
   command definitions as policy templates that always require per-project
   confirmation.** A preset (e.g. "Go default": `go test ./...`,
   `go vet ./...`, gofmt check) carries complete `lint_commands` entries —
   but applying a preset always walks the same preview-diff-and-confirm flow;
   presets are never auto-applied at init. Command-ID-only storage was
   rejected (not portable); unconfirmed templates were rejected (violates
   confirmation-defaults-No).
   **Increment split (resolves the review-found contradiction):** the first
   increment ships **built-in presets only** (compiled into the binary for Go
   and Node/Astro; `--preset go` selects a built-in). *User-defined* preset
   storage in global config with project overrides stays in Later Increments,
   and when built requires real schema work: `internal/config` uses
   `DisallowUnknownFields`, so a `defaults.lint_presets` field must be added
   to the schema with Merge/Normalize/Validate/source-note support — that is
   its own bead, not a side effect.
6. Resolved (2026-07-02, made concrete in revision 2): **declined scoped
   setup warns and records; it does not block.** When detection finds
   multiple roots and the user declines scoped setup, the wizard: (a) prints
   an explicit reduced-coverage warning naming the unchecked roots; (b)
   records the decision durably in the manifest; (c) lint output surfaces the
   reduced coverage so agents cannot over-read a green lint as full-repo
   evidence. Blocking was rejected: Burpvalve refuses unverified *claims*,
   not explicitly-accepted reduced scope.
   **Concrete shape (binding):** the manifest gains one new top-level block:

   ```yaml
   lint_coverage:
     declined_roots: ["apps/web", "services/py"]   # repo-relative, validated
     declined_at: "2026-07-02"                      # date of the human decision
   ```

   `LintResult` gains `coverage` (`"full"` when `declined_roots` is empty or
   absent, `"partial"` otherwise) and `unchecked_roots` (echo of
   `declined_roots`). Semantics: coverage does NOT change `status`,
   `enforced`, or `evidence_strength` — commands that ran still ran — but
   human output appends "coverage: partial — unchecked roots: ..." to the
   summary line, and agents must treat `coverage: partial` as reduced
   evidence. Manifest loading must *validate* this block (repo-relative
   paths, no traversal), which requires ending the current
   ignore-unknown-YAML behavior for known top-level sections (see
   Implementation Contract).

## Implementation Contract (Revision 2 — Binding)

Added after review round: the original text assumed manifest/runtime behavior
that does not exist. Current reality: `internal/lintconfig/lintconfig.go`
defines only `id`, `command`, `required`, `paths`, `timeout_seconds`;
`internal/backpressure/core.go` silently ignores unknown YAML keys; and
`internal/backpressure/lint.go` always runs commands with `cmd.Dir = root`.
Every schema promise in this plan therefore requires explicit units, in this
order.

### Unit A — Schema and runtime foundation (prerequisite for everything)

- Extend `lintconfig.Command` with `run_directory` (string, optional) and
  `serial` (bool, optional). Extend the manifest model with the
  `lint_coverage` block (Resolved Q6).
- `validateLintCommandNode` (and manifest validation generally) must
  **reject** unknown per-command keys and unknown known-section fields
  instead of silently ignoring them — a typo like `run_dir:` must be an
  error, or every field this plan adds is a silent no-op trap. Top-level
  unknown sections may warn rather than fail for forward compatibility;
  per-command unknown keys fail.
- `run_directory` validation: repo-relative, cleaned, no `..`, no absolute
  paths; containment is **lexical after cleaning** (symlink resolution is not
  attempted); the directory must exist at lint run time — a missing
  `run_directory` fails that command with a clear error (`status: failed`,
  not a crash), and manifest *write* time additionally warns if the
  directory does not exist.
- `runLintCommand` sets `cmd.Dir` to the validated `run_directory` (default
  `.` = repo root). **`paths` and `BACKPRESSURE_LINT_PATHS` remain
  repo-root-relative in all cases**, including when `run_directory` is set.
  This is the single rule that prevents the `apps/web/apps/web` ambiguity;
  document it in the schema docs and test it.
- Legacy `cd <dir> && <cmd>` commands remain fully supported, forever.
  Generated commands prefer `run_directory`. Migration of existing `cd`
  commands is a separate, confirmed, later-increment flow — never automatic.
- `LintResult` gains `coverage` and `unchecked_roots` (Resolved Q6) and the
  JSON/text vocabulary below.

### Unit B — Output vocabulary (extends the #8 model, does not replace it)

- Per-command statuses stay (`passed`, `failed`, `timeout`, ...). An optional
  (`required: false`) command that fails keeps overall `status: passed` but
  the result gains `advisory_failures: [<ids>]`, and human output prints
  "advisory: <id> failed (does not block)". "Advisory" is a reporting label
  for optional-command failures, not a new status enum value.
- Coverage fields per Resolved Q6. The three states demanded by the
  acceptance criteria map as: no-op = `not_enforced`/`enforcement_level:
  scaffold-only` (existing #8 fields); advisory = `advisory_failures`
  non-empty; enforced = `enforced: true` with `evidence_strength:
  command-output`.

### Unit C — Manifest writer

No safe YAML patch helper exists today; the wizard needs one:

- Read-modify-write that preserves existing conditions, existing
  `lint_commands`, key order, and comments where the YAML library allows;
  writes are atomic (temp file + rename).
- Duplicate command IDs: `executableLintCommands` currently fails on
  duplicates, so the wizard must detect collisions at proposal time and ask
  update / skip / rename (robot flow: explicit per-command `on_conflict`
  field; default skip).
- Idempotency: re-running the wizard with the same selections produces zero
  diff; this is a required test.
- `backpressure/lint-rules.md` recommendation writes get the same treatment
  as manifest writes: shown as an exact diff, confirmed (default No), and
  appended under a marked section (`<!-- burpvalve:lint-init recommendations
  -->`) so reruns replace that section instead of duplicating it.

### Unit D — CLI and robot wiring

- `lint` grows the `init` subcommand without changing `burpvalve lint
  --json` behavior (compat test required). New flags: `--detect`, `--write`,
  `--preset <name>`, `--jobs N` (on `lint` itself), `--force`.
- Flag matrix (binding): `--detect` is always read-only regardless of other
  flags; `--write` performs the write only after confirmation; `--force`
  skips the interactive confirmation **only when `--write` is also present**
  (i.e. `--force` without `--write` still mutates nothing); `--robots`
  accepts the structured input including an explicit `"confirm": true` field
  — no robot path mutates without it (consistent with `config init
  --robots`).
- Robot/help surfaces updated: `robotInputDoc`, `--robots` examples,
  `lint init` help describing detection facts, proposed diff, confirmation,
  and coverage fields.

### Unit E — Detection bounds (first increment, exact)

- Go root: a directory containing `go.mod`. Candidate commands `go test
  ./...`, `go vet ./...`, `gofmt -l .` are proposed when the root exists and
  `go` is on PATH — "existing project script" evidence is not required for
  the Go toolchain trio, but each is clearly labeled as a toolchain-standard
  proposal, not a detected script.
- Node/Astro root: a directory containing `package.json`. Only *existing*
  `scripts` entries named `lint`, `test`, `typecheck`, `check`, `build`,
  `format:check` (exact-name match) are proposed, invoked via the detected
  package manager (lockfile detection: `package-lock.json`/`pnpm-lock.yaml`/
  `yarn.lock`/`bun.lockb`).
- Explicitly OUT of the first increment: Python, Rust, CI-workflow parsing,
  generic file-size/artifact checks, and any directory classified as
  vendored/generated (`node_modules`, `vendor`, `dist`, `docs/demos/generated`).
- "Multiple roots" heuristic (binding): more than one distinct directory
  (after exclusions) containing `go.mod` or `package.json`, or one of each.
  Nothing else triggers the polyglot question in the first increment.

### Acceptance additions (beyond the existing list)

- Unknown per-command manifest keys are rejected with a named error.
- `run_directory` positive/negative cases: valid subdir, `.`, absolute,
  `..`, nonexistent-at-run-time.
- Legacy `cd`-style command continues to work unchanged alongside
  `run_directory` commands in one manifest.
- `--detect` never mutates (test with `--force --write --detect`).
- Wizard rerun idempotency (zero diff).
- Concurrency tests from the Concurrency section, including `--jobs 1`
  equivalence.
- `coverage: partial` appears in JSON and human output when
  `declined_roots` is non-empty, while `status`/`enforced` are unaffected.
