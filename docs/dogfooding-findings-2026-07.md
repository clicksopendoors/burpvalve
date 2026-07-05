# Dogfooding Findings Log — 2026-07 Orchestration Round

Living log of findings from using Burpvalve, Beads, and the multi-agent
workflow on Burpvalve's own repo during the 2026-07 planning/landing round.
Each entry is a candidate for a future plan, GitHub issue, or doc fix. Status
values: `raw` (observed, not yet triaged), `issue-candidate`,
`plan-candidate`, `docs-fix`, `resolved`.

Sources: agent-mail reports from Codex agents EmeraldBrook and ScarletMarsh
(messages 2264-2277), orchestrator (RusticDog) observations, and the
2026-07-01/02 plan review round. Context: decisions in
`decisions-2026-07-02-review-round.md`, plans in `/plans/`.

## A. Burpvalve product findings

### A1. Commit choreography needs a canonical explainer command — `plan-candidate`

The gate flow (source invocation, staged-slice verification, first-pass
attestation bounce, no post-attestation staging) is cognitively dense.
EmeraldBrook had to repeat the full contract in all 24 landing beads because
there is no single citable reference. Proposal: `burpvalve explain commit`
or `burpvalve commit --explain` printing the canonical choreography
(numbered steps, per D4 no arrow chains), so beads/docs can cite one command
instead of restating the contract. (EmeraldBrook, msg 2277)

### A2. Agent-facing text must be paste-safe — validated, extend enforcement — `issue-candidate`

Three independent confirmations of D4's premise in one session:
1. Issue #16's zero-byte files match shell-redirection fallout from a bare
   `->` workflow chain (hypothesis, verification bead exists: `land: verify
   issue #16`).
2. EmeraldBrook produced a malformed bead when Markdown backticks inside a
   heredoc were interpreted by zsh.
3. ScarletMarsh hit the same backtick/heredoc hazard independently.
Beyond the landing unit 19 sanitization, consider: a lint-style check for
shell-active punctuation in Burpvalve's own generated output/templates
(prose `->` chains, unquoted backticks in agent-facing instructions).

### A3. Gate self-hosting has a bootstrap problem — documented, keep as doctrine — `docs-fix`

A repo cannot gate the commit that first tracks the gate's own rules, and an
active hook preferring a repo-local binary can silently gate with a stale
binary. Resolved for this repo via the Phase 0 bootstrap exception and the
`go run` gate contract (`plans/land-working-set.md`). Product angle worth a
future doc/feature: `burpvalve setup` could detect "hook exists but would run
a binary older than the working tree" and warn — the stale-ignored-binary
trap is generic, not specific to this repo.

### A4. Burpvalve development-loop confidence effect — positive signal — `raw`

The intended pressure-transfer effect was observed at plan level before any
gated commit: twelve structured, evidence-specific review reports let the
orchestrator accept/reject plans quickly because claims were checkable
(file:line specifics, verified against the tree). Both agents also reported
the Beads dependency model "made the plan order hard to scramble." This is
the spirit of the tool working; capture as marketing/README narrative
material for the launch plan.

### A5. Commit-gate usage still unexercised at time of logging — `raw`

As of this entry no commit has ever passed through this repo's own gate
(`backpressure/attestations/` empty until the landing sequence runs). The
real dogfooding evidence — do agents run real verifiers, does the gate catch
anything, does `--responses` friction push toward shortcuts — lands with the
`land:` bead execution. Evaluation criteria staged in advance:
1. Were verifier cells backed by real subagent runs (spot-audit transcripts)?
2. Did the gate block anything real (count blocked reports vs passes)?
3. Did any agent attempt `--no-verify`, gate bypass, or evidence reuse?
4. Reviewer-side: did attestations actually speed up commit review?
Update this entry as landing commits accumulate.

### A6. First-ever gate firing: blocked correctly, but recovery steps were hook-context dead ends — `issue-candidate` (partly resolved by landed work)

2026-07-02, first commit attempted through this repo's own gate (land-01).
The gate **worked**: correct binary fallback (`./bin/burpvalve` gone → `go
run ./cmd/burpvalve`), correct `status: deleted` classification of the staged
binary deletion, structured JSON with payload/manifest/condition hashes, a
blocked report, and a real block on two-cluster feature detection. The
implementing agent behaved exactly as intended: no `--no-verify`, no
fabrication, explicit blocker report. **The product gap:** the gate's
`next_steps` said "pass `--feature` explicitly," but the *live* hook had no
way to forward it — a recovery instruction that cannot be executed in the
context that produced the error is a dead end. Root cause was a stale local
hook predating the template fix (gh-5 env forwarding), but the general
finding stands: `next_steps` should be hook-context-aware (suggest
`BURPVALVE_FEATURE=... git commit` when running under a hook, not standalone
`burpvalve commit`). Candidate issue after landing.

### A7. Bootstrap gate failures write evidence into untracked `log/` — `raw`

During Phase 0, blocked reports land in `log/backpressure/failed/` before
`log/` is tracked — useful evidence, but one more untracked artifact during
the most fragile window. Options if this recurs elsewhere: document it, or
have `setup` warn when the evidence directories are untracked.

### A8. Independent verification paths: Codex native subagents AND peer NTM panes both work — `raw`

Resolution for A6's block used a second pane agent (separate context,
different session) as the independent verifier for the 13 condition cells,
with `verifier prompts` packets relayed over Agent Mail. Owner correction
(2026-07-02): **Codex CLI does have native subagents** — the orchestrator
initially assumed otherwise. Consequences: (a) native subagents are the
preferred default for verifier cells (cheaper, no cross-pane mail relay),
matching GOAL.md's original design and the repo contract; (b) the peer-pane
pattern remains valid and useful when extra independence is wanted (a
different top-level agent/model reviewing, not a child of the committing
agent) or when a runtime truly lacks subagents; (c) verifier-orchestration
plan's spawn profiles should treat `native` as available for Codex.
Orchestrator lesson: verify runtime capabilities before designing around
their absence. Outcome of the land-01 verification to be recorded when the
commit lands.

Owner follow-ups (2026-07-02, same thread): (1) native subagents **may not
have the tooling to use burpvalve** — sandboxed/read-limited children can't
necessarily run gate commands, execute tests for evidence, or submit
schema'd verdicts; (2) the owner's working hypothesis is that **NTM agent
patterns are the better fit for burpvalve verification** — full-tooling peer
panes as independent verifiers, coordinated via Agent Mail, exactly the
pattern used live for land-01. Implications: the verifier-orchestration
plan's `spawn_method` default should be per-runtime-capability, not
blanket-`native`; `docs/ntm-bridge.md` (which currently discourages NTM
per-cell fanout) is a revision candidate; and the land-02 commit should run
a native-subagent experiment (tooling permitting) so the comparison is
empirical rather than assumed. Status: `plan-candidate` — feeds vorch beads
before implementation starts.

### A9. Verifier packets don't let the verifier independently reproduce the payload hash — `issue-candidate`

2026-07-02, first real verifier run (land-01, 13 cells, ScarletMarsh): all
verdicts came back evidence-backed (7 pass, 5 not_applicable, 0
rubber-stamps), and the verifier attempted to independently verify the
staged payload hash — `git diff --cached --binary ... | sha256sum` did not
match Burpvalve's reported hash, because the packet does not state the hash
algorithm/canonicalization rule. The verifier correctly declined to treat
that as a failure, but the gap is real: an independent verifier cannot
currently confirm they verified the same payload the gate will attest.
Fix candidates: packet includes the exact reproduction command or a
`burpvalve hash --staged` helper; feeds the vorch prompt-contract bead
(binding hashes are already planned — add the reproduction rule).
Positive finding from the same run: the verdict-quality bar held — every
pass named the commands run and observations, every not_applicable carried a
concrete reason grounded in the condition file's text.

### A10. First attested commit landed (43a760c) — gate choreography worked end to end — `resolved` (evidence for A5)

2026-07-02: land-01 became the repo's first gated, attested commit. Observed
sequence: stale-hook block (A6) resolved by the plan's hook update; no-TTY fail-closed with
blocked report; response-backed first pass wrote the attestation and
demanded staging; final pass validated, excluded the staged attestation from
the payload hash, ran lint honestly (`not_enforced`/`scaffold-only`), and
allowed the commit. Verifier evidence was real (13 cells from a separate
agent, 8 pass / 5 not_applicable, all command-backed; orchestrator
spot-audited claims against the index). No `--no-verify`, no fabrication.
Residual friction items from the implementer's report, each an
issue-candidate: no-TTY message should also mention `BURPVALVE_RESPONSES`
for env-forwarding hooks; `verifier prompts --json` top-level uses
`feature` (singular) which naive consumers misread; a direct
`burpvalve commit --responses` validation run writes the attestation and can
blur the hook choreography (docs warning needed). Non-burpvalve friction:
Agent Mail attachments reject non-image files, forcing inline packet text.

### A11. `burpvalve ci` cannot validate a specific commit against a dirty tree — `issue-candidate`

Orchestrator audit after land-01: running `ci` in the dirty repo re-ran
feature detection on the current staged/working state and blocked on
"multiple diff clusters" — there is no `ci --commit <sha>` mode to validate
one committed attestation independent of tree state. The reviewer story
("check the attestation in seconds") needs commit-scoped validation.

### A12. Hook does not forward a bead ID, so hook-gated attestations have `bead_ids: null` — `issue-candidate`

The hook forwards `BURPVALVE_FEATURE/RESPONSES/AGENT/MODEL` but no
`BURPVALVE_BEAD`, so `--bead` can't be supplied through `git commit`.
land-01's attestation carries the bead ID only via the `feature` field
(acceptable here since feature==bead id, and by policy for the landing
sequence: set `BURPVALVE_FEATURE` to the bead ID so traceability survives).
Product fix candidate: forward `BURPVALVE_BEAD` in hook templates; feeds
issue #14 / bead-closure-mode.

### A13. Native-subagent probe results: capable but weaker provenance; peer-pane wins on audit trail — `plan-candidate` (empirical answer to A8)

2026-07-02, land-02 probe (one low-stakes cell): a native Codex subagent
COULD read the staged diff, run read-only evidence commands, and return
schema-conformant verdict JSON — the tooling concern was not disqualifying.
But: model provenance was weak (subagent self-reported `gpt-5` with no
machine-readable runtime identifier, despite a gpt-5.5 session), and there
is no durable, attributable artifact of the verification besides the main
agent's relay. Peer-pane verification produced an Agent Mail message with a
stable id, sender identity, and cc-audit — strictly better evidence. Owner
hypothesis (NTM patterns better for burpvalve) is supported for
evidence-quality reasons, not capability reasons. Feeds vorch spawn-profile
beads: `native` is viable for low-stakes cells; provenance-heavy cells
favor peer-pane; the transcript_ref work (vorch-05) would narrow the gap.

### C4b. Pairwise contact approvals stall packet relays, not just orchestrator reports — `docs-fix`

land-02: EmeraldBrook→BrightOwl contact was pending even though
BrightOwl→RusticDog was approved; the verifier packet send stalled until
BrightOwl approved live. Extension of C4: at session start (or when adding
agents), pre-approve the full expected communication mesh
(implementer↔verifiers, everyone→orchestrator), not just orchestrator
contacts.

### A14. Verdict-semantics rule: pass vs not_applicable — codified after first real verifier disagreement — `resolved into beads`

2026-07-02, land-03: the two-verifier pool disagreed on security-boundaries
(BrightOwl `pass`, WhiteGorge `not_applicable`) with fully aligned factual
evidence — the hold-and-escalate protocol fired exactly as designed, and the
implementer refused to pick the convenient verdict. Orchestrator ruling,
now binding: `not_applicable` = the payload contains no surface the
condition governs; `pass` = the payload contains a governed surface that
was inspected and found satisfied, even when the finding is "nothing
dangerous present." The distinction is whether there was something to
check, not whether problems were found. (land-03 stages an executable hook
= a governed security surface; land-02's Markdown+symlink = none.) This
rule belongs in condition templates and verifier packet success criteria
(vorch-03) so future verifiers never have to guess.

### A14b. Verdict-semantics refinement: reference-to ≠ governed; diligence scans support NA evidence — `resolved` (rule; vorch-03 update pending)

2026-07-02, third security-boundaries classification dispute (land-05a,
staging-map-only payload). Ruling refined A14: a governed surface means the
payload itself carries regulated content/behavior; a document merely
referring to security-adjacent areas is not governed, and
possibility-of-defect (any file could contain secrets) is not a governed
surface — otherwise `not_applicable` collapses. Diligence scans (e.g.
credential scan of a text payload) are correct practice but belong as
evidence supporting the NA claim, not grounds for `pass`. Precedent note:
land-04's committed security `pass` on docs-only stands as pre-refinement
history. Tiering datum: the medium-effort verifier made the correct call —
classification quality tracked rule clarity, not reasoning depth (supports
C13). Refinement flows into vorch-03 packet text at WhiteGorge's next touch
(assigned via ruling message 2349).

### A15. Verifier packets omit staged paths under generated-path prefixes — `resolved into beads`

land-03 (caught by the implementer's native probe): the verifier packet
listed 18 of 20 staged paths, omitting `backpressure/attestations/README.md`
and `log/backpressure/failed/README.md` — exactly the payload-hash
generated-path exclusion prefixes leaking into the packet's path list.
Two distinct problems: (1) verifiers must see ALL staged paths even when
some are hash-excluded — the packet should show both lists labeled; (2)
prefix-based exclusion also removes legitimate scaffold READMEs under those
directories from the payload hash itself, a small integrity gap for
commits that intentionally track those directories. Mitigation in use:
implementer includes true `git diff --cached --name-status` in packets.

### A16. Payload-ownership accounting wants a mechanical helper — `plan-candidate`

File-level payload accounting strains when beads intentionally split one
file by hunk/function (main.go, commit_test.go, artifacts.go...). The QA
agent reconstructs ownership from bead prose every audit. Proposal
(ScarletMarsh): `burpvalve account payload` / `explain staged-owner` —
surface shared-path ownership and generated-evidence exceptions
mechanically. Candidate for the orchestrator-experience evidence surfaces
or a later round.

### A17. Attestation schema carries one verifier per condition — dual-verification and rulings can't be encoded — `resolved into beads` (feeds vorch)

land-03 used two verifiers on shared cells plus an orchestrator ruling on a
disagreement; the response/attestation schema could only record BrightOwl's
final cell, so WhiteGorge's corroborating evidence and the ruling exist
only in Agent Mail, not the committed artifact. Product fix candidate: a
supplemental-verifiers array and an optional ruling/adjudication slot per
condition, so multi-verifier evidence and disagreement resolution stay
inside the auditable artifact. Belongs in the vorch schema work (vorch-01/
03/05 territory) — flag to the polish agent at next vorch touch. Related:
future packets should embed the A14 verdict-semantics rule text directly
(done via vorch-03 update) so classification stays consistent without
orchestrator rulings.

### A18. The gate failed the orchestrator's own document — the system disciplining its operator — `plan-candidate` (best evidence yet for A5)

2026-07-02, land-04: a verifier (BrightOwl) returned `fail` on
anti-reward-hacking because the staged decision document — authored by the
orchestrator — contained a bare arrow chain in D10 while D4 of the same
document defines such chains as defects. The commit was held, the
orchestrator ruled the fail binding against its own work, fixed the docs, and
verification was refreshed against the new payload hash. Notable: (1) the
backpressure applied upward — the tool caught the agent who assigns the
work, not just the agents who receive it, validating D12's "orchestrator is
a user" framing; (2) the two verifiers split exactly along scope
interpretation (whole-payload self-consistency vs shortcut-authorization
language), which sharpens the packet success-criteria text for
anti-reward-hacking cells: the cell covers the staged payload's consistency
with its own claims; (3) second use of the disagreement protocol, second
clean resolution — hold, escalate, rule, refresh, proceed.

### A19. Verifier caught a live code defect pre-commit — the gate's first implementation catch — `raw` (strongest A5 evidence class)

2026-07-02, land-09: during judgment-cell verification, the high-effort
verifier (BrightOwl) found a binding implementation issue in the staged
lint code — unsupported per-command manifest keys were silently ignored,
the precise silent-ignore trap the issue-17 plan review had identified as
the worst lint failure mode. The implementer patched pre-commit
(`validateLintCommandShape` default-branch rejection; table-driven tests
including `run_directory` → "not supported" until Unit A lands) and ran
the established delta re-check protocol with the second verifier, fully
self-managed. Significance: previous gate catches were docs/consistency
(A18) and process (A6); this is the first *code* defect stopped by
independent verification before commit — and it materially pre-hardens the
codebase for the lint-plan's Unit A. The catch occurred on a
judgment-adjacent cell reviewed at high effort, consistent with C13's
allocation logic.

### A23. `ci --commit` still cannot audit multi-cluster historical commits — `issue-candidate`

2026-07-02, OXP closure pre-audit (WhiteGorge, Agent Mail 2942):
`go run ./cmd/burpvalve ci --commit <sha> --feature <bead> --json` passed
for the single-cluster staged-hash helper commit (`1ead769`) but blocked
for older multi-cluster OXP commits and for the new `ci --commit` commit
itself (`0c41470`) with "staged changes map to multiple diff clusters ...
split commit or pass --feature explicitly." The command had already been
given `--feature`; under the new contract that flag is assertion-only, so
it does not select a feature cluster for historical multi-cluster commits.
Net effect: `ci --commit` is useful for some commits, but not yet the
universal reviewer-speed path the OXP plan wanted for post-hoc attestation
audit.

Why it matters: the reviewer/orchestrator story is "validate this landed
commit's attestation independent of today's dirty tree." If historical
gated commits are multi-cluster because they legitimately include `.beads`,
hooks, templates, or multiple Go packages, `ci --commit` should still be
able to validate the artifact-bound evidence without re-running feature
clustering as a blocker. Follow-up shape: make commit-scoped CI derive the
audit feature set from the committed attestation first, then treat
`--feature` solely as an assertion against that artifact. Add regression
tests using a historical-style multi-cluster fixture with `.beads` plus
code paths and confirm `ci --commit <sha> --feature <bead>` returns
`passed` with artifact/provenance output.

### A24. Split-blocker escalation worked when the merge path was pre-authorized — `resolved` (process evidence)

2026-07-03, COS route-aware scaffold (EmeraldBrook, Agent Mail 3262;
ruling follow-up 3267; verifier packets 3289/3290; completion 3293):
the implementer found that the planned Unit 3/Unit 4 split broke the
existing CLI repair adoption test. Instead of widening the staged payload
unilaterally, the implementer held, cited the plan's explicit merge
escape hatch, named concrete options, and waited for the ruling. After the
ruling, the same thread carried refreshed verifier packets and the final
commit report.

Why it matters: pre-authorized split handling turns an implementation
blocker into a governed decision instead of an ad hoc scope expansion. The
useful pattern is not "merge when convenient"; it is "hold, cite the plan's
merge condition, state the smallest options, obtain the ruling, refresh
verification against the actual staged payload, then proceed." How to
apply: future bead plans that may cross public API/test boundaries should
name the exact merge escape hatch and the evidence required before an
implementer can combine adjacent units.

### A25. Focused test evidence missed a cross-package prompt-list regression — `issue-candidate`

2026-07-03/04, proactive audit after the `7b3f761` gate escape: a
temp-worktree sweep ran `go test ./...` across `39b5ada`, `e79595d`,
`7b3f761`, and `cd1b042`. `39b5ada` and `e79595d` were green. `7b3f761`
was the sole introducing escape: it added the `gate-operator-brief` prompt
and updated focused prompt/scaffold tests, but the full suite failed in
`cmd/burpvalve` because `TestPromptsListJSONAndHumanOutput` still expected
the old prompt list. `cd1b042` also tested red, but only because it inherited
the already-red baseline from `7b3f761`; its docs-only attestation did not
claim code tests.

Why it matters: the valve did not fail because evidence was fabricated. The
attestations matched the focused suites the verifiers actually ran. The gap
was that the test-evidence condition had no policy-defined minimum suite for
prompt-bank changes, so focused `internal/backpressure` and
`internal/scaffold` checks missed a downstream CLI contract test.

How to apply: round 3 needs two hardening changes. First, define minimum test
suites by evidence surface for the test-evidence condition; for example,
prompt bank or exported prompt changes must include the relevant
`cmd/burpvalve` prompt command tests as well as internal prompt tests. Second,
add a red-baseline detector to the gate: refuse to land on an already-red tree
unless the work unit explicitly targets the failing baseline and its evidence
proves the failure is fixed or intentionally quarantined.

## B. beads_rust (`br`) / bv findings — upstream feedback candidates

These concern Dicklesworthstone's `br`, not Burpvalve itself; they matter to
Burpvalve because `burpvalve init` scaffolds Beads and Burpvalve's docs teach
agents to use it. Candidate actions: file upstream issues and/or add hints to
Burpvalve's generated AGENTS templates.

### B1. `br list` defaults to `--limit 50` silently — `docs-fix` (confirmed twice)

ScarletMarsh's audit initially saw "missing" beads; the orchestrator's QA
hit the identical trap minutes later (counted 18+9 beads when 24+11
existed). Silent truncation during an audit reads as data loss. Fix in our
own docs/templates: always `br list -a --limit 200 --json` for audits.
Upstream suggestion: print a truncation notice on stderr when the limit
binds.

### B2. `br list --json` envelope shape — `docs-fix`

Returns `{"issues": [...]}` envelope, not a bare array; `br show --json`
shape differs again. Brittle `jq` breaks. Our agent docs should show the
actual shapes; upstream could document them.

### B3. Label syntax rejects periods — `docs-fix`

`v0.1.2` is an invalid label; error surfaced late in the workflow. Hint for
generated docs: use `v0-1-2` style. (ScarletMarsh)

### B4. `br sync --flush-only` says "Nothing to export" while JSONL is modified — `docs-fix` / upstream

Auto-flush during creation had already exported; the message reads as a
no-op while `git status` shows changes. Confusing for agents wiring
sync-then-commit sequences. Upstream wording suggestion: "already exported;
JSONL may differ from git HEAD."

### B5. Description authoring through the shell is hazardous — `issue-candidate` (upstream)

Large Markdown descriptions passed via shell args/heredocs risk both
corruption and accidental execution. Feature request for `br`:
`--description-file <path>` (or `@path` support) on create/update. Both
agents independently wanted this. Interim guidance in our templates: write
description to a temp file and use whatever safe input path `br` offers.

### B7. `br update --description` is all-or-nothing — `issue-candidate` (upstream)

Review-polish edits to long self-documenting beads require replacing the
entire description block, which is risky for small fixes. Patch-style edits
or comment-based amendments would make polish passes safer. Related agent
hazards observed while authoring descriptions: zsh's read-only `$status`
variable broke a generated script; `br create -s deferred` worked well for
deferral beads; dense graphs make `br dep tree` hard to read (a
transitive-reduction/compact mode would help). (ScarletMarsh, EmeraldBrook)

### B6. Legacy bead ID prefixes persist — expected behavior, note it — `raw`

New beads in this repo originally used a legacy pre-Burpvalve ID prefix
(prefix is configurable history, not a bug — see beads-workflow migration notes).
Decision D-Phase-1 (plan 1) already treats historical IDs as immutable
history. No action beyond awareness; renaming would falsify records.

## C. Process/orchestration findings

### C1. Two independent reviewers per plan is the right default — `raw`

All six plans came back "needs revision" with substantially overlapping
blockers found independently — high-confidence signal, low duplication
waste. The overlap rate (roughly 70% of blockers found by both) suggests two
reviewers is near the sweet spot for plan review; a third would mostly
duplicate.

### C2. Review prompts that point at specific files outperform generic ones — `raw`

The strongest findings came from prompts that named exact files to cross-check
("verify against internal/lintconfig/lintconfig.go", "check what install.sh
does unauthenticated"). Generic "find gaps" framing produced weaker output in
early messages. Template the review-dispatch prompt accordingly for future
rounds.

### C3. Plans that promise schema/config fields must cite current parser behavior — `raw`

Three of six plans initially depended on fields that would be *silently
ignored* (manifest unknown-key tolerance) or *hard-rejected*
(`DisallowUnknownFields` in config). Rule for future plans: any plan
introducing a config/manifest field must quote the loader's current
unknown-key behavior and include the schema unit explicitly.

### C4. Agent Mail contact-approval gating stalls first-contact workflows — `docs-fix`

First review round: both agents' reports were blocked pending contact
approval to the orchestrator; work sat finished but undelivered until the
orchestrator polled pane state, approved contacts, and requested resends.
Mitigation for future sessions: orchestrator pre-approves expected senders
(or uses a contact-policy macro) immediately after registering.

### C5. Stale-scrollback false positives in pane monitoring — `docs-fix`

A monitor grepping pane tails for "Message id:" matched old scrollback and
fired early. Reliable idle detection: require the absence of the runtime
`Working (` marker across consecutive polls, then verify via the actual
inbox. Codify in future monitor scripts.

### C6. Beads DB sharing between two agents worked — `raw`

With brief-retry instructions on lock errors, two Codex agents created 35+
beads concurrently against one SQLite DB without corruption or deadlock.
Keep the retry guidance in dispatch templates; no further mitigation needed
at this swarm size.

## D. Improvement backlog (mined from A-C, for future planning rounds)

| Candidate | Source | Suggested vehicle |
|-----------|--------|-------------------|
| `burpvalve explain commit` choreography explainer | A1 | New GitHub issue after v0.1.2 |
| Paste-safety lint for generated agent-facing text | A2 | Extend issue #16 or new issue |
| `setup` warning: hook would run stale repo-local binary | A3 | New GitHub issue |
| README/launch narrative: plan-level pressure story | A4 | `plans/open-source-launch.md` README pass |
| Gate-usage evaluation report after landing | A5 | Update this doc; feeds future issues |
| Native-vs-peer verifier evidence and spawn-profile guidance | A13 | `vorch:` spawn/profile work and `oxp:` NTM peer-pane guidance |
| Verdict-semantics rule in verifier packet success criteria | A14 | `vorch-03` bead update — done |
| Full staged-path packet accounting and generated-prefix hash fix | A15 | `vorch-03` update plus A15 generated-path exclusion bead — done |
| Payload ownership/accounting helper | A16 | Future `burpvalve account payload` / `explain staged-owner` issue |
| Supplemental verifiers plus adjudication slot in attestations | A17 | `vorch:` schema/prompt/submit beads — done |
| Gate-disciplined-orchestrator narrative | A18 | Launch README/material pass |
| Commit-scoped CI for multi-cluster historical commits | A23 | New GitHub issue or `ci --commit` follow-up bead |
| AGENTS template: br audit hints (limit, envelope, labels) | B1-B3 | Landing unit 18 follow-up or small issue |
| Upstream `br` feedback bundle (B1, B4, B5, B7 and fresh br friction) | B | Upstream draft — drafted |
| Review-dispatch prompt template | C2 | `docs/` runbook when next round starts |
| Agent Mail pre-approval step in orchestrator startup | C4 | NTM/session runbook note |
| Persistent orchestrator transition monitor | C11 | `oxp:` prompt bank / orchestrator runbook item |
| Mail-plus-terminal-wake handoff protocol | C12 | `oxp:` `verifier-packet-relay` and `verifier-bootstrap` prompts |
| Short `.beads` reservation windows for tracker handoffs | C19 | Prompt-bank / implementer protocol update |
| Delivery bead close invariant: commit hash required | C20 | Final-acceptance checklist and dispatch prompt update |
| Owned-path patch backups for long-lived unstaged slices | C21 | Process rule, later Agent Mail/Burpvalve helper candidate |
| Assignment claim verification and unclaimed-bead detector | C29 | Orchestrator protocol and persistent monitor rule |

### C7. Barrier monitors idle finished agents — use per-pane monitors — `resolved`

2026-07-02: the orchestrator's pane monitor required BOTH codex panes idle
before firing, so when EmeraldBrook finished the vorch conversion first, it
sat idle and unassigned until the human noticed. Same class of waste as a
`parallel()` barrier where a `pipeline()` belongs. Fix applied: one monitor
per pane, each triggering that pane's QA-and-reassign step independently.
Rule for future sessions: monitor per work-unit, never per-fleet, whenever
completion times can diverge.

### C8. Agent finished work but never sent its report — verify handoffs against artifacts — `raw`

2026-07-02: 13 `lint:` beads existed in the DB while ScarletMarsh's report
mail never arrived and the pane sat idle at the prompt (classic
handoff-failure pattern: work real, handoff missing). Detection that worked:
grounding in the artifact store (`br list` by title prefix) rather than
waiting on the mail. Rule: when a pane goes idle without its expected
report, check the artifact (DB/git/files) before assuming the work was not
done — then request the report, don't redo the work.

### C10. `ntm add` can renumber panes — verify identity before every send — `resolved`

2026-07-02: after `ntm add burpvalve --cod=1`, the new agent landed at pane
4 and the existing agent (ScarletMarsh) shifted to pane 5. The orchestrator
sent the new-agent role brief to "pane 5," which delivered it to
ScarletMarsh — who plausibly accepted it because she was already doing
verifier work, masking the misdelivery. The user caught it. This is
vibing-with-ntm OC-045/AP-60 in the wild. Fix applied: before pane-targeted
sends after any topology change, verify identity via `ntm --robot-snapshot`
signals (`output_tail_lines` distinguishes a fresh pane from a
long-history pane) rather than assuming index stability. Better long-term:
target by pane ID where surfaces support it, not index.

### C11. Orchestrator wake-up reliability: one-shot monitors go blind — use one persistent transition monitor — `resolved`

2026-07-02, third occurrence of the idle-agent class (C7, C8, now this):
the consolidated monitor marked each pane "done" after its first idle event
and stopped watching it, so a pane that finished its NEXT task sat
unassigned until the owner noticed. One-shot and expiring monitors
structurally cannot serve a continuous no-idle-agents contract. Fix
applied: a single PERSISTENT monitor emitting an event on every
working-to-idle transition of any codex pane, for the life of the session.
Rule: the orchestrator's wake-up source must be as continuous as the
obligation it serves. (Also: a short pane tail can misread "working" as
"idle" — pane 2 appeared idle at 3-line depth while actively mid-task;
transition detection uses the runtime `Working (` marker, not prompt-line
presence.)

### C12. Agent Mail does not wake panes — every mail handoff needs a terminal nudge — `resolved` (protocol) / `plan-candidate` (product)

2026-07-02, land-04: the implementer mailed verifier packets and even a
reminder mail; both verifier panes sat idle because **mail delivery does
not wake a codex pane** — messages wait in the inbox until the pane gets
terminal input. Mail-only handoffs between panes therefore deadlock
silently (the sender waits on replies; the receivers never look).
Protocol fix applied: after mailing a packet, the sender immediately runs
`ntm --robot-send` to the recipient pane with a "check your inbox" wake
(verify pane indices via snapshot first — they shift). Product angle for
the orchestrator-experience prompt bank: the `verifier-packet-relay` and
`verifier-bootstrap` prompts must bake in the mail+wake pairing; NTM-side,
`pending_mail` in the snapshot could drive an automatic wake, an upstream
suggestion. This finding also explains part of the peer-pane pattern's
latency cost noted in A13 — some of it was silent deadlock, not work time.

### C13. Verifier effort tiering: reasoning depth follows cell type, not role — `resolved` (operational policy)

2026-07-02 latency analysis: the serial implementer critical path (xhigh,
10-20 min per gated commit) dominates round latency; verification runs 2-6
min in parallel and is not the bottleneck — but verifier effort burns the
SHARED account window that the implementer needs. All observed verifier
value-adds (arrow-chain catch, both disagreements) occurred on
judgment-heavy cells (anti-reward-hacking, security-boundaries,
scope-control, dry, definition-of-done); mechanical cells (payload-scope
lookups, clearly-not-governed conditions) never needed depth. Policy
applied: packets split by CELL TYPE — judgment cells to a high-effort
verifier (dual-verified), mechanical cells to a medium-effort verifier,
with an explicit escalation verdict ("this cell needs the high verifier")
available to the medium tier. The condition pre-analysis
(log/landing-condition-preanalysis.md) is what makes the mechanical tier
safe to downgrade. Feeds vorch `condition_models` design: effort/model
tiering per condition was already in the config schema — this is its
empirical justification.

### C19. Reserving `.beads/issues.jsonl` for a full bead duration serializes peer closures — `docs-fix` / `process-fix`

2026-07-02, OXP parallel work: agents repeatedly needed brief tracker
handoffs for bead close/claim while another implementer held
`.beads/issues.jsonl` for the whole implementation duration. This turned a
short bookkeeping resource into a long-running cross-chain serialization
point. The worst case was visible around OXP handoffs: a peer could have
code ready, but had to wait for a stale or broad `.beads` reservation before
closing or claiming unrelated work.

Why it matters: `.beads/issues.jsonl` is shared global state. Holding it
while editing code does not protect code correctness, but it does block
unrelated agents from doing the small Beads operations that unblock their
own work. The real contention point is the gate/window moment when tracker
state is staged or committed, not the whole bead's edit/test period.
Protocol update candidate: reserve `.beads/issues.jsonl` only for
gate-window bookkeeping or a deliberately brief close/claim window, then
release immediately. For normal implementation, reserve the code/doc paths
only. If a bead needs tracker edits early, perform them in a short explicit
handoff and announce release via Agent Mail plus terminal wake.

### C20. Closed-before-commit bead violation caught by closure audit — `process-fix`

2026-07-02, OXP closure pre-audit (WhiteGorge, Agent Mail 2942):
the OXP hook-context-next-steps bead was marked closed
with a generic close reason, while `git log` showed no corresponding landed
hook-context commit after `0c41470`, and the hook-context
payload/reservations were still live. The audit caught the protocol
violation before final acceptance: bead closure had run ahead of the landed
commit and commit reference.

Why it matters: Beads are the execution graph, but commits are the durable
shipped artifact. If a bead closes before the gated commit lands,
downstream beads can become ready on false evidence, final acceptance can
under-count remaining work, and reservation/GATE-WINDOW state becomes hard
to reason about. This is the same family of handoff-truth problem as C8,
but more dangerous because the graph itself reports completion.
Protocol update candidate: close a delivery bead only after the gated
commit succeeds and is pushed when required; the close reason must include
the commit hash and a short artifact note. For accidental early closure,
reopen immediately or block downstream handoff until the commit lands and
the close reason is corrected. Add a final-acceptance checklist item:
`br show` must show closed delivery beads with commit refs in
`close_reason`.

### C21. Peer stash churn can erase implementer worktree slices despite reservations — `process-fix` / `tooling-candidate`

2026-07-02, parallel-chain endgame: an implementer's unstaged slice was
lost/reverted during peer stash or worktree churn despite reservations
(observed on ScarletOwl's docs slice). Reservations signaled ownership, but
they did not create a durable copy of unstaged work. In a dirty shared
worktree with frequent gate-window handoffs, unstaged slices are still
vulnerable to accidental stash/apply/reset-style recovery by another pane.

Why it matters: Agent Mail reservations prevent intentional collision, not
filesystem history loss. Unstaged work has no durable identity; once another
operation rewrites the worktree, recovery depends on shell history, editor
state, or luck. This extends C12's mail-plus-wake lesson from communication
durability to worktree durability: a handoff protocol has to preserve both
the message and the bytes. Process rule candidate for long-lived unstaged
slices: before yielding the GATE-WINDOW or waiting behind another agent,
save a patch-file backup outside the commit path and mention that path in
Agent Mail. Refresh the patch after material edits. Longer-term tooling
candidate: a Burpvalve or Agent Mail helper that snapshots owned-path diffs
with reservation metadata and can report stale/unbacked slices.

### C22. Agent Mail identity recovery needs a canonical-identity rule — `process-fix`

2026-07-03, COS identity recovery (EmeraldBrook, Agent Mail 3133; follow-up
3136; refreshed packets 3151/3152): the agent recovered the original
`EmeraldBrook` registration token, declared `EmeraldBrook` the single
identity for COS work, identified `TealOwl` as a temporary recovery
identity, and released abandoned reservations 1903/1904/1905. The concrete
recovery event was token recovery plus explicit canonical identity plus
stale reservation cleanup.

Why it matters: split identities make reservations, verifier packets,
commit authorship, and audit trails look like multiple actors when the work
is actually one chain. The durable rule is: after identity recovery, one
canonical Agent Mail identity owns all continuing work; temporary recovery
identities must be named, retired, and prevented from retaining
reservations. How to apply: recovery mail should include the canonical
agent name, the retired temporary identity, released reservation IDs, and
which future artifacts will use the canonical identity.

### C23. Gate-release wakes must target every known unblocked peer — `process-fix`

2026-07-03, queue-stall and release-handoff review: several handoffs
showed that an Agent Mail release notice is not enough by itself when
multiple peers are blocked on the same GATE-WINDOW or shared file release.
The correction is a process recommendation from those observed queue
stalls, not a measured pane-count claim: a release has to wake every known
peer unblocked by that release, not only the next window holder.

Why it matters: Agent Mail is durable audit state, but it is not a reliable
attention signal for active terminal panes. A release can unblock a window
holder, a file-reservation waiter, and a parked verifier or implementer at
the same time. How to apply: before release, take a fresh `ntm
--robot-snapshot`; send the Agent Mail release notice; then terminal-wake
the specific panes whose work is unblocked by the released files, staged
index, or GATE-WINDOW. Do not broad-broadcast unless the release truly
unblocks the whole fleet.

### C24. Four parallel tracks can saturate the single shared index — `planning-candidate`

2026-07-03, open-source-launch endgame: four implementation tracks
repeatedly produced verified, ready, staged, or parked slices faster than
the single shared index/GATE-WINDOW could publish them. This is a
qualitative throughput finding, not a measured ratio: the recurring symptom
was implementers waiting behind unrelated staged payloads even when their
own work was complete and non-overlapping.

Why it matters: a single index is a correctness boundary, but it is also
the throughput ceiling for a fleet. The more parallel the implementation
graph becomes, the more queue discipline, staged-slice verification, and
release fan-out determine progress. How to apply: for future rounds,
consider explicit lane policies for docs-only or tracker-only slices,
batched low-risk docs gates when the owner authorizes them, or a second
window policy with a mechanical proof that it cannot mix staged payloads.
Until such a policy exists, keep the single-window rule and make the queue
visible in Agent Mail.

### C25. Broad stash/reset handling over peer dirt caused clobbers — `process-fix`

2026-07-03, prompt-bank/C2/COS overlap (LilacGlacier, Agent Mail 3343;
EmeraldBrook 3344; correction acknowledgements 3345/3346/3349/3350/3351):
broad `git stash push --keep-index --include-untracked` handling during a
gate/regate swept peer unstaged work. LilacGlacier reported C2 was swept
twice and had to be reconstructed from tool-log patch content; EmeraldBrook
reported COS guidance files were wiped and restored. The root cause was
moving shared peer dirt during gate choreography, not merely the absence of
patch backups.

Why it matters: reservations communicate ownership, but they do not protect
bytes from a stash/reset/checkout operation run by another pane. In a
shared worktree, peer unstaged work is not part of the current staged
payload and is not yours to move. How to apply: never use `git stash`,
`git reset`, or `git checkout` over peer unstaged work as part of gate
sequence. If peer dirt is present, coordinate for the owner to park it or
run staged-slice verification in a temporary worktree. Before parking any
implemented-but-unstaged slice, write a patch backup outside the repo path
and mention it in the park announcement.

### C26. Fleet-idle diagnosis needed a three-layer fix — `process-fix`

2026-07-03, throughput recovery after queue stalls and idle panes:
owner-ordered diagnosis split the fleet-idle problem into three causes:
pane-idle is a lagging signal without a HEAD-advance monitor,
release-wakes were being treated as information rather than authorization,
and verification was still happening while the GATE-WINDOW was held. The
matching three-layer fix is an orchestrator HEAD-advance monitor with a
20-second poll, a standing self-promotion doctrine where a release wake is
authorization and a silent predecessor means the next agent takes the
window after a step-zero check, and verify-early/gate-fast: send
hash-bound packets at park time so the window hold is mechanics only.

Why it matters: queue state is not the same as pane state. A pane can look
quiet because it is waiting for a wake, because a predecessor released only
by mail, or because verification is consuming the only shared index lease.
How to apply: monitor HEAD advancement as the authoritative signal that
the queue changed; treat release wakes as permission to acquire the next
window after the index-empty step-zero check; and send verifier packets as
soon as a parked slice has a stable hash instead of waiting for the gate
turn.

### C27. Post-fix latency ranking points to a single gate runner — `planning-candidate`

2026-07-03, post-fix throughput review: after verify-early/gate-fast, the
remaining dominant cost is per-commit ceremony executed as LLM turns,
observed at roughly 20-30 turns per commit. Repeated full test runs are
the second-ranked cost. Product direction: a single `gate run` invocation
that collapses the mechanical sequence is a round-3 headline candidate.

Why it matters: once verification is hash-bound and moved before the
window, the gate bottleneck is less about human or verifier judgment and
more about repetitive ceremony: index check, stage, hash check, gate,
commit, push, close, sync, release, and wake. How to apply: specify a
Burpvalve command that takes a prepared hash-bound handoff and executes
the mechanical sequence fail-closed, with explicit stop points for hash
mismatch, verifier disagreement, dirty index, or test failure.

### D14. Spark gate-operator pilot tests low-tier mechanics behind the valve — `raw`

2026-07-03, owner-approved gate-operator pilot: a dedicated pane, Spark
(pane 4, `gpt-5.3-codex-spark` low), executes gate ceremony from prepared
hash-bound handoffs. The product thesis is that a fail-closed,
hash-bound valve can make cheap-fast models safe for mechanics:
model-tiering behind a distrusting gate. The measurement is gate wall-time
against the observed roughly 25-minute `5.5-medium` baseline. The rule is
escalate, do not judge: Spark can execute ceremony and report blockers,
but not make judgment calls.

Why it matters: if the gate can distrust its operator, expensive model
capacity can stay on implementation and judgment cells while a cheaper
operator handles repeatable mechanics. How to apply: keep Spark's inputs
fully prepared and hash-bound; require exact hash match before staging;
stop on any ambiguity; and compare completed gate runs against the
baseline without letting Spark waive or reinterpret verifier results.

### C28. `ntm add` renumbering recurred and shifted pane-addressed sends — `process-fix`

2026-07-03, fleet expansion and pane-map correction: inserting a new pane
mid-sequence shifted six pane indices, caused one misdelivered assignment,
and created a near-miss `/model` change on a verifier. This is the same
class as C10, but it recurred under a larger active queue with more
pane-addressed sends.

Why it matters: pane numbers are operational addresses, not identities.
After `ntm add`, stale pane maps can send work, wakes, or model changes to
the wrong agent. How to apply: after any `ntm add`, re-verify the full
pane map from process/banner evidence before any pane-addressed send.
Update queue notes and wake targets immediately, and include the corrected
map in the next coordination message.

### C29. Assignment without claim is a silent handoff failure — `process-fix`

2026-07-03, OSL open work: the orchestrator assigned two beads via terminal
wake, but both agents absorbed the wake without claiming the beads. The beads
remained in the ready queue for roughly 30-60 minutes. The orchestrator's
verification checked pane activity, but pane activity is not claim evidence:
an active pane proves the runtime is doing something, not that the assignment
registered or that the correct bead moved to `in_progress`.

Why it matters: wake messages compete with in-flight context. An agent can
finish its current thought, lose the assignment, and still look alive in pane
monitoring. Pane liveness can therefore mask a dropped handoff. This is the
same root class as C23: acknowledgment-free handoffs fail silently when the
system treats an attention signal as evidence of state change.

How to apply: an assignment is complete only when the bead shows
`in_progress` with the right assignee; verify Beads state within one
orchestrator tick of assigning. The orchestrator now runs an unclaimed-bead
detector as a third persistent monitor, alerting when a bead remains ready
for roughly 12+ minutes. Assignees must claim first, before reading the bead
or planning the work: claim-then-read, never read-then-claim.

### C30. Tracker mutations can be parked, but are not landed until gated — `process-fix`

2026-07-03, D-OSL/D-LEX parked tracker slice (WhiteGorge, Agent Mail 3375,
3382, 3386): tracker wiring was prepared worktree-side, including closing
the OSL audit-history-secrets bead, adding follow-up
beads, and syncing `.beads/issues.jsonl`. The later gate attempt stopped
because the index contained another agent's staged payload, and the slice
remained parked with a patch backup. The process signal is distinct from
C20: tracker changes can be prepared before the gate, but they must not be
treated as landed state until the `.beads` delta is staged, verified,
gated, committed, and pushed.

Why it matters: Beads state is both planning data and committed audit data.
If a local DB close/update is described as done before its JSONL delta
lands, downstream agents can make decisions from state that is not yet in
the shared history. How to apply: park tracker mutations explicitly as
"worktree-side only" or "DB-local only"; include the patch backup and the
exact files; avoid claiming closure as durable until the gated commit
contains `.beads/issues.jsonl` and the close reason cites the landed commit.

## Log discipline

Append new findings with an ID (next: A26, B8, C31, D15), date, source, and status.
When a finding is promoted to an issue/plan, update its status and link the
artifact rather than deleting the entry.
