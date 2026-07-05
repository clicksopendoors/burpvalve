# Bead Closure Mode: Gate Delivery Work At The Closure Boundary

## Status

Planning, revision 2. Revised 2026-07-02 after two independent Codex reviews
(Agent Mail messages 2272/2273; both "needs revision" — the original
sequencing could produce stale verifier evidence, resumability was asserted
but not designed, and the drift heuristic contradicted the close-before-commit
order). Convert to beads only after this revision is accepted. Depends on
`plans/land-working-set.md` and composes with
`plans/verifier-orchestration.md` (shared safe order below).

## Source

GitHub issue #14 and its comment; decision D8 in
`docs/decisions-2026-07-02-review-round.md`.

## Problem

The commit gate fires at `git commit`, but agents *claim* completion at bead
closure. When those boundaries drift, tracker state and git state tell
different stories (as this repo itself demonstrated). The landed read-only
half exists: `beads preflight` plans delivery, attestations record
`bead_ids`. This plan adds the mutating closure flow and a drift check —
with the ordering, resumability, and contract detail the review round
demanded.

## Goals

- A documented, tool-assisted closure policy: delivery beads close inside a
  sequenced, evidence-complete flow; admin beads may batch.
- `burpvalve beads close` (resolved name): one entry point that drives the
  full sequence with an explicit state machine and safe resume.
- A drift check that flags closed-beads-plus-dirty-tree situations.
- Combined-bead closure stays possible but always explicit
  (`--bead-rationale`, preserved into the attestation via
  `PreCommitOptions.BeadRationale`).

## Non-Goals

- No absolute one-commit-per-bead law; admin/tracker housekeeping batches.
- Burpvalve does not become an issue tracker; `br` is the mutation surface.
  Burpvalve never passes `--force`, `--bypass-policy`, or any bypass flag to
  `br`, and it records the exact `br` command line and captured
  stdout/stderr in its result JSON.
- No blocking of direct `br` use. The drift check is advisory unless a
  project wires it into lint/CI; the plan must not imply enforcement.
- `burpvalve beads close` **blocks** (structured JSON, exit 2, matching
  `beads preflight` behavior) when `br` is absent — a Beads-mutating command
  cannot degrade to guidance. Core Burpvalve remains fully usable without
  Beads; only the `beads` subcommands require `br`.

## The Safe Order (shared with verifier-orchestration)

The review round found the original order collected verifier evidence before
`.beads/issues.jsonl` was staged, so the evidence never covered the payload
actually hashed and attested. The corrected order — written as numbered steps
per D4, never as an arrow chain:

1. Agent stages the delivery payload (implementation, docs, tests, config).
   `beads close` never scans for or stages dirty files; the payload staging
   is the agent's explicit act, snapshotted at preflight.
2. `beads close` runs the preflight checks (reusing the landed report code).
3. `br close <id> --reason "<reason>"` — the reason references the bead and
   feature, never a commit SHA (which cannot exist yet).
4. `br sync --flush-only`.
5. `beads close` stages exactly one additional path: `.beads/issues.jsonl`.
   The staged payload is now final.
6. Verifier evidence is collected against this final staged payload — via
   `verifier begin` + `verifier submit` (auto-discovered by the gate), or a
   caller-supplied `--responses` file bound to the current staged hash.
   Evidence gathered before step 5 is stale by construction and the gate's
   hash binding will reject it; the command says this up front.
7. `burpvalve commit --bead <id>` runs the gate. On first pass it writes the
   attestation and instructs staging it (an expected, documented
   intermediate state — see State Machine).
8. Stage the attestation; the gate revalidates.
9. `git commit` — owned by `beads close` **only** behind explicit
   confirmation (interactive default No; robots require `"confirm": true`
   plus a message). Without confirmation, the command stops after step 8
   with the exact `git commit` invocation in `next_steps`.

## State Machine And Resume (was a blocker; now the core design)

`beads close` maintains a journal at
`log/backpressure/closures/<bead-id>.json`: an ordered step list with
per-step `id`, `status` (`pending|done|failed|skipped`), the executed
command, captured output reference, and timestamps. Every run:

- Recomputes reality first (bead status via `br show --json`, staged paths,
  responses-file presence, attestation presence) and reconciles the journal
  against it — reality wins over journal claims.
- Is idempotent per step: an already-closed bead is `skipped` (with a
  warning naming who/when per `closed_at`), an already-flushed sync is
  `skipped`, an existing bound responses file is reused, an already-written
  attestation is revalidated rather than rewritten.
- On any failure, stops with `partial_success: true`, the journal updated,
  and structured `next_steps` (per-step objects with `id`, `message`, exact
  `command`, `fatal` — the setup/init/repair result-contract shape, NOT bare
  strings) including the exact resume invocation (`burpvalve beads close
  <id> --resume`).
- Named partial-failure fixtures required by acceptance: close succeeded,
  sync failed; sync succeeded, gate blocked; attestation written but
  unstaged; attestation staged but payload changed afterward; `git commit`
  failed after gate pass; bead already closed on entry; `br` missing;
  unrelated dirty files present.

"No silent middle state" is thereby defined honestly: middle states exist
(the attestation-staging bounce is normal gate behavior), but every stop
point reports exactly where it is and what to run next. There is no state
the command can leave that its own resume cannot classify.

## Classification (concrete)

Decisive signal: **the staged payload**, not bead metadata.

- **Admin path allowed** only when the staged payload consists exclusively
  of `.beads/**` paths (plus nothing). `--admin-only` with any non-`.beads`
  staged path is a hard error naming the offending paths — this closes the
  bypass the review found. Admin closures run steps 2-5 plus a tracker-only
  commit (same confirmation rules), demand no code attestation (never ask
  agents to fabricate one), and may batch multiple admin beads.
- **Delivery** is everything else. `issue_type` and `labels` (which `br`
  exposes; the landed `inspectBead` struct must be extended to decode
  `issue_type`, `labels`, `closed_at`, `close_reason`) are used for warnings
  only — e.g. type `docs` with a staged payload is delivery (user-facing
  docs are payload); type `docs` with only `.beads` staged is admin. When
  metadata and payload disagree, payload wins and the discrepancy is
  reported.

## Reuse Contract (was a gap)

`beads close` extends the landed code, it does not reimplement it:
`buildBeadsPreflightReport`, `normalizeCommitBeads` (multi-bead +
`--bead-rationale` enforcement), `inspectBead` (extended fields as above),
and `stagedPathNames` in `cmd/burpvalve/main.go`. Preflight severities must
be classified for the close context: e.g. preflight's `action_needed` on an
in-progress bead is *expected* at close time (advisory), while missing `br`
or empty staged payload on the delivery path is fatal.

## Command Surface

`burpvalve beads close <bead-id> [<bead-id>...]` with: `--root`,
`--admin-only`, `--reason` (required), `--bead-rationale` (required for
multiple beads; every bead must intend the same staged payload),
`--responses <file>`, `--feature <id>`, `--resume`, `--yes`/interactive
confirmation for the mutating steps (default No; the dry-run preview prints
every command the flow would run before asking), `--json`, and `--robots`
(structured stdin including explicit `"confirm": true`, mirroring `config
init --robots`). Robot help and `robotInputDoc` updated accordingly.

## Drift Check (redesigned around attestations, not close reasons)

`burpvalve beads drift` (advisory): correlates recently closed beads
(`br list --status closed --json`, using `closed_at` within a window,
default 7 days) against (a) attestation `bead_ids` under
`backpressure/attestations/` and git history, and (b) current tree
dirtiness. A bead closed within the window that appears in no attestation's
`bead_ids` and no commit message, while the tree is dirty, is reported as
"**possible** drift" — the wording never claims matched changes, because the
correlation is heuristic (this was review-validated: close reasons cannot
carry commit SHAs under the safe order, so attestation `bead_ids` are the
reliable linkage). Inverse check (in-progress beads whose named work appears
committed) is deferred until bead-file linkage exists. Output follows the
result contract; projects may add the command to `lint_commands` or CI to
make it enforcement (ladder: command, then CI gate).

## Documentation

`docs/beads-delivery-workflow.md` states the policy (delivery vs admin, the
numbered safe order, the batching exception, drift check usage); README's
`beads preflight` section links to it; the generated `AGENTS.md` template
gains a short "close delivery beads through the gate" clause. All numbered
steps, no arrow chains, in every generated or printed text (D4) — including
this plan and the command's own output.

## Increments

1. Policy doc + AGENTS.md clause (can land with the release round).
2. `inspectBead` field extension + classification logic + `--admin-only`
   payload guard (pure additions to landed code).
3. `beads close` single-delivery-bead path: journal, state machine, resume,
   confirmation, result-contract JSON.
4. Admin batched path + multi-bead rationale path.
5. `beads drift`, advisory.
6. (Later) CI recipe; inverse drift check if file linkage materializes.

## Acceptance Criteria

- A delivery bead closes through one command that ends in a gated, attested
  `git commit` recording its `bead_id` (with confirmation), or stops at a
  classified state with exact structured `next_steps` and a working
  `--resume`. All eight partial-failure fixtures pass.
- Verifier evidence is always evaluated against the final staged payload
  (including `.beads/issues.jsonl`); pre-final evidence is rejected by hash
  binding with a message pointing at the safe order.
- `--admin-only` with non-`.beads` staged paths errors, naming the paths.
- Admin closures never demand code attestations.
- `br` absence: structured block, exit 2, actionable guidance.
- No `br` bypass flags ever; every `br`/`git` mutation is printed before
  execution and recorded in the journal.
- `beads drift` flags the closed-beads-plus-dirty-tree fixture as possible
  drift; a properly attested closure produces no flag. (The existing
  stale-payload e2e test does NOT satisfy this; a new fixture is required.)
- All flows expose result-contract JSON with structured `next_steps`.

## Open Questions For Grilling

1. Should the journal directory (`log/backpressure/closures/`) be scaffolded
   by `init` for all repos, or created lazily on first `beads close`?
   (Recommendation: lazily; not every repo uses Beads.)
2. Multi-bead delivery closures: close all beads before the single gate run
   (current design), or gate first and close after `git commit` via a
   post-commit step? The current design keeps tracker mutation ahead of the
   commit so `.beads/issues.jsonl` is inside the attested payload — the
   alternative leaves closure metadata for a follow-up commit. (Current
   design recommended; revisit only if the stale-evidence UX proves painful.)
