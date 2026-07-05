# Changelog

This is a synthesized, agent-facing changelog for the full history of
`burpvalve`.

Scope window: project inception on 2026-06-20 through the v0.2.0 private launch
release preparation on 2026-07-05.

This document was rebuilt from git history, GitHub release metadata,
`.beads/issues.jsonl`, and the working tree. The v0.1.2 section was written
from the live landing sequence, committed attestations, and GitHub issue
records rather than prior-release reconstruction. The v0.1.3 section was
written from the post-v0.1.2 dogfooding and release-prep chain. The v0.2.0
section was written from the private open-source launch preparation chain.
It is intentionally organized by landed capabilities, not raw diff order.

## Version Timeline

| Version | Kind | Date | Summary |
|---------|------|------|---------|
| v0.2.0 | Release | 2026-07-05 | Private launch release with MIT licensing, public skill metadata, licensed archives, scrubbed public-facing docs, SECURITY reporting policy, public install defaults, Claude/orchestrator route support, ownership accounting, and final pre-flip gate evidence. |
| v0.1.3 | Release | 2026-07-03 | Ships the guided lint setup, verifier orchestration, Beads close/drift flow, prompt bank and prompt export surfaces, orchestrator target/config work, commit evidence helpers, hook recovery hints, NTM bridge docs, skill prompt-bank refactor, and dogfood-mode findings prompt. |
| v0.1.2 | Release | 2026-07-02 | Dogfoods Burpvalve on its own landing sequence, tracks the live gate scaffold and attestations, adds verifier provenance/policy, improves setup/lint/recovery evidence, handles staged deletes, and removes source-tracked release artifacts. |
| v0.1.1 | Release | 2026-06-22 | Adds global/project config defaults, makes repo-local `bin/burpvalve` opt-in, teaches completion setup to handle command PATH wiring, and updates the agent skill for JSON/robot flows. |
| v0.1.0 | Release | 2026-06-21 | First packaged `burpvalve` release with the consolidated CLI, skill-only bootstrap path, cross-platform binary assets, targeted init/repair controls, and authenticated-repo friendly installer downloads. |
| [`5d1c1cd`](https://github.com/clicksopendoors/burpvalve/commit/5d1c1cd89c5f5d3000aeec42d98fe3753dece9ee) | Commit | 2026-06-20 | Built the first working Burpvalve agent backpressure gate. |
| [`abf3234`](https://github.com/clicksopendoors/burpvalve/commit/abf3234f6d517330a2565cfe6d27a62f94bdea5b) | Commit | 2026-06-20 | Renamed the human-approval condition to autonomy boundary in the plan. |
| [`bd617e0`](https://github.com/clicksopendoors/burpvalve/commit/bd617e03eddbfdf10e2bb34c54d90274210b30b7) | Commit | 2026-06-20 | Added the final compile and commit-gate verification bead. |
| [`fb1d62f`](https://github.com/clicksopendoors/burpvalve/commit/fb1d62fc11072af3183091119140e2c4383c43a9) | Commit | 2026-06-20 | Initialized the original planning state and Beads state. |

## v0.2.0 - Private Launch Release

This release is the private open-source launch package prepared before the
visibility flip. It keeps the repository private during release creation, but
ships the source tree, skill package, and archives with the public metadata and
license files required for the later flip. Earlier versions were private
pre-releases.

### Delivered capability

- Switched the repository and packaged skill to MIT/public launch metadata,
  including root `LICENSE`, third-party notices, public install defaults, and
  release archives that carry the license and notices bundle.
- Rewrote public-facing README, install, contribution, security, demo, and
  runbook surfaces for the `clicksopendoors/burpvalve` repository and removed
  stale private/proprietary wording from the public tree.
- Added `SECURITY.md` with private vulnerability reporting guidance and kept
  GitHub private vulnerability reporting as a documented post-flip enablement
  step while the repository remains private.
- Added Claude route support, including route config schema, route-aware
  scaffold internals, generated Claude orchestrator skill packaging, public CLI
  route surfaces, and end-to-end route acceptance coverage.
- Added payload ownership accounting and display-only Beads accounting context
  so staged work can describe ownership without changing payload hashes.
- Added stale repo-local binary warnings and public installer defaults so users
  are guided toward pinned release packages instead of source-local binaries or
  Go module installation.
- Codified launch-window orchestration standards: model tiering, role split,
  Agent Mail registration, contact mesh preapproval, polling standards, gate
  operator ritual, and main-agent verification policy for the launch window.
- Scrubbed current-tree launch residue, fixture emails, local paths, legacy
  naming, launch-policy wording, and tracker prefix data needed for the fresh
  public tree.
- Deleted the old v0.1.0 and v0.1.1 GitHub releases while preserving their
  tags, so stale private pre-release assets do not become public release
  downloads.
- Recorded release-size evidence from local packaging: the linux/amd64,
  darwin/amd64, and darwin/arm64 binaries crossed the 8 MiB warning threshold,
  with the largest observed binary at 8,927,312 bytes, below the 12 MiB hard
  failure limit.
- Closed the final private pre-flip gate with a read-only re-audit covering
  runtime artifacts, legacy-prefix residue, scrub campaign residue, dependency
  graph health, release-deletion completion, and private vulnerability reporting
  deferral.

### Representative commits

- [`e6b5173`](https://github.com/clicksopendoors/burpvalve/commit/e6b5173) packages MIT license and third-party notices.
- [`2e3f64f`](https://github.com/clicksopendoors/burpvalve/commit/2e3f64f) sets the public installer default.
- [`b10ae74`](https://github.com/clicksopendoors/burpvalve/commit/b10ae74) adds the security reporting policy.
- [`f05aeac`](https://github.com/clicksopendoors/burpvalve/commit/f05aeac) scaffolds the Claude orchestrator skill package.
- [`8ecdb73`](https://github.com/clicksopendoors/burpvalve/commit/8ecdb73) adds route-aware Claude scaffold behavior.
- [`6a351e8`](https://github.com/clicksopendoors/burpvalve/commit/6a351e8) exposes Claude route CLI and robot setup surfaces.
- [`5576214`](https://github.com/clicksopendoors/burpvalve/commit/5576214) adds payload ownership accounting.
- [`2bf5bbd`](https://github.com/clicksopendoors/burpvalve/commit/2bf5bbd) scrubs current-tree launch residue.
- [`81b8322`](https://github.com/clicksopendoors/burpvalve/commit/81b8322) rewrites public README and docs.
- [`fa7215a`](https://github.com/clicksopendoors/burpvalve/commit/fa7215a) scrubs fixture email residue.
- [`979dae0`](https://github.com/clicksopendoors/burpvalve/commit/979dae0) scrubs legacy naming residue.
- [`6df0acf`](https://github.com/clicksopendoors/burpvalve/commit/6df0acf) renames the Beads prefix to `burpvalve`.
- [`d38e3e8`](https://github.com/clicksopendoors/burpvalve/commit/d38e3e8) closes the pre-flip gate with re-audit evidence.

## v0.1.3 - Guided Setup, Orchestration, And Evidence Surfaces

This release is the July 3 dogfooding wave on top of v0.1.2. It packages the
parallel plan chains that grew out of the live landing sequence: guided lint
configuration, verifier orchestration, Beads close-mode delivery, orchestrator
experience, prompt-bank reuse, and release-critical evidence fixes. The release
is not yet public; repository visibility and public-launch metadata are unchanged.

### Delivered capability

- Added guided lint setup: manifest writing, `lint init` CLI/robot wiring,
  built-in Go and Node/Astro preset templates, a guided init wizard, bounded
  deterministic parallel lint execution, and parallel setup-probe inspection.
- Added verifier orchestration surfaces: response-file binding, verifier begin
  templates, verifier submit ingestion, response auto-discovery, and a
  report-only verifier doctor. Generated path exclusions were fixed so verifier
  evidence does not self-contaminate staged payload checks.
- Added Beads delivery support: preflight payload classification, the beads
  close state machine, close-resume fixtures, admin and multi-bead close paths,
  and attestation-keyed drift checking.
- Added prompt-bank tooling: `burpvalve prompts` list/show APIs, canonical
  prompt-bank content, stable prompt export metadata, content-hash divergence
  protection, and skill documentation that tells agents to use the installed
  prompt bank instead of stale copied prompt text.
- Added orchestrator experience option-B work: config schema support,
  explicit `ORCHESTRATOR.md` scaffold target, install/config surfaces for the
  orchestrator contract, and NTM bridge documentation calibrated to real
  peer-pane verification.
- Added commit evidence helpers: `hash --staged`, `BURPVALVE_BEAD` forwarding
  through hook context, commit-scoped `ci --commit` audits, and a multi-cluster
  historical commit fix for feature-scoped CI assertions.
- Added hook-aware recovery text so failed commit gates point at context-aware
  next steps rather than generic retry advice.
- Added dogfood mode for orchestrator findings: an opt-in scaffold/config
  surface that tells orchestrators to log issues and complications into a
  reviewable findings log using numbered entries with Why and How-to-apply.
- Filed and triaged the post-v0.1.2 tracker issue batch, closing already-shipped
  follow-ups and leaving the remaining product follow-ups open for later plans.
- Recorded OSL owner decisions and launch-audit blockers without changing repo
  visibility or open-source launch metadata.

### Closed and boundary workstreams

- The guided lint setup issue (#17) receives the implemented init, preset,
  wizard, and parallel-execution pieces.
- The verifier orchestration follow-up receives begin/submit/doctor/reporting
  surfaces, while full autonomous verifier fanout remains governed by repo
  policy and runtime capabilities.
- Beads close-mode now has delivery state machinery and drift checks, but
  launch/public cleanup and history scrubbing remain in the OSL chain.
- Orchestrator option-B ships through `ORCHESTRATOR.md`; option-A `CLAUDE.md`
  orchestrator migration remains deferred.
- Public launch, license flip, and anonymous verification are still out of
  scope for this release.

### Representative commits

- [`3c3ff89`](https://github.com/clicksopendoors/burpvalve/commit/3c3ff8946810e865753427d454b61edc3c120d7f) adds lint manifest writing.
- [`b8b39f8`](https://github.com/clicksopendoors/burpvalve/commit/b8b39f8a0893c9f496f2dc998444f9f12b145722) adds built-in lint presets.
- [`05d45b9`](https://github.com/clicksopendoors/burpvalve/commit/05d45b9f6ca8578d035539952e91e93979cf9734) runs lint commands in parallel.
- [`372db52`](https://github.com/clicksopendoors/burpvalve/commit/372db5296fbe74b837a6b44137c172ffdad34c7e) binds verifier prompt packets.
- [`66e127d`](https://github.com/clicksopendoors/burpvalve/commit/66e127d5d109257a9190321379a08b3fde482d34) ingests verifier submissions.
- [`908cfa1`](https://github.com/clicksopendoors/burpvalve/commit/908cfa1d30c37cd0a9faa00fa9963f4fc4fcc004) adds verifier doctor reporting.
- [`023570b`](https://github.com/clicksopendoors/burpvalve/commit/023570b6209b01ee400df37586fd7437c701e70b) adds the Beads close state machine.
- [`f2d3907`](https://github.com/clicksopendoors/burpvalve/commit/f2d39078444454029e55f654d8a8d71cf218203d) adds attestation-keyed drift checks.
- [`711063d`](https://github.com/clicksopendoors/burpvalve/commit/711063d1b70b1672197ed06bd096636c019ef879) adds the prompts command API.
- [`126a0fc`](https://github.com/clicksopendoors/burpvalve/commit/126a0fc257f6f51439ed2c29888c28338e728c55) adds canonical prompt-bank content.
- [`932abfc`](https://github.com/clicksopendoors/burpvalve/commit/932abfc726739822096d548f5f12611f458dcbb0) adds the orchestrator scaffold target.
- [`8520bfb`](https://github.com/clicksopendoors/burpvalve/commit/8520bfb61b1f34313007682ab4335718cb45615f) forwards bead metadata through hooks.
- [`1ead769`](https://github.com/clicksopendoors/burpvalve/commit/1ead769796f1600ed41b4a67ba3fcbcdfe90254c) adds the staged payload hash helper.
- [`0c41470`](https://github.com/clicksopendoors/burpvalve/commit/0c414706f9f16957d4d469b2bc2c9292855bd4d) adds commit-scoped CI audit.
- [`92a8e17`](https://github.com/clicksopendoors/burpvalve/commit/92a8e17a15e866b0bc6a5306faafdd40ddc23535) fixes multi-cluster CI commit assertions.
- [`e3464a4`](https://github.com/clicksopendoors/burpvalve/commit/e3464a40f3012f8dc2ac76cef8284f15c35f105c) adds hook-aware recovery steps.
- [`a17a1ca`](https://github.com/clicksopendoors/burpvalve/commit/a17a1ca9ff5713a758c5e24a40c14f901a00742b) adds opt-in orchestrator dogfood mode.

## v0.1.2 - Self-Hosted Gate Evidence And Agent-Safe Release Prep

This release is the July 2 landing sequence on top of v0.1.1. It turns the
repository into Burpvalve's own dogfooding target: every working-set commit was
gated, attested, and reviewed through the tool's live rules before release
prep continued.

### Delivered capability

- Added the repository operating contract (`AGENTS.md` with `CLAUDE.md`
  symlink), tracked the live pre-commit hook, committed the manifest and
  condition files, and added log/attestation/tooling README files so future
  evidence binds to rules stored in history.
- Dropped tracked runtime/release artifacts from source control: `.ntm/` is
  ignored, the stale repo-local `bin/burpvalve` binary is gone, and committed
  `dist/` archives/checksums are untracked so GitHub Release assets remain the
  canonical distribution channel.
- Added staged delete/rename handling for commit feature detection, covering
  normal file removals and migrations without reading deleted staged blobs
  ([#15](https://github.com/clicksopendoors/burpvalve/issues/15)).
- Added stale-attestation protection and stable generated JSON evidence so
  passing artifacts cannot be reused after payload drift and common formatter
  checks have a clearer generated-evidence boundary
  ([#10](https://github.com/clicksopendoors/burpvalve/issues/10)).
- Made no-op lint structurally distinct from enforced lint evidence with
  `not_enforced`, `enforced`, `command_count`, `evidence_strength`,
  `enforcement_level`, and `coverage` fields
  ([#8](https://github.com/clicksopendoors/burpvalve/issues/8)). The larger
  guided lint setup remains tracked separately in #17.
- Added verifier provenance and policy fields to attestation cells, response
  templates, and validation. Cells can now record verifier kind, model, runtime,
  separate-context status, transcript references, and manifest-level policy
  such as `independent_required` or `main_agent_allowed`
  ([#11](https://github.com/clicksopendoors/burpvalve/issues/11),
  [#12](https://github.com/clicksopendoors/burpvalve/issues/12)).
- Clarified that verifier authorization is configuration/policy, never
  per-cell evidence. The release keeps the fail-closed path instead of
  simulating or fabricating subagent confirmation
  ([#12](https://github.com/clicksopendoors/burpvalve/issues/12)).
- Added `burpvalve verifier prompts` to generate condition-specific verifier
  packets and response schemas. This is the prompt-generation foundation for
  [#13](https://github.com/clicksopendoors/burpvalve/issues/13); full spawn
  profiles, prompt storage, submit ingestion, and runtime-limit orchestration
  are deferred to `plans/verifier-orchestration.md`.
- Added attestation query surfaces, including list/show/latest commands and a
  TUI browser, so agents and orchestrators can inspect passing and blocked
  evidence without manually reading JSON artifacts.
- Added delivery bead IDs to attestations and a read-only Beads preflight
  report. This is the foundation for
  [#14](https://github.com/clicksopendoors/burpvalve/issues/14); full bead
  closure mode and close-before-commit sequencing are deferred to
  `plans/bead-closure-mode.md`.
- Added recovery and explain result contracts, including structured
  `next_steps`, `fatal`, and `partial_success` signals for setup/init/repair
  flows.
- Improved non-git setup and hook wiring behavior with explicit `--git-init`
  affordances and exact recovery paths, while distinguishing required hook
  wiring from optional NTM registration conflicts
  ([#6](https://github.com/clicksopendoors/burpvalve/issues/6),
  [#9](https://github.com/clicksopendoors/burpvalve/issues/9)).
- Added `completion verify` and install-path smoke coverage for shell
  completion/PATH setup.
- Reproduced the zero-byte-file report as a shell parsing hazard from bare
  arrow workflow prose, sanitized active agent-facing docs and CLI output, and
  left historical examples clearly marked as historical or fenced text
  ([#16](https://github.com/clicksopendoors/burpvalve/issues/16)).
- Added an end-to-end test harness plus residual regression coverage for setup,
  init, repair, commit, CI, explain, verifier prompts, attestations, install
  smoke, and completion verification.

### Closed and boundary workstreams

- #6, #8, #9, #10, #11, #12, and #15 receive concrete release functionality in
  this window.
- #13 and #14 receive foundations and detailed follow-on plans, not full
  orchestration/closure-mode implementation.
- #16 receives reproduction evidence and active-doc sanitization; the release
  notes should not claim that Burpvalve itself created those files.
- #17 intentionally stays open for guided lint setup and language/preset
  configuration.

### Representative commits

- [`8ab3fae`](https://github.com/clicksopendoors/burpvalve/commit/8ab3fae7ef447993685e93e85f82625ff4de2b81) handles staged deletes and renames.
- [`c180cb0`](https://github.com/clicksopendoors/burpvalve/commit/c180cb0c5308b66327cccc62485b0278c72152ec) adds verifier provenance and policy.
- [`2cdc1ce`](https://github.com/clicksopendoors/burpvalve/commit/2cdc1cec2570572bdfb230ceaac53e0f67cbc2e2) guards stale attestation evidence.
- [`bb74e4c`](https://github.com/clicksopendoors/burpvalve/commit/bb74e4ca6e44ab293575dbf2a0aaf50352ca5655) exposes lint enforcement status.
- [`3c2671f`](https://github.com/clicksopendoors/burpvalve/commit/3c2671ff52385c10709aad3e4436a106bcdcedda) adds setup/init readiness behavior.
- [`26a4882`](https://github.com/clicksopendoors/burpvalve/commit/26a4882383f1964c78730d923714c26ea532c540) adds verifier prompt generation.
- [`d650b9a`](https://github.com/clicksopendoors/burpvalve/commit/d650b9a017bac0f8bec9b1d8f7492f18d4dc88ee) adds attestation query and browser surfaces.
- [`3f05003`](https://github.com/clicksopendoors/burpvalve/commit/3f05003faa84e74fd9cb492d1c982cae2d30671a) adds Beads delivery preflight and bead IDs.
- [`94c3465`](https://github.com/clicksopendoors/burpvalve/commit/94c3465ee31b16f2205ca569dae0484f197156f8) sanitizes unsafe arrow-chain workflows.
- [`a21e4ef`](https://github.com/clicksopendoors/burpvalve/commit/a21e4ef87defc232ea41985627020a605c7e12ac) untracks committed release archives from source.

## v0.1.1 - Config Defaults And Global PATH Model

### Delivered capability

- Added lightweight JSON config loading from `~/.config/burpvalve/config.json`
  with `.burpvalve.json` project overrides.
- Added `burpvalve config` to show config paths and merged defaults.
- Changed standard `init` and `repair` so repo-local `bin/burpvalve` is opt-in
  via `--repo-bin`, the `bin` target, or `repo_bin: true` config.
- Changed the generated pre-commit hook to prefer global `burpvalve` on `PATH`
  and fail with an install hint when neither global nor repo-local fallback is
  available.
- Extended completion setup so it can install shell completions, create a
  global command shim, and add the shim directory to shell startup files.
- Updated the Burpvalve skill instructions for the new human TUI versus agent
  JSON/robot workflow, including explicit verifier-evidence rules.
- Regenerated the README demo artifacts with a longer, turn-by-turn VHS capture.

## v0.1.0 - Burpvalve CLI Consolidation

This working-tree wave turns the original multi-script shape into one coherent
developer CLI and packaged skill.

### Delivered capability

- Replaced separate setup and commit binaries with one `burpvalve` command
  surface: `setup`, `init`, `repair`, `commit`, `lint`, `ci`, and `version`.
- Added Cobra-powered help with first-run descriptions, quick starts, shell
  integration notes, two-column command listings, and per-command help.
- Kept hidden legacy `--mode` compatibility for existing scripted callers.
- Changed release packaging to ship only `scripts/bin/burpvalve`.
- Changed the installer to install one command shim and default skills install
  destination to `~/skills`.
- Added targeted `burpvalve init [target...]` and `burpvalve repair [target...]`
  controls for scoped setup and repair of pieces such as `AGENTS.md`,
  `CLAUDE.md`, `log`, `attestations`, `hooks`, `precommit`, `hooks-path`, and
  `bin/burpvalve`.
- Expanded opt-out flags for `init` and `repair`, including targeted skips for
  Beads, NTM, AGENTS.md, CLAUDE.md symlink, docs, plans, log, backpressure,
  attestations, hooks, hook path, repo-local binary, and tool docs.
- Changed `repair CLAUDE.md` to preserve regular-file content by importing it
  into `AGENTS.md` before replacing `CLAUDE.md` with a symlink.
- Removed project registry preview and registry mutation from setup/init/repair.
- Added regression coverage proving local `.registry/projects.yaml` is ignored
  and not mutated.

### Closed workstreams

- Supersedes the older `setup-command-project` and `burpvalve-commit` naming
  used in the original Beads plan.
- Establishes the first GitHub release package as the supported remote install
  path.

## 1. Project Readiness Plan And Work Graph

The first wave established the problem, the operating model, and the issue graph
for building a reusable setup/backpressure tool.

### Delivered capability

- Created `plans/GOAL.md` as the original implementation plan.
- Initialized Beads state under `.beads/`.
- Added a final compile/run/commit-gate verification bead.
- Renamed the "human approval" condition to "autonomy boundary" before the first
  implementation landing.

### Closed workstreams

- Planning and bootstrap records live in `.beads/issues.jsonl`.
- The planning text still contains historical working titles and setup-era
  vocabulary.

### Representative commits

- [`fb1d62f`](https://github.com/clicksopendoors/burpvalve/commit/fb1d62fc11072af3183091119140e2c4383c43a9) created the initial plan and Beads repository state.
- [`bd617e0`](https://github.com/clicksopendoors/burpvalve/commit/bd617e03eddbfdf10e2bb34c54d90274210b30b7) added terminal verification as an explicit closeout workstream.
- [`abf3234`](https://github.com/clicksopendoors/burpvalve/commit/abf3234f6d517330a2565cfe6d27a62f94bdea5b) corrected the backpressure vocabulary around autonomy boundaries.

## 2. First Working Backpressure Gate

The first implementation landing turned the plan into a Go module with scaffold
inspection, mutation, repair, commit evidence, lint, CI, skill packaging, and
release automation.

### Delivered capability

- Added Go module structure, tests, build targets, and binary size budget checks.
- Implemented scaffold templates for `AGENTS.md`, docs, plans, logs,
  `backpressure/`, and `.githooks/pre-commit`.
- Implemented readiness inspection, init, repair, Beads setup, NTM reporting,
  and fixture-based workflow tests.
- Implemented the backpressure plan model: manifest loading, condition order,
  condition hashes, staged payload hashes, feature detection, and matrix cells.
- Implemented passing attestation artifacts and local blocked-evidence reports.
- Added CI validation for committed or staged evidence.
- Added manifest-declared lint command execution.
- Added release workflow assets for linux/darwin and amd64/arm64.

### Closed workstreams

- `.beads/issues.jsonl` records PRS-00 through PRS-14 and final verification as
  closed.
- The original implementation included now-removed project registry preview and
  separate `burpvalve-commit` packaging.

### Representative commits

- [`5d1c1cd`](https://github.com/clicksopendoors/burpvalve/commit/5d1c1cd89c5f5d3000aeec42d98fe3753dece9ee) landed the first end-to-end Burpvalve gate, tests, docs, skill wrapper, CI, and release workflow.

## Notes for Agents

- Start with the version timeline for chronology.
- Treat `v0.1.0` as the first packaged release baseline.
- Use `README.md` for user setup and `docs/ARCHITECTURE.md` for codebase
  orientation.
- Treat `plans/GOAL.md` as historical planning evidence, not the current command
  reference.
- Do not reintroduce registry setup. Registry preview was intentionally removed.
