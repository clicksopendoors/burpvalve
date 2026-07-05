# Land The Working Set As Atomic Gated Commits

## Status

Planning, revision 2. Revised 2026-07-02 after two independent Codex reviews
(Agent Mail messages 2264 and 2265, agents ScarletMarsh and EmeraldBrook; both
verdicts "needs revision"). Convert to beads only after this revision is
accepted. This plan blocks every other plan in the current round.

## Source

Decision D1/D2/D3/D4 in `docs/decisions-2026-07-02-review-round.md`.
Decomposition of the ~6,900 tracked-diff lines plus ~7,300 untracked lines of
uncommitted work on top of commit `331a61e` (v0.1.1).

## Problem

HEAD is the v0.1.1 release. The working tree contains roughly sixteen logical
units of finished, tested work (build green, `go test ./...` green) addressing
10 of the 11 open GitHub issues — all uncommitted. The work must land as
atomic commits, each passing Burpvalve's own gate with real verifier evidence.

Review round 1 found the original sequencing unexecutable: the gate scaffold
was ordered last, the active hook would have gated commits with a stale
ignored binary, and the `main.go` split was under-specified. This revision
fixes ordering, defines the gate execution contract, and tightens the
file-to-unit map.

## Goals

- Every logical unit lands as one atomic commit with a passing, staged
  attestation from `burpvalve commit`, produced under tracked gate rules.
- Each staged payload builds and tests green **as staged** (not merely as part
  of the full working tree).
- The tree contains no runtime state, no tracked binary, no TBD stubs.
- Issues #6 and #16 get explicit verification, not assumed fixes.
- `git status` is clean at the end — every currently-untracked file is either
  landed by a named unit or ignored by a named rule. No file is unaccounted.

## Non-Goals

- No new features. This plan lands existing work plus mechanical cleanup and
  the #16 arrow sanitization (a defect fix mandated by D4).
- No refactoring during landing (e.g. splitting `main.go` into packages).
- Do not bypass the gate with `--no-verify` for any commit in this plan.

## Gate Execution Contract (resolves review blocker: which binary gates?)

For every commit in this plan:

- The active root `.githooks/pre-commit` is updated **in Phase 0** to invoke
  the gate as `go run ./cmd/burpvalve` explicitly, and the stale local
  `bin/burpvalve` executable is **deleted from the worktree** (not merely
  de-tracked) so no ignored binary can silently produce evidence. The global
  v0.1.1 binary must not be used to gate these commits (it lacks, among other
  things, the deleted-path handling the payloads need).
- Rationale, recorded: gating with `go run` over the working tree is a
  deliberate, documented compromise. Perfect self-hosting (each commit gated
  by exactly its own committed code) is impossible while the fixes being
  landed are themselves needed to run the gate. The evidence contract here is
  "the current tool verified the staged payload," not "the committed snapshot
  verified itself." This is the one-time bootstrap exception; after v0.1.2 is
  released and installed, the hook returns to preferring the global binary.
- Staged-slice verification: before each commit, run the build/test check
  against the **staged content only**, not the whole dirty tree:

  ```bash
  git stash push --keep-index --include-untracked -m "landing-verify"
  go build ./... && go test ./...
  git stash pop
  ```

  (Equivalently: `git worktree add` a temp dir, `git checkout-index` into it,
  and test there. Either is acceptable; the requirement is that the verified
  tree equals the staged payload plus HEAD, nothing else.)
- Attestation choreography per commit, in exact order: (1) stage the payload;
  (2) run verifier work and assemble responses; (3) run `burpvalve commit
  --responses ...` — it writes the attestation and exits nonzero; (4) `git
  add` the attestation file it names; (5) rerun the gate; (6) `git commit`.
  Do not stage `.beads/issues.jsonl` or other generated files between steps 3
  and 6, because that changes the payload hash and stales the attestation.

## Phase 0 — Gate Bootstrap (commits 1-4, land first)

Reviews established that the gate scaffold must be tracked **before** any
attested commit, or the evidence binds to untracked local files and is not
reproducible from any commit snapshot.

1. **Chore: ignore runtime state, delete stale binary.** Add `.ntm/` to
   `.gitignore` (`bin/` is already ignored — do not re-add). `git rm --cached
   bin/burpvalve` AND `rm bin/burpvalve` from the worktree. This commit is
   pre-gate (see bootstrap exception): the gate cannot honestly run before
   commit 3, so commits 1-3 carry the documented exception instead of
   attestations.
2. **Docs: fill and track the operating contract.** `AGENTS.md`: replace the
   TBD project purpose and TBD commands with real ones (`make build`,
   `go test ./...`, `make package`); **keep** the existing NTM, backpressure,
   atomic-work sections and **keep** the appended `bv-agent-instructions-v2`
   Beads block as-is (it is live operational instruction, not scaffold
   residue). Track `CLAUDE.md` (the symlink — currently untracked; verify
   `git ls-files` shows both afterward).
3. **Chore: track the live gate.** Root `.githooks/pre-commit` (updated per
   the Gate Execution Contract to use `go run ./cmd/burpvalve` and to forward
   `BURPVALVE_FEATURE/RESPONSES/AGENT/MODEL`), `backpressure/manifest.yaml`,
   all `backpressure/*.md` condition files, `backpressure/attestations/README.md`,
   `log/` READMEs, `tools/burpvalve/` README. From the **next** commit onward,
   every commit must carry a valid attestation produced under these tracked
   rules.
4. **Docs: land the planning round.** `docs/decisions-2026-07-02-review-round.md`,
   `plans/land-working-set.md`, `plans/release-v0.1.2.md`,
   `plans/issue-17-guided-lint-setup.md` (as revised),
   `plans/verifier-orchestration.md`, `plans/bead-closure-mode.md`,
   `plans/open-source-launch.md`, updated `plans/README.md` and
   `docs/README.md` index lines. This is admin/docs work (gated; conditions
   will mostly be `not_applicable` with reasons — that is the honest answer
   for planning documents).

## Phase 1 — Beads History Decision (commit 5)

Resolved (was an open question; reviews demanded a decision): the modified
`.beads/issues.jsonl` — which records already-closed historical beads, some
with legacy pre-Burpvalve IDs and an old `source_repo_path` — is
committed **as-is, as one admin commit**, explicitly labeled as historical
tracker state imported ahead of commit history. Rewriting or renaming bead
history to look cleaner would be falsifying records. Fresh beads are then
created for THIS landing sequence (one per remaining commit) so `--bead`
metadata on landing commits refers to real, current beads. No landing commit
may cite a historical bead as its delivery bead.

## Phase 2 — Feature Units (commits 6+)

Ordering unchanged in spirit (foundations first), with the corrected
file-to-unit map. **The file lists below are exhaustive for tracked-diff and
untracked files; a file appearing in two units means named-hunk/function
ownership applies (see Main.go Protocol).**

| # | Unit | Payload (exhaustive) | Issues |
|---|------|----------------------|--------|
| 6 | Staged deletes/renames in payload + feature detection | `internal/backpressure/core.go` (name-status parsing, `StagedPayloadFile`, entry sorting — NOT the `VerifierPolicy` field, which is unit 7's), `internal/backpressure/core_test.go`, the staged-deletion test functions in `cmd/burpvalve/commit_test.go` | #15 |
| 7 | Verifier provenance + policy model | `internal/attestations/attestations.go`, `internal/attestations/attestations_test.go`, `internal/backpressure/prompts.go`, `VerifierPolicy` field + plumbing in `core.go`, verifier-policy hunks of `internal/backpressure/artifacts.go`, verifier-policy sections of `docs/attestation-schema.md`, response-template test functions in `commit_test.go`, `--responses-template` + verifier-schema hunks of `newCommitCommand`/`runCommitRobots` in `main.go` | #11, #12 |
| 8 | Formatter-safe attestation JSON + stale guard | `MarshalIndent`/trailing-newline + `stagedAttestationPaths` hunks of `artifacts.go`, `internal/backpressure/artifacts_test.go`, formatter-safety sections of `docs/attestation-schema.md` | #10, #16 (partial) |
| 9 | Lint enforcement-level exposure | `internal/backpressure/lint.go`, `internal/backpressure/lint_test.go`, lint no-op/malformed test functions in `commit_test.go` | #8 |
| 10 | Recovery contract + `explain` | `explain` command + registration in `main.go`, `cmd/burpvalve/explain_test.go`, `docs/result-contract.md`, `NextSteps` hunks of `artifacts.go`/lint/commit outputs | #6 (contract) |
| 11 | Config foundation | `internal/config/config.go`, `internal/config/config_test.go`, `config`/`config init` commands in `main.go`, `cmd/burpvalve/config_test.go` (~1,150 lines, command-level), config-related assertions in `cmd/burpvalve/help_test.go` | - |
| 12 | Setup/init readiness | `internal/scaffold/inspect.go`, `apply.go`, `repair.go` + their three test files, readiness/`git-repo` assertions in `help_test.go` | #6, #9 |
| 13 | Hook feature-context in templates | both `templates/githooks/pre-commit` copies, hook-env-forwarding test functions in `internal/scaffold/template_contract_test.go` | #5 follow-up |
| 14 | Verifier prompt generator | `internal/backpressure/verifier_prompts.go`, `internal/backpressure/verifier_prompts_test/` (note: directory name as it exists), `cmd/burpvalve/verifier_test.go`, `verifier prompts` command + robot help in `main.go` | #13 |
| 15 | Attestation query + TUI | `internal/attestations/query.go`, `cmd/burpvalve/attestations_tui.go`, `cmd/burpvalve/attestations_test.go`, `cmd/burpvalve/attestations_tui_test.go`, `attestations list/show/latest/browse` commands in `main.go` | #13-adjacent |
| 16 | Beads delivery preflight | `beads preflight` command in `main.go`, `cmd/burpvalve/beads_test.go`, `bead_ids`/rationale hunks of `artifacts.go`, bead-metadata test functions in `commit_test.go`, beads robot-help assertions in `help_test.go` | #14 |
| 17 | Completion verify + guided install | completion block in `main.go`, `install.sh`, `cmd/burpvalve/install_script_test.go`, `docs/release-install.md`, completion-verify assertions in `help_test.go` | - |
| 18 | AGENTS template refresh | `internal/scaffold/agents_template.go`, AGENTS-related functions in `template_contract_test.go` | - |
| 19 | Arrow-chain sanitization | Replace bare `->` workflow chains in agent-facing prose and CLI output with numbered steps: `README.md`, `install.sh`, `skill/burpvalve/INSTALL.md`, `skill/burpvalve/SKILL.md`, `skill/burpvalve/references/deterministic-backpressure.md`, `docs/matt-pocock-skill-patterns.md`, `docs/setup-pre-commit-patterns.md`, `plans/bead-closure-mode.md`, the `create command shim %s -> %s` output string in `main.go`. Rule: bare `->` chains in prose or printed CLI output are defects; `->` inside fenced code blocks that are NOT presented as copy-paste shell workflows is allowed, as is symlink notation (`CLAUDE.md -> AGENTS.md`). | #16 |
| 20 | E2E harness | `cmd/burpvalve/e2e_test.go`, plus any residual `commit_test.go`/`help_test.go`/`init_test.go` hunks not owned by earlier units (final sweep — after this commit those three files must match the working set exactly) | - |
| 21 | Docs and skill refresh | `README.md` (feature docs), `docs/README.md`, `docs/CHANGELOG_RESEARCH.md`, `docs/matt-pocock-skill-patterns.md`, `docs/setup-pre-commit-patterns.md`, `skill/burpvalve/SKILL.md`, `skill/burpvalve/INSTALL.md`, `plans/GOAL.md`, scaffold template READMEs (`templates/backpressure/attestations/README.md`, `templates/log/...`, `internal/scaffold/templates/...`) | #7 etc. |

Unit 21 note: where a doc hunk documents exactly one unit's feature and
separates cleanly, prefer moving it into that unit's commit; the remainder
lands here as one documented docs commit with `--bead-rationale` naming the
covered features. Do not hold feature commits hostage to doc-hunk splitting.

## Main.go Protocol (resolves review blocker: split feasibility)

`cmd/burpvalve/main.go` has ~3,545 interleaved changed lines shared by units
7, 10, 11, 12, 14, 15, 16, 17, including moved/rewritten functions
(`newCommitCommand`, `newConfigCommand`) and shared robot-help maps.

1. **First deliverable before commit 6:** the landing agent produces a
   function-level staging map of the `main.go` diff — every added/changed
   function, command registration, and robot-help entry assigned to exactly
   one unit — and records it in `log/` (e.g.
   `log/landing-mainggo-staging-map.md`). Beads for units 6+ are not started
   until the map exists.
2. Staging is done per-function from that map (`git add -p`, edit hunks where
   necessary). After staging each slice, the staged-content check from the
   Gate Execution Contract must compile. A staged `cmd.AddCommand(...)`
   without its command constructor, or a test without its surface, is a
   staging error — fix before committing.
3. Where the map proves two units' `main.go` changes are genuinely
   inseparable (shared moved function, interleaved help map rewrite), combine
   those units into one commit with `--bead-rationale` naming both. Expected
   candidates per review: parts of 7+16 (commit-surface flags), 11+12
   (config consumption in setup/init/repair). Do not silently merge anything
   else.

## Verification Cells (explicit, not assumed)

1. **#6 exit-code inversion.** After unit 12 lands, reproduce the A/B from the
   issue comment (non-git dir vs git dir, `burpvalve init --force --json
   --no-beads --no-ntm` via `go run`): the non-git run must surface missing
   hooksPath wiring via status/`next_steps`/exit code; the optional NTM
   conflict must not out-rank it. Record the transcript in the unit-12
   attestation evidence.
2. **#16 zero-byte files.** Test the D4 hypothesis in a scratch directory
   (execute the arrow workflow line; confirm it creates `verify`, `close`,
   `sync`, `stage`, `Burpvalve`, `commit`). Unit 19 then removes every bare
   arrow chain per its rule. Post-unit-19 check: a repo-wide grep for
   `\-> ` in prose/output contexts returns only allowed cases. Comment
   findings on issue #16; close only if confirmed + sanitized.

## Issue Boundary Notes (resolves review risk 4)

- **#13**: units 14-15 deliver the prompt generator and attestation querying.
  The issue **stays open** for the orchestration scope in
  `plans/verifier-orchestration.md` (preferences, submit/ingestion,
  transcripts). Landing agents must not close #13 or extend these units.
- **#14**: unit 16 delivers read-only preflight + `bead_ids`. The issue
  **stays open** for closure mode (`plans/bead-closure-mode.md`). Same rule.
- **D9 goroutines**: no concurrency work lands in this plan. It is scoped in
  `plans/issue-17-guided-lint-setup.md` (tracked by commit 4), which is not
  part of the landing sequence.

## Acceptance Criteria

- `git status` clean after the sequence; every previously-untracked file is
  either committed by a named unit or covered by a named ignore rule.
- Commits 1-3 carry the documented bootstrap exception; every commit from 4
  onward has a staged, valid attestation produced under tracked rules;
  `burpvalve ci` passes on the final tree.
- `bin/burpvalve` absent from index AND worktree; `.ntm/` ignored;
  `AGENTS.md`/`CLAUDE.md` tracked with no TBDs.
- Staged-content build/test check ran for every commit (the stash/worktree
  method), not just full-tree checks.
- The `main.go` staging map exists in `log/` and matches what was committed.
- Issues #8, #9, #10, #11, #12, #15 closed with commit references; #6/#16
  handled per their verification cells; #13/#14 commented with boundary notes
  and left open.

## Risks

- The stash/pop cycle with `--include-untracked` on a tree this size is the
  riskiest mechanical step; if a stash conflict occurs, prefer the
  worktree/checkout-index method instead of force-resolving.
- Hunk-level staging of `main.go` remains the most error-prone step even with
  the map; budget for it and verify compile per slice.
- The bootstrap exception (commits 1-3 ungated) is a one-time, documented
  deviation from D1; do not let it expand past commit 3.
