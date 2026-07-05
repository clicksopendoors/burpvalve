# OXP Round Retrospective - 2026-07

This retrospective records the orchestrator-experience (OXP) round after the
landing, release, lint, verifier-orchestration, bead-closure, and OXP chains
were completed. It is written as a durable reconstruction aid under decision
D11: autonomy is acceptable only when the owner can audit what happened from
docs, plans, commits, attestations, and tracker state.

Sources used:

- OXP bead close reasons from `br list -a --limit 300 --json`.
- `docs/dogfooding-findings-2026-07.md`, especially A23 and C19-C21.
- WhiteGorge/BrightOwl final-acceptance audits in Agent Mail, including the
  closure pre-audit, the final-acceptance hold, and the refreshed co-sign
  after the blocker fix.
- `docs/decisions-2026-07-02-review-round.md`, especially D5, D6, D10, D11,
  and D12.
- `plans/orchestration-goal-prompt.md`.

## Commit Ledger

The requested ledger scope is the OXP commit range from `2858846` through
`92a8e17`, plus support commits `dfda7b1` and `c466233`. Two BCM commits were
interleaved in the same git range (`e01037d` and `f2d3907`); they are not OXP
ledger rows, but they explain some gate-window contention captured in the
process findings.

| Commit | Bead or feature | Attestation | Result |
| --- | --- | --- | --- |
| `2858846` | `burpvalve-oxp-config-orchestrator-enum-kpl` | `backpressure/attestations/b1749be84827251a76ae949a7d3cb14df27b2a743834fa4b4ac2a6f414f9c921.json` | Added the orchestrator config enum schema for `defaults.init.orchestrator` and related validation. |
| `711063d` | `burpvalve-oxp-prompts-command-api-6lv` | `backpressure/attestations/765cda42017ef7921e4a58ad15152c745ddda2f0e18736c2fc549158006262aa.json` | Added the `prompts` command API and JSON contract. |
| `126a0fc` | `burpvalve-oxp-prompt-bank-content-f6l` | `backpressure/attestations/ff344b4cbec33ea1d77fab3e768f4eae322123c1d8e5f042ad18882ef5f5815d.json` | Added the canonical orchestrator prompt bank content, including verifier bootstrap material. |
| `c8d0a2d` | `burpvalve-oxp-prompt-export-divergence-9le` | `backpressure/attestations/b59a90e2f8f2c89443b2fd4a6f6ae01a8fb059df5ba39c7e55a1c436f0f77cfa.json` | Added prompt export content hashes and local divergence protection. |
| `932abfc` | `burpvalve-oxp-orchestrator-md-target-template-el4` | `backpressure/attestations/cb6acd320561b4a9a18bae16d49aefd1cc9c75f8642b893db50ccb8820aa22b8.json` | Added the explicit `ORCHESTRATOR.md` scaffold target and structural drift guard. |
| `8520bfb` | `burpvalve-oxp-burpvalve-bead-hook-forwarding-9ns` | `backpressure/attestations/07693e14c8e4d1dd4152ce14240fb9150debf991dbe52b8fd1b9b44ac55edf67.json` | Forwarded `BURPVALVE_BEAD` through hook templates with exact parse rules. |
| `1ead769` | `burpvalve-oxp-hash-staged-helper-a48` | `backpressure/attestations/d43d61d789b4fb7fe0e53cecf381af38d370324211168a04a6d32beaf149b2d2.json` | Added the read-only `hash --staged` helper for verifier payload binding. |
| `0c41470` | `burpvalve-oxp-ci-commit-audit-g8z` | `backpressure/attestations/0e6f885d26ea0ea7f6169dd788ba8aa4f47cadba57d771aa96cacfe647c43a2c.json` | Added first-pass commit-scoped attestation audit via `ci --commit`. |
| `e3464a4` | `burpvalve-oxp-hook-context-next-steps-gff` | `backpressure/attestations/f538d04254cb7974eed5c3a5472ad922bcde37f1ab9f1a009d9a4dfe3009eedc.json` | Added hook-context-aware recovery steps for evidence failures. |
| `0a6deb7` | `burpvalve-oxp-ntm-bridge-peer-pane-docs-0oq` | `backpressure/attestations/6a89af004c5b328ddd48063e39c22e1825d9172f55ff02dd2048b6a233d1e70b.json` | Documented calibrated NTM peer-pane verifier patterns. |
| `f3c4a46` | `burpvalve-oxp-option-b-install-surfaces-idu` | `backpressure/attestations/66154fa9880b4acce1a5af4fa4484e2e2235d8daa904f30efd61e6ef89966237.json` | Wired option-B `ORCHESTRATOR.md` install behavior through init, setup, repair, TUI, and robot surfaces. |
| `dc8c2ee` | `burpvalve-oxp-skill-refactor-prompt-bank-fy8` | `backpressure/attestations/213435be8d686ff2a8574002cb889dec62301169f4f9a6d68ed56c35e852568c.json` | Refactored the Burpvalve skill to cite the prompt bank with a bootstrap fallback. |
| `92a8e17` | `burpvalve-ci-commit-multicluster-feature-9hf` | `backpressure/attestations/e2332d370b939efb9f151dba4ec8b9b83277ac904c587208482cf09e95878135.json` | Fixed the final-acceptance blocker in `ci --commit` for multi-cluster commits with explicit `--feature`. |
| `dfda7b1` | `dogfooding-findings-a23-c19-c20-c21` | `backpressure/attestations/bdde462c226769f002eaf42a2818a2ea630a2c38bde8d9f80946ac03fdf67e23.json` | Updated the dogfooding findings log with A23 and C19-C21. |
| `c466233` | `final-housekeeping-tracker-clean` | `backpressure/attestations/2d5debb052ee5afd9d865fcd275a18e8ea0280b0765210a9a471a8a563917a71.json` | Synced final tracker state after OXP closure cleanup. |

## Verifier-Caught Defect 12

The final OXP acceptance check intentionally re-audited the completed chain
instead of trusting the commit ledger. That audit found product defect #12:
`ci --commit <sha> --feature <bead> --json` worked for the single-cluster
staged-hash-helper commit `1ead769`, but still failed for legitimate
multi-cluster historical commits such as `f3c4a46`, `dc8c2ee`, and the
first-pass `ci --commit` implementation `0c41470`. The error still said the
staged changes mapped to multiple diff clusters even though `--feature` had
already been supplied.

The ruling was to fix, not waive. The reason is central to the OXP contract:
the orchestrator is supposed to validate a landed commit's attestation
without depending on today's dirty working tree. A command advertised as
commit-scoped evidence audit cannot reject ordinary gated commits merely
because their payload includes tracker state, skill files, docs, or multiple
Go packages. At the same time, `--feature` must remain an assertion against
the committed attestation, not a way to launder unrelated evidence.

The fix landed as `92a8e17` under
`burpvalve-ci-commit-multicluster-feature-9hf`. The regression
test covers a committed multi-cluster fixture where matching `--feature`
passes and an incorrect feature assertion still fails. The refreshed
final-acceptance co-sign then validated:

- `go run ./cmd/burpvalve ci --commit f3c4a46 --feature burpvalve-oxp-option-b-install-surfaces-idu --json`
  passed with the `66154fa...` attestation.
- `go run ./cmd/burpvalve ci --commit dc8c2ee --feature burpvalve-oxp-skill-refactor-prompt-bank-fy8 --json`
  passed with the `213435...` attestation.
- `go run ./cmd/burpvalve ci --commit 92a8e17 --feature burpvalve-ci-commit-multicluster-feature-9hf --json`
  passed with the `e2332d...` attestation.
- The wrong-feature control still failed with a feature assertion mismatch.

This was the correct failure mode for Burpvalve. The verifier pool found a
real product defect before final acceptance was declared complete, the owner
ruling kept pressure on the artifact instead of accepting a procedural
exception, and the blocker fix itself went through the same gated path.

## Protocol Corrections Applied

Three protocol fixes became part of the round rather than merely notes for
later.

First, delivery beads close after the gated commit exists. WhiteGorge's OXP
closure pre-audit caught `burpvalve-oxp-hook-context-next-steps-gff`
closed ahead of its landed commit. That is unsafe because dependency graphs
then report completion before the durable artifact exists. The applied rule:
delivery close reasons must cite the commit hash and the attestation or a
short artifact note. If a close runs early, downstream handoff pauses until
the commit lands and the close reason is corrected.

Second, close reasons are audit evidence. The final-acceptance audit also
required `burpvalve-oxp-skill-refactor-prompt-bank-fy8` to carry
its commit and attestation reference. After correction, its close reason
points at `dc8c2ee` and
`backpressure/attestations/213435be8d686ff2a8574002cb889dec62301169f4f9a6d68ed56c35e852568c.json`.
This made final acceptance reconstructable from `br show`, not from chat.

Third, `.beads/issues.jsonl` reservations were narrowed to gate-window
bookkeeping and short tracker handoffs. C19 records that holding `.beads`
for an entire implementation bead serialized unrelated agents' closes and
claims. The active rule is to reserve code or doc paths while editing, reserve
`.beads` only while mutating or staging tracker state, then release with an
Agent Mail handoff plus terminal wake.

Related gate-window discipline also stabilized during OXP: acquire the
`GATE-WINDOW` token before staging, confirm the index is empty before adding
payload files, keep peer unstaged work out of the index, push exactly once
after a successful gated commit, and wake the next queued agent on release.

## Completion-Criteria Evaluation

`plans/orchestration-goal-prompt.md` defined six completion criteria. The OXP
round closed against those criteria as follows.

1. `land:` completion was already accepted before OXP began: all land beads
   were closed with gated, attested commits, the final tree passed
   Burpvalve CI, and previously untracked payload was either owned or ignored.

2. `rel:` completion was accepted before OXP: v0.1.2 was released,
   post-smoke passed, release assets were verified, and release hygiene closed
   the intended GitHub issues for the v0.1.2 track.

3. The non-deferred `lint:`, `vorch:`, and `bcm:` chains were closed. Deferred
   P4 beads intentionally remain deferred, including the OXP option-A truth
   table bead `burpvalve-oxp-defer-option-a-truth-table-me5`.

4. GitHub issue handling followed the boundary notes. The release and lint
   chains closed or commented the issues they owned; open-source launch and
   owner-decision work remains outside this round by D5 and the OSL start
   gate.

5. The dogfooding findings log was brought current in `dfda7b1`. A23 was
   promoted immediately into the blocker bead fixed by `92a8e17`. C19-C21
   were recorded as process/tooling findings and applied to the live protocol
   where possible.

6. `osl:` remains intact and blocked on owner decisions. That is not drift;
   D5 says public launch, license selection, badge/metadata changes, and
   outward-facing visibility work happen last, after feature completeness and
   explicit owner decisions.

## Open Remainder

The round deliberately leaves three classes of work open.

- `osl:` owner-decision beads remain blocked until the owner answers launch
  questions. This includes public visibility, license choice, launch metadata,
  and any other irreversible or outward-facing action covered by D5 and D11.
- Two local-only issue bundles are parked for owner approval before external
  filing: `docs/burpvalve-issue-drafts-2026-07.md` for Burpvalve follow-ups
  and `docs/upstream-br-feedback-2026-07.md` for upstream `br`/`bv` feedback.
- Deferred beads remain deferred by design. They preserve future work without
  making the current round incomplete.

## Retrospective Judgment

The OXP round validated D12's premise: the orchestrator is a real Burpvalve
user, and orchestrator-facing friction is product signal. The chain shipped
prompt-bank surfaces, an explicit `ORCHESTRATOR.md` scaffold target,
peer-pane verification guidance, hook metadata forwarding, staged-hash
helpers, commit-scoped CI audit, and install-surface wiring. It also proved
that acceptance gates must audit the tools used by the gate itself: the
multi-cluster `ci --commit` defect was not a paperwork issue, it was a
missing product capability.

The strongest process lesson is that autonomy depends on reconstruction
quality. Atomic commits, attestations, close reasons, narrow reservations,
and findings-log updates are not ceremony; together they let the owner veto,
rollback, or extend the work without guessing what happened.
