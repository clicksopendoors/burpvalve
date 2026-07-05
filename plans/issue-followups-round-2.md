# Issue Followups Round 2

## Status

Approved, 2026-07-03. Revised after FuchsiaBear's adversarial gap review in
Agent Mail 3042 and approved by owner ruling on Agent Mail 3045. Convert this
plan to beads before assignment. Do not implement any resulting bead until the
bead list receives a quality-pass ruling.

This plan covers the four open approved follow-up issues from
`docs/burpvalve-issue-drafts-2026-07.md`:

- Issue draft 3: stale repo-local binary warning in `setup`/`repair`.
- Issue draft 7: full staged-path listing in verifier packets plus narrowed
  generated-prefix hash exclusion.
- Issue draft 8: payload ownership accounting helper for staged/untracked
  paths and split-file beads.
- Issue draft 9: supplemental verifiers and adjudication slots in the
  response/attestation schema.

## Source

Primary sources:

- `docs/burpvalve-issue-drafts-2026-07.md`, issue drafts 3, 7, 8, and 9.
- `docs/dogfooding-findings-2026-07.md` findings A3, A15, A16, and A17.
- FuchsiaBear adversarial review, Agent Mail 3042.
- Owner ruling on Agent Mail 3045:
  - Unit A first increment uses mtime/source freshness plus the short-timeout
    version check when available; unavailable or conflicting facts stay
    `unknown`.
  - Unit B must not add `staged_payload_all`; the union invariant over
    `staged_payload` and `hash_excluded_staged_payload` is the contract.
  - Unit C `expires_or_scope` is structured from round 1 as enum
    `single_bead`, `plan_round`, or `until_commit`, plus an optional
    free-form note field.
- Existing implementation state after the VORCH/OXP round:
  - `internal/gitindex/gitindex.go` already narrows generated-evidence
    detection to JSON files under generated evidence directories and excludes
    scaffold README files from generated-evidence classification.
  - `internal/backpressure/verifier_prompts.go` and `burpvalve hash --staged`
    already expose hash-included and hash-excluded staged path lists.
  - `internal/backpressure/verifier_responses.go` already defines
    `SupplementalVerifier` and `ResponseAdjudication` and validates/merges
    submit-time supplemental/adjudication input.
- `internal/attestations/attestations.go` and `docs/attestation-schema.md`
    still describe one primary `Condition` shape and do not yet commit a
    durable artifact-level compatibility contract for supplemental verifiers
    and adjudication.

## Draft 2 Review Resolutions

These resolutions are binding for bead conversion and override looser wording
elsewhere in the document.

1. **Unit A freshness is conservative and tri-state-plus-NA.** Repo-local
   binary freshness values are exactly `fresh`, `stale`, `unknown`, and
   `not_applicable`. Partial facts never produce a "safe" claim. Timeout,
   symlink, non-regular file, conflicting mtime/version facts, or unreadable
   metadata produce `unknown` with the observed facts.
2. **Unit B uses a union invariant, not a new list by default.** Do not add a
   `staged_payload_all` field. The contract is: every staged path is in
   exactly one of `staged_payload` or `hash_excluded_staged_payload`; no staged
   path may be omitted from both or duplicated in both. A duplicate all-paths
   field invites drift and is explicitly out of scope for this round.
3. **Unit C starts from an explicit ownership schema.** Round 1 has no implicit
   durable repo-local ownership config. The helper accepts stdin JSON and/or
   `--ownership-file <path>`. Beads may enrich display, but may not create
   ownership claims unless a later accepted design names a specific authoritative
   Beads field. `expires_or_scope` is structured from round 1 as enum
   `single_bead`, `plan_round`, or `until_commit`, with optional
   `expires_note` free-form context.
4. **Unit C is split before bead conversion.** Convert it as at least C1
   ownership schema/parser/result contract, C2 staged/untracked classifier
   command, and C3 optional Beads enrichment/docs/help. Do not land all of Unit
   C as one broad command bead.
5. **Unit D adjudication is audit metadata only in this round.**
   `adjudication.final_verdict` records the adjudicator's ruling but does not
   override artifact acceptance. A primary `fail` or `unknown` condition still
   blocks passing artifact validation even when adjudication records
   `final_verdict: pass`. Override semantics are future work requiring a
   separate authority-validation design.
6. **Absorption stop rules are mandatory.** Unit B must not rework existing
   generated-path/hash/prompt code when current behavior satisfies the contract;
   add missing regression tests/docs only. Unit D must reuse existing
   `SupplementalVerifier` and `ResponseAdjudication` submit-flow code rather
   than duplicating schema types.

## Problem

The v0.1.2 landing and the VORCH/OXP round closed the highest-risk workflow
gaps, but they also left four approved product follow-ups in a half-resolved
state. Some are brand-new work; others are already partially landed and need
absorption, regression tests, docs, and compatibility rules rather than a second
parallel implementation.

The common theme is evidence surface integrity:

- A hook may still run a repo-local binary whose age or origin makes the gate
  decision hard to trust.
- Verifiers must see every staged path, including generated evidence paths that
  are excluded from payload hashing, without broad prefix rules hiding tracked
  scaffold/docs files.
- Orchestrators need mechanical accounting from current git state to declared
  bead ownership, especially when one file is intentionally split across units.
- Attestations should preserve the real verification process when a condition
  has a primary verifier, supplemental verifier evidence, and an adjudication
  or ruling.

## Goals

- Make `setup` and `repair` explicitly warn when installed hooks would use a
  repo-local Burpvalve binary that may be stale, ignored, absent, or different
  from the preferred command path.
- Lock in full staged-path accounting for verifier packets and staged hashes:
  all paths are listed, generated evidence JSON is labeled and excluded, and
  scaffold/docs files under generated directories remain hash-included.
- Add a read-only ownership accounting surface that maps staged and untracked
  paths to known beads or declared ownership map entries, and flags unowned or
  conflicting paths before gate time.
- Complete the supplemental-verifier/adjudication schema story across response
  files, passing attestations, docs, queries, and compatibility tests without
  breaking older single-verifier artifacts.

## Non-Goals

- No workflow engine. Burpvalve does not assign agents, send Agent Mail, spawn
  NTM panes, or decide who owns a bead.
- No automatic staging, unstaging, committing, or file reservation management.
  Every new surface in this plan is read-only except existing `repair` behavior
  and schema-bearing commit/submit flows.
- No attempt to infer hunk ownership from prose with an LLM. Ownership
  accounting is path/function/test-area metadata plus explicit exceptions.
- No broad generated-directory exclusion. Only recognized generated evidence
  artifacts are hash-excluded.
- No breaking schema bump unless a later review proves it is required.
  Preferred path: additive schema fields under schema version 1 with legacy
  artifact compatibility.
- No GitHub issue filing or bead conversion in this plan.

## Binding Contracts

### Unit A: Stale Repo-Local Binary Warning

**Issue/finding:** issue draft 3; finding A3.

**Contract:**

1. `burpvalve setup --json` and human setup output must expose the command
   path a hook would use and warn when that path is a repo-local fallback with
   stale-risk facts.
2. `burpvalve repair` must preserve setup's warning facts in its result when
   repair installs, preserves, or detects a repo-local `bin/burpvalve`.
3. Warning severity is diagnostic, not fatal, unless an existing required check
   is already blocked. A valid but possibly stale repo-local fallback should not
   make a healthy repo fail setup; it should make the active gate path explicit.
4. The warning must name:
   - hook command source (`source`, `path`, `repo-local`,
     `repo-local-conflict`, or `missing`);
   - repo-local binary path when present;
   - whether `bin/burpvalve` is git-ignored;
   - whether a PATH command exists;
   - the comparison basis used for staleness, or `unknown` when no reliable
     comparison was possible.
5. Staleness detection should be conservative:
   - freshness status is exactly `fresh`, `stale`, `unknown`, or
     `not_applicable`;
   - if this repo contains `cmd/burpvalve`, compare `bin/burpvalve` mtime with
     the newest tracked file in the comparison set: `cmd/`, `internal/`,
     `go.mod`, `go.sum`, `internal/scaffold/templates/`, `templates/`, and
     install/release scripts that affect the built binary;
   - exclude `.git`, `bin/`, package/build output, generated logs,
     `backpressure/attestations/*.json`, and untracked work from the freshness
     mtime set; if relevant source dirt is detected but not included in the
     mtime set, emit `dirty_source_unknown`;
   - compare repo-local binary mtime with PATH command mtime only when both are
     regular files;
   - optionally compare `burpvalve --version` output when both commands can be
     run with a short timeout;
   - if facts conflict, time out, involve symlinks/non-regular files, or cannot
     be read, report `unknown`, not `fresh`.
6. Recovery text must not command users to delete or replace their binary. It
   should present explicit choices: run from source for this repo, install/use
   PATH, or intentionally keep repo-local fallback.
7. JSON output must expose stable fields for robot consumers:
   `hook_command_source`, `repo_local_path`, `repo_local_exists`,
   `repo_local_ignored`, `path_command`, `freshness_status`,
   `comparison_basis`, and `warning_code`.

**Why:** A3 was not that repo-local binaries are bad. The defect is silent
provenance ambiguity: users saw a gate result but could not tell whether the
result came from current source, installed binary, or stale repo-local shim.

### Unit B: Verifier Packet Staged-Path Accounting And Generated Exclusions

**Issue/finding:** issue draft 7; finding A15.

**Contract:**

1. Every verifier packet must list every staged path from the index.
2. Packet output must distinguish:
   - hash-included staged paths;
   - hash-excluded generated evidence paths;
   - generated evidence prefixes/rules;
   - bounded staged content details for paths where content excerpts are safe.
3. Hash exclusion must use `gitindex.IsGeneratedEvidencePath`, not broad prefix
   matching. Scaffold or docs files under `backpressure/attestations/` and
   `log/backpressure/failed/` must be hash-included unless they are recognized
   generated evidence artifacts.
4. `burpvalve hash --staged --json`, `burpvalve verifier prompts --json`, and
   human verifier prompt output must remain consistent on path classification.
5. Union invariant: all staged paths are exactly the union of `staged_payload`
   and `hash_excluded_staged_payload`; no staged path may be omitted from both;
   no staged path may appear in both. Docs and robot help must say
   `staged_payload` means hash-included staged paths, not all staged paths.
6. This unit is primarily an absorption and hardening unit. Existing VORCH work
   appears to have landed the core behavior; implementation agents must inspect
   current behavior first and add missing docs/tests only where the contract is
   not already satisfied.
7. Stop rule: if current `gitindex.IsGeneratedEvidencePath`, `hash --staged`,
   and `verifier prompts` already satisfy classification and the union
   invariant, implementation is limited to missing regression tests/docs.

**Why:** A15 had two defects that must stay separate. Path visibility is for
review completeness; hash exclusion is for payload identity. A generated file
can be visible to a verifier while excluded from the hash, and a scaffold README
under a generated directory can be included in the hash.

### Unit C: Payload Ownership Accounting Helper

**Issue/finding:** issue draft 8; finding A16.

**Contract:**

1. Add a read-only command, tentatively `burpvalve account payload`, with JSON
   and human output. The command name can change during implementation review,
   but the contract must not.
2. The command reports staged and optionally untracked paths against declared
   ownership data:
   - owning bead or unit id;
   - ownership kind: `whole_path`, `function`, `test`, `hunk`, `generated`,
     `admin`, or `exception`;
   - source of the ownership claim;
   - conflicts when two active units claim the same path incompatibly;
   - unowned paths;
   - generated evidence exceptions;
   - split-file paths that require human/agent hunk discipline.
3. The first implementation uses explicit metadata, not inference. It accepts
   stdin JSON and/or `--ownership-file <path>`. There is no implicit durable
   repo-local ownership file in round 1.
4. Ownership input schema fields:
   - `unit_id` (required);
   - `bead_id` (optional);
   - `path` (required, repo-relative);
   - `ownership_kind` (required: `whole_path`, `function`, `test`, `hunk`,
     `generated`, `admin`, or `exception`);
   - `symbol` (optional, for function/type/method ownership);
   - `test_pattern` (optional);
   - `hunk_label` (optional);
   - `source` (required: plan path, bead id, stdin, or explicit review packet);
   - `rationale` (required for shared/conflict-prone paths and exceptions);
   - `expires_or_scope` (optional enum: `single_bead`, `plan_round`, or
     `until_commit`);
   - `expires_note` (optional free-form context for the structured
     `expires_or_scope` value).
5. Source precedence:
   - explicit stdin JSON wins over `--ownership-file` when both provide the
     same unit/path/kind claim;
   - `--ownership-file` wins over display-only Beads enrichment;
   - Beads may identify active/open/in-progress tasks and show bead metadata,
     but must not create ownership claims unless a future accepted plan names a
     specific authoritative Beads field.
6. "Active unit" means explicit ownership records in input, plus open or
   in-progress Beads only when Beads enrichment is requested/available. Closed,
   deferred, tombstoned, and historical Beads are not conflict participants.
7. Output statuses are fixed for round 1: `owned`, `shared_declared`,
   `conflict`, `unowned`, `generated_exception`, `ignored_untracked`, and
   `covered_exception`.
8. The command must not mutate Beads, stage files, reserve files, or decide
   claims. It is an audit/readiness helper for orchestrators and committing
   agents.
9. For untracked paths, output must say whether each path is:
   - ignored by git;
   - under a generated evidence prefix;
   - covered by an explicit ownership/exception record;
   - unowned.
10. Split-file support is declarative in round 1. The helper may say
   `cmd/burpvalve/main.go` is shared by units A/B/C with named function/test
   areas; it does not need to parse Go AST hunks in the first bead unless the
   implementation agent can do so with small blast radius.

**Why:** A16 came from orchestration load, not from commit hashing. `hash
--staged` proves what payload is staged; it does not answer whether every path
belongs to the right bead or whether an untracked path is an intentional
exception. This helper is an ownership ledger and conflict detector.

### Unit D: Supplemental Verifiers And Adjudication Schema

**Issue/finding:** issue draft 9; finding A17.

**Contract:**

1. Response files and passing attestations must be able to preserve:
   - one primary verifier result;
   - zero or more `supplemental_verifiers`;
   - optional `adjudication` with authority, summary, final verdict when
     applicable, and audit reference.
2. Existing single-verifier artifacts remain valid. Query/explain/doctor
   surfaces must not reject older artifacts merely because they lack the new
   optional fields.
3. `verifier submit` must support three submission shapes:
   - primary-only;
   - supplemental-only, when a primary response is already present or when the
     caller is intentionally recording non-primary evidence;
   - adjudication-only, when resolving an already-recorded disagreement.
4. Merge rules must be deterministic:
   - duplicate supplemental verifier keys replace prior entries with a warning;
   - adjudication replaces prior adjudication with a warning;
   - primary verdict replacement retains existing supplemental/adjudication
     data unless explicitly cleared by a future command flag.
5. Passing artifact validation remains primary-condition-verdict plus
   verifier-policy based. `adjudication.final_verdict` is audit metadata only
   and never turns primary `fail` or `unknown` into a passing artifact.
6. Docs and prompt schemas must teach verifiers that supplemental evidence is
   additive. A supplemental verifier disagreement must trigger hold/escalate
   protocol; it must not be silently treated as a pass.
7. Future-work rule: adjudication override semantics, if ever desired, require
   a separate design naming authority validation, compatibility behavior, and
   schema/tool-version implications.
8. Stop rule: do not duplicate `SupplementalVerifier` or
   `ResponseAdjudication`; work starts from existing response-submit structs and
   merge behavior and propagates them into artifacts/query/docs where missing.

**Why:** A17 was about audit truth. If two verifiers and an orchestrator ruling
were required to make a commit trustworthy, the committed artifact should not
pretend only one verifier existed.

## File-Touch Map

### Unit A: stale repo-local binary warning

Likely files:

- `internal/scaffold/inspect.go`
- `internal/scaffold/inspect_test.go`
- `internal/scaffold/repair.go`
- `internal/scaffold/repair_test.go`
- `cmd/burpvalve/init_test.go`
- `cmd/burpvalve/e2e_test.go`
- `cmd/burpvalve/help_test.go`
- `docs/result-contract.md` if setup JSON fields or warning semantics change.

Avoid unless necessary:

- hook templates. This unit warns about what hooks would run; it should not
  change hook command resolution unless tests reveal setup and hook semantics
  already disagree.

### Unit B: staged-path packet accounting/generated exclusions

Likely files:

- `internal/gitindex/gitindex.go`
- `internal/gitindex/gitindex_test.go`
- `internal/backpressure/core.go`
- `internal/backpressure/core_test.go`
- `internal/backpressure/verifier_prompts.go`
- `internal/backpressure/verifier_prompts_test.go`
- `cmd/burpvalve/hash_test.go`
- `cmd/burpvalve/verifier_test.go`
- `cmd/burpvalve/help_test.go`
- `docs/attestation-schema.md` or `docs/result-contract.md` for path
  classification wording.

Expected absorption:

- If current behavior already satisfies the issue, this unit should be mostly
  regression tests and docs. Do not rework hashing or packet structs just to
  show activity.

### Unit C: payload ownership accounting helper

Likely files:

- `cmd/burpvalve/main.go`
- `cmd/burpvalve/help_test.go`
- new `cmd/burpvalve/account_test.go` or similar command tests
- new package under `internal/backpressure/` or `internal/accounting/` for
  read-only ownership classification
- `docs/result-contract.md`
- possibly `plans/README.md` only if the plan index needs this new plan listed
  after owner ruling.

Possible fixture files:

- `testdata` under the new internal package if ownership-map parsing needs
  stable examples.

Avoid:

- `.beads/` mutation. The helper can read Beads state; it must not update it.

### Unit D: supplemental verifier/adjudication schema

Likely files:

- `internal/attestations/attestations.go`
- `internal/attestations/attestations_test.go`
- `internal/backpressure/verifier_responses.go`
- `internal/backpressure/verifier_responses_test.go`
- `internal/backpressure/artifacts.go`
- `internal/backpressure/artifacts_test.go`
- `internal/backpressure/verifier_prompts.go`
- `internal/backpressure/verifier_prompts_test.go`
- `cmd/burpvalve/verifier_test.go`
- `cmd/burpvalve/attestations_test.go`
- `cmd/burpvalve/explain_test.go`
- `docs/attestation-schema.md`
- `docs/result-contract.md`

Expected absorption:

- `verifier_responses.go` already contains submit-time supplemental and
  adjudication structs/merge validation. Do not duplicate these types. The
  remaining work is to ensure the final attestation schema carries them,
  validation accepts them, query/explain surfaces display them, and docs/tests
  lock compatibility.

## Commit Sequencing

1. **Plan acceptance only.** Land this plan only after adversarial review and
   owner ruling, if the owner asks for a planning commit. This assignment does
   not authorize implementation beads.
2. **Unit B absorption/regression first.** It is mostly already covered and
   reduces risk for later verifier packet work. If it is fully absorbed, its
   bead can be a test/docs hardening commit.
3. **Unit D schema hardening second.** It depends on the packet/response
   vocabulary staying stable. Landing it before ownership accounting lets
   later helpers cite final evidence schema.
4. **Unit A setup warning third.** It touches setup/repair surfaces and can
   proceed independently, but it should not collide with Unit C command-help
   churn if both touch `cmd/burpvalve/main.go`.
5. **Unit C ownership helper last, split into at least three beads.**
   - C1: ownership input schema, parser, validation, and result contract; no
     command integration beyond fixtures.
   - C2: read-only staged/untracked classifier command with JSON/human output.
   - C3: Beads enrichment, docs, and help only if still desired after C1/C2.
   This prevents command wiring, schema design, git status parsing, generated
   exceptions, and Beads enrichment from landing as one broad feature.

Concurrency note:

- Units A and C both likely touch `cmd/burpvalve/main.go` and robot help.
- Units B and D both likely touch verifier prompt files.
- Use Agent Mail reservations per bead and prefer non-overlapping work. Do not
  stage tracker/admin closure over another agent's gate window.

## Test Plans

### Unit A tests

- `go test ./internal/scaffold -run 'Inspect|Repair|RepoLocal|Tool|Hook|Setup'`
- `go test ./cmd/burpvalve -run 'Init|Setup|Repair|Explain|Robots|Help|RepoLocal|Hook'`
- `go test ./cmd/burpvalve -run 'E2E|Setup|RepoLocal'` if E2E fixtures are
  extended.
- `go test ./...`

Required scenarios:

- no repo-local binary, PATH command present: no stale warning;
- repo-local binary present and hook would use PATH: report repo-local fact but
  do not warn as active hook source;
- repo-local binary present and hook would use repo-local: warning names source
  and path;
- repo-local binary ignored by git: warning includes ignored/conflict fact;
- repo-local binary older than source tree or PATH command: warning says stale;
- mtime/version unavailable, version check timeout, symlink command, or
  non-regular command: warning says `unknown` rather than `fresh`;
- source dirt present outside the mtime comparison set: comparison basis records
  `dirty_source_unknown`;
- repair result preserves warning facts after installing or preserving
  `bin/burpvalve`.

### Unit B tests

- `go test ./internal/gitindex -run 'Generated|Evidence|Path'`
- `go test ./internal/backpressure -run 'Verifier|Prompt|Hash|Generated|StagedPayload'`
- `go test ./cmd/burpvalve -run 'Verifier|Hash|Robots|Help|Generated|Staged'`
- `go test ./...`

Required scenarios:

- staged `backpressure/attestations/README.md` and
  `log/backpressure/failed/README.md` are listed and hash-included;
- staged generated attestation JSON is listed and hash-excluded;
- packet JSON has all staged paths even when some are hash-excluded;
- verifier prompt JSON and hash JSON satisfy the union invariant:
  `staged_payload` union `hash_excluded_staged_payload` equals all staged paths,
  with no duplicates;
- human prompt output names hash-excluded generated paths separately;
- `hash --staged --json` and verifier packet JSON agree on classification.

### Unit C tests

- `go test ./cmd/burpvalve -run 'Account|Payload|Owner|Staged|Untracked|Robots|Help'`
- `go test ./internal/... -run 'Account|Payload|Owner|Split|Generated'` for the
  package that owns classification.
- `go test ./...`

Required scenarios:

- ownership schema rejects missing `unit_id`, missing `path`, invalid
  `ownership_kind`, and exception/shared records without rationale;
- staged path with one owner reports `owned`;
- staged path with no owner reports `unowned`;
- staged path with two incompatible owners reports `conflict`;
- split-file path with declared function/test ownership reports `shared` and
  names all owners;
- generated evidence JSON reports generated exception;
- untracked ignored path is reported as ignored;
- untracked non-ignored path without ownership reports `unowned`;
- ignored generated-response scratch path and untracked non-ignored source path
  are distinct statuses;
- command is read-only: repeated runs do not change git status.

### Unit D tests

- `go test ./internal/attestations -run 'Supplemental|Adjudication|Schema|Validate|Legacy'`
- `go test ./internal/backpressure -run 'VerifierSubmit|Supplemental|Adjudication|Artifact|Blocked|Passing'`
- `go test ./cmd/burpvalve -run 'Verifier|Attestations|Explain|Supplemental|Adjudication|Robots|Help'`
- `go test ./...`

Required scenarios:

- legacy single-verifier artifact validates shape;
- primary plus supplemental verifier survives `verifier submit` merge and
  `burpvalve commit` artifact writing;
- supplemental-only submit adds evidence without overwriting primary;
- duplicate supplemental verifier replaces with warning;
- adjudication-only submit records authority/summary/audit ref;
- primary `fail` or `unknown` plus adjudication `final_verdict: pass` still
  blocks passing artifact validation;
- disagreement case is representable and queryable;
- query/explain output displays supplemental disagreement and instructs
  hold/escalate rather than hiding it behind the primary verdict;
- legacy single-verifier artifacts remain readable/queryable.

## Acceptance Criteria

- Issue 3: `setup`/`repair` JSON and human output make active hook command
  provenance explicit and warn on stale-risk repo-local fallback without
  forcing a workflow.
- Issue 7: verifier packets and `hash --staged` show complete staged path
  accounting with generated evidence JSON excluded and scaffold/docs files
  included.
- Issue 8: a read-only ownership helper reports staged and untracked path
  ownership, generated exceptions, unowned paths, conflicts, and split-file
  declarations in JSON and human output from an explicit stdin or
  `--ownership-file` schema.
- Issue 9: response files, passing attestations, docs, and query/explain
  surfaces preserve supplemental verifier and adjudication metadata while
  keeping legacy artifacts valid; adjudication is audit metadata only and cannot
  make a failing primary cell pass.
- Every implementation bead produced from this plan has its own staged-slice
  verification and Burpvalve gate evidence. This plan itself does not authorize
  bead conversion.

## Owner Rulings

The owner resolved the remaining open questions on 2026-07-03. These rulings
are binding for bead conversion and implementation:

1. Unit A first increment uses mtime/source freshness plus the short-timeout
   version check when available. Unavailable, timed out, or conflicting facts
   remain `unknown`.
2. Unit B must not add a `staged_payload_all` field. The union invariant over
   `staged_payload` and `hash_excluded_staged_payload` is the contract because a
   duplicate all-path list invites drift.
3. Unit C `expires_or_scope` is structured from round 1 as enum `single_bead`,
   `plan_round`, or `until_commit`, plus optional free-form `expires_note`.
   Loosening a structured schema later is cheaper than tightening an
   unstructured string after adoption.
