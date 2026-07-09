# Burpvalve Architecture

Burpvalve is a Go CLI and packaged Codex skill for adding repo-local
backpressure to agentic development. It installs operating contracts,
scaffold directories, optional coordination hooks, a git pre-commit valve,
verifier evidence prompts, lint execution, and release packaging.

The important product boundary is that Burpvalve does not make an agent's
claim true. It makes the claim checkable by binding each commit gate to the
staged payload, the enabled condition files, and recorded verifier evidence.

## Build And Package Shape

- Module: `burpvalve`, Go 1.25 (`go.mod:1`, `go.mod:3`).
- CLI entry point: `cmd/burpvalve/main.go` (`newRootCommand` starts at
  `cmd/burpvalve/main.go:112`).
- Main dependencies: Cobra for CLI structure, Bubble Tea/Bubbles/Lip Gloss for
  guided terminal flows, and `gopkg.in/yaml.v3` for manifest parsing.
- `make build` writes `bin/burpvalve`, injects the build version, and checks
  the binary against warning and hard size budgets (`Makefile:1`-`Makefile:24`).
- `scripts/package-skill.sh` builds a platform binary, copies
  `skill/burpvalve`, writes `VERSION`, includes `LICENSE` and
  `THIRD_PARTY_NOTICES.md`, and emits a tarball plus `.sha256` file
  (`scripts/package-skill.sh:5`-`scripts/package-skill.sh:41`).
- Release automation builds Linux and macOS assets for amd64 and arm64, then
  publishes the archives plus aggregate `checksums.txt`
  (`.github/workflows/release.yml:24`-`.github/workflows/release.yml:67`).

## Public Surfaces

The root command describes the tool as repo backpressure for atomic work units,
supports JSON and robot modes, and registers the command family at startup
(`cmd/burpvalve/main.go:120`-`cmd/burpvalve/main.go:171`).

| Surface | Purpose | Code anchor |
| --- | --- | --- |
| `setup` | Read-only repo inspection and recovery facts. | `cmd/burpvalve/main.go:2464` |
| `init` | Mutating scaffold installation. | `cmd/burpvalve/main.go:3039` |
| `repair` | Restore missing generated pieces without overwriting project knowledge. | `cmd/burpvalve/main.go:3107` |
| `commit` | Fail-closed staged-payload valve that writes passing attestations or blocked reports. | `cmd/burpvalve/main.go:3171` |
| `lint` / `lint init` | Run manifest-declared commands and write preset command manifests. | `cmd/burpvalve/main.go:3215` |
| `ci` | Validate staged or committed attestation evidence. | `cmd/burpvalve/main.go:3272` |
| `hash --staged` | Print the staged payload hash and hash-included path set. | `cmd/burpvalve/main.go:3318` |
| `account payload` | Read-only staged-path ownership accounting. | `cmd/burpvalve/main.go:3448` |
| `prompts` | List, render, and export canonical prompt-bank entries. | `cmd/burpvalve/main.go:3622` |
| `verifier begin/submit/doctor/prompts` | Prepare and ingest verifier packets, inspect verifier config, and generate handoff text. | `cmd/burpvalve/main.go:3792` |
| `attestations` | Browse, list, show, and query passing or blocked evidence. | `cmd/burpvalve/main.go:4175` |
| `beads preflight/drift/close` | Plan and execute Beads delivery closure through the valve. | `cmd/burpvalve/main.go:4538` |
| `config` | Show and write global/project defaults. | `cmd/burpvalve/main.go:5795` |
| `completion` | Install and verify shell completion plus command PATH wiring. | `cmd/burpvalve/main.go:1147` |

## Data Flow

```text
human or agent command
  |
  v
cmd/burpvalve Cobra command tree
  |
  +-- setup/config/completion -----> read-only probes and config sources
  |
  +-- init/repair -----------------> internal/scaffold
  |                                    |
  |                                    +-- embedded templates
  |                                    +-- optional Beads and NTM setup
  |                                    +-- hook dispatcher and hooksPath
  |                                    +-- Claude route selection
  |
  +-- commit/hash/verifier/ci ------> internal/backpressure
                                       |
                                       +-- manifest and condition hashes
                                       +-- staged or committed payload hash
                                       +-- feature/work-unit binding
                                       +-- verifier response collection
                                       +-- attestation validation
                                       +-- blocked-report recovery facts
```

## Scaffold Model

`internal/scaffold` owns setup inspection, init, and repair.

The embedded scaffold includes `AGENTS.md`, `ORCHESTRATOR.md`, docs, plans,
log directories, backpressure condition files, the attestation directory, the
pre-commit hook, and `tools/burpvalve` notes (`internal/scaffold/apply.go:56`-
`internal/scaffold/apply.go:82`). `ApplyOptions` controls target selection,
skip flags, verifier defaults, Claude route, dogfood mode, optional Beads, NTM,
and repo-local binary behavior (`internal/scaffold/apply.go:88`-
`internal/scaffold/apply.go:111`).

`init` copies generated files only when missing, initializes Beads and NTM when
enabled, installs the pre-commit hook, and can opt into a repo-local
`bin/burpvalve` fallback (`internal/scaffold/apply.go:113`-
`internal/scaffold/apply.go:220`). The default Claude route is the agent
symlink, but Burpvalve also supports an orchestrator-skill route and an explicit
none route (`internal/scaffold/apply.go:574`-`internal/scaffold/apply.go:608`).

`setup` is non-mutating. It reports checks, planned changes, readiness
severity, repo-local binary facts, Claude route facts, config sources, and
recovery steps (`internal/scaffold/inspect.go:220`-
`internal/scaffold/inspect.go:348`). `repair` reuses the scaffold machinery but
is designed to restore generated pieces while preserving project knowledge
(`internal/scaffold/repair.go:40`-`internal/scaffold/repair.go:120`).

## Backpressure And Evidence Model

`internal/backpressure` owns the valve.

The manifest contains condition specs, verifier policy, lint commands, and lint
coverage (`internal/backpressure/core.go:32`-`internal/backpressure/core.go:43`).
Plans bind together the mode, manifest hash, condition file hashes, staged
payload hash, staged paths, detected features, matrix cells, and lint commands
(`internal/backpressure/core.go:66`-`internal/backpressure/core.go:79`).

The staged reader uses `git diff --cached --name-status -M -z` and reads staged
content through `git show :<path>` (`internal/backpressure/core.go:104`-
`internal/backpressure/core.go:128`). Commit-scoped CI uses a committed reader
that diffs the target commit against its first parent, with root-commit fallback
to the empty tree (`internal/backpressure/core.go:130`-
`internal/backpressure/core.go:155`).

`RunPreCommit` builds the plan, collects or reads verifier responses, validates
the attestation binding, writes a passing artifact under
`backpressure/attestations/`, or writes a blocked report under
`log/backpressure/failed/` (`internal/backpressure/artifacts.go:116`).

Verifier evidence is policy-aware. New response files should use the structured
`verifier` object; legacy `subagent_confirmed` remains accepted only through
the compatibility path documented in the result contract. Supplemental
verifiers and adjudication are preserved in attestation artifacts, but they do
not turn a primary `fail` or `unknown` into a passing record.

## Lint And CI

`burpvalve lint` is deliberately explicit: it runs only commands declared in
`backpressure/manifest.yaml`. If no executable commands are declared, the result
is `not_enforced`, with `evidence_strength: "none"` and
`enforcement_level: "scaffold-only"` (`internal/backpressure/lint.go:79`-
`internal/backpressure/lint.go:127`).

Declared commands support required versus advisory behavior, scoped paths,
timeouts, `run_directory`, serial execution, and bounded parallelism
(`internal/backpressure/lint.go:22`-`internal/backpressure/lint.go:59`,
`internal/backpressure/lint.go:235`-`internal/backpressure/lint.go:280`).
Built-in Go and Node/Astro presets live in
`internal/backpressure/lint_presets.go`.

`burpvalve ci` validates attestation evidence against either the staged payload
or a requested commit. With `--commit`, `--feature` is an assertion against the
committed attestation feature rather than a way to reshape the historical diff
(`internal/backpressure/ci.go:53`-`internal/backpressure/ci.go:135`).

## Prompt Bank And Verifier Flow

`burpvalve prompts` exposes canonical orchestrator prompts from the binary.
The list/show/export UI prints names, versions, variables, content hashes, and
the reminder that exported prompt files are local copies rather than the
authority (`cmd/burpvalve/main.go:4093`-`cmd/burpvalve/main.go:4164`).

`burpvalve verifier begin` creates a responses file for the current staged
payload. `verifier prompts` packages condition-specific read-only verifier
handoffs. `verifier submit` merges verdicts into the response file while
preserving condition provenance and warnings (`cmd/burpvalve/main.go:4012`-
`cmd/burpvalve/main.go:4090`).

## Beads Delivery Helpers

`burpvalve beads preflight` is read-only and explains whether the bead closure
is admin-only or delivery-class. `beads drift` checks closed beads against
attestation and commit evidence. `beads close` is a journaled state machine that
closes Beads, syncs `.beads/issues.jsonl`, stages tracker state, runs the valve,
stages the named attestation bounce, revalidates, and then commits
(`cmd/burpvalve/main.go:4436`-`cmd/burpvalve/main.go:5575`).

The close helper is intentionally strict because `.beads/issues.jsonl` is part
of the committed delivery record. If the payload or attestation shifts, the
gate refuses stale evidence and reports a recovery command.

## Install And Release

The installer defaults to `clicksopendoors/burpvalve`, can pin a release tag,
can install from a local archive, asks for or reads the preferred skills
directory, and copies the command executable into `--bin-dir` unless
`--no-shims` is passed (`install.sh:4`-`install.sh:150`). It supports Linux and
macOS on amd64 and arm64, accepts agent JSON through `--robots`, prefers
`gh release download`, falls back to direct release URLs, persists selected
install locations in Burpvalve config, verifies the archive against
`checksums.txt`, and refuses packages missing `scripts/bin/burpvalve`.

Public install documentation should use a pinned release tag, prefer the
download-inspect-run installer path, show piped install only as a tradeoff, and
avoid Go's module installer because the module path is intentionally
`burpvalve`.

## Public Documentation Sources

- `README.md`: public landing page, command overview, install path, lexicon, and
  limitations.
- `docs/vocabulary.md`: canonical D-LEX vocabulary for backpressure, valve,
  burp, seal, attestation, work unit, feature, and bead.
- `docs/result-contract.md`: JSON result and recovery contract for setup, init,
  repair, commit, hash, verifier, lint, CI, account, and Beads helpers.
- `docs/release-install.md`: release package and install workflow.
- `SECURITY.md`: security reporting policy and pre-flip activation
  deferral.
- `THIRD_PARTY_NOTICES.md`: license and attribution bundle for the shipped
  binary dependency graph.
- `skill/burpvalve/SKILL.md`: installed agent-facing operating guide.

## Verification Commands

Use the smallest command set that matches the changed surface:

```bash
go test ./...
make build
VERSION=dev ./scripts/package-skill.sh
jsm validate skill/burpvalve
burpvalve setup --json
burpvalve lint --json
burpvalve ci --commit <commit>
```

`burpvalve lint` only proves executable lint evidence when the manifest declares
commands. Otherwise it reports scaffold-only policy text honestly.
