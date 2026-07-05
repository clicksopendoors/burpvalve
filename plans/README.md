# Plans

Plans live here. A plan explains what should be built and why. Actionable work
from a plan should be converted into beads with dependencies, so agents can
execute without re-reading the whole plan.

Workflow rule: discuss first, then write the detailed plan here in Markdown,
then convert the plan into detailed beads, and only then assign work to
agents. A plan does not exist until it exists in this directory. Decisions
behind the current round are recorded in
[docs/decisions-2026-07-02-review-round.md](../docs/decisions-2026-07-02-review-round.md).

Review round: on 2026-07-01/02 every plan below was independently reviewed by
two Codex agents (structured gap reports via Agent Mail to RusticDog,
messages 2264-2275). All six came back "needs revision"; all six were revised
to revision 2 the same day, addressing every blocker.

## Plan Index

Execution order for the 2026-07 round is top to bottom: landing blocks
everything; the launch plan is explicitly last.

| Plan | Status | Notes |
|------|--------|-------|
| [orchestration-goal-prompt.md](orchestration-goal-prompt.md) | **Active** (autonomy by documentation, D11) | Standing mission prompt for the session orchestrator: completion definition for the 2026-07 round, operating tick, roles, gate rules, autonomy-by-documentation contract, and the issue protocol (document + double-check + proceed; owner vetoes after the fact; hard exceptions escalate). |
| [orchestrator-experience.md](orchestrator-experience.md) | Draft, queued for two-agent review after current round | D12 direction: Burpvalve for the orchestrator role — contract file options (`ORCHESTRATOR.md` vs `CLAUDE.md`-as-orchestrator at init), `burpvalve prompts` bank, evidence surfaces (`ci --commit`, `hash --staged`, `BURPVALVE_BEAD`), NTM peer-pane patterns as documented guidance. |
| [land-working-set.md](land-working-set.md) | Rev 2 after review — awaiting bead conversion | Land the working set as atomic gated commits. Rev 2 adds the Phase 0 gate bootstrap (scaffold tracked first, stale binary deleted, `go run` gate contract with a documented one-time exception), staged-content verification per commit, the mandatory `main.go` function-level staging map, an arrow-sanitization unit for #16, and exhaustive file-to-unit mapping. Blocks all other plans. |
| [release-v0.1.2.md](release-v0.1.2.md) | Rev 2 after review — awaiting bead conversion (after landing) | Changelog, badges + CI badge, packaging, annotated-tag push sequence, pre-release local smoke + post-release pinned smoke. Rev 2 untracks `dist/` artifacts (owner may veto), fixes the tag-trigger sequence, and validates the extracted package. |
| [issue-17-guided-lint-setup.md](issue-17-guided-lint-setup.md) | Rev 2 after review — awaiting bead conversion (after landing) | Guided `burpvalve lint init` wizard + D9 goroutine work. Rev 2 adds the binding Implementation Contract: schema/runtime units for `run_directory`/`serial` (unknown manifest keys become errors), concrete `lint_coverage` model, manifest writer rules, flag matrix, bounded first-increment detection, built-in-presets-first split. |
| [verifier-orchestration.md](verifier-orchestration.md) | Rev 2 after review — awaiting bead conversion (after landing) | Issues #11/#12/#13 follow-through. Rev 2: recorded authorization fact (not embedded boilerplate), full config schema unit, `verifier begin`+`submit` with hash bindings and evidence-on-pass, commit auto-discovery, legacy-file compatibility, report-only `verifier doctor`, NTM prompt storage deferred. |
| [bead-closure-mode.md](bead-closure-mode.md) | Rev 2 after review — awaiting bead conversion (after landing) | Issue #14. Rev 2: corrected safe order (evidence after `.beads` staging, composing with verifier begin/submit), journal-backed state machine with `--resume` and eight partial-failure fixtures, payload-based classification closing the `--admin-only` bypass, attestation-keyed drift check. |
| [open-source-launch.md](open-source-launch.md) | Rev 2 after review — deferred until feature-complete (D5, executes last) | Rev 2: real installer command (public default repo + pinned-tag fetch), Go module installer path explicitly rejected, old v0.1.0/v0.1.1 release handling, LICENSE inside release archives + dependency-license review, full-surface audit checklist with owner thresholds, concrete SECURITY.md channel. Five owner decisions listed. |
| [GOAL.md](GOAL.md) | Historical | Original readiness-kit goal file and implementation plan. It predates the `burpvalve` name and still contains older planning vocabulary. Use the root [README.md](../README.md), [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md), and [docs/release-install.md](../docs/release-install.md) for current operational documentation. |
