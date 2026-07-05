# Attestation Artifact Schema

`burpvalve commit` writes two schema-compatible artifact kinds:

- `passing`: tracked under `backpressure/attestations/<staged-payload-hash>.json` and accepted only when every feature/condition cell is independently verified.
- `blocked`: local recovery evidence under `log/backpressure/failed/<timestamp>-blocked.json` when any cell fails, is unknown, lacks a verifier, or has insufficient evidence.

The schema binds the agent claim to the current staged payload and current backpressure condition text. It is not a new backpressure condition.

Burpvalve writes these artifacts as generated evidence. Passing attestations and
blocked reports are formatted as stable, indented JSON with a trailing newline so
they can be reviewed and diffed like normal project files. If a broad project
formatter rewrites or rejects generated evidence files, prefer excluding
`backpressure/attestations/*.json` and `log/backpressure/failed/*.json` from
that formatter rather than editing the evidence by hand.

Required top-level fields:

- `schema_version`
- `tool`
- `tool_version`
- `artifact_kind`
- `staged_payload_hash`
- `manifest_hash`
- `condition_order`
- `generated_by`
- `git_head_before_commit`
- `created_at`
- `feature`
- `bead_ids` when the commit was associated with delivery beads
- `coupled_work_rationale` when multiple bead ids intentionally share one staged payload
- `atomicity`
- `conditions`

Blocked reports may also include top-level `next_steps`, a de-duplicated list of
the recovery actions from blocked condition cells.

`bead_ids` is stable query metadata written by `burpvalve commit --bead`. It is
separate from the feature id: a feature names the staged behavior, while bead ids
name the delivery tasks this commit closes. If more than one bead id is recorded,
`coupled_work_rationale` should explain why one staged payload intentionally
delivers them together.

Each condition entry records:

- `condition_id`
- `condition_file`
- `condition_file_hash`
- `verifier_policy`
- `verifier`
- `subagent_confirmed`
- `subagent_model`
- `verdict`
- `message`
- `evidence`
- `next_action`
- `supplemental_verifiers` when additional verifiers contributed non-primary evidence
- `adjudication` when an owner/ruling resolved or documented verifier disagreement
- `timestamp`

`verifier_policy` comes from `backpressure/manifest.yaml`. Supported values:

- `independent_required` (default): the cell needs a separate verifier context.
- `main_agent_allowed`: the committing agent may provide the evidence.
- `ci_allowed`: CI evidence may satisfy the cell.
- `human_allowed`: human review evidence may satisfy the cell.
- `optional`: verifier provenance may be `none`, but `unknown` is still not
  accepted in a passing artifact.

`verifier.kind` records who or what checked the cell. Supported values are
`independent_subagent`, `main_agent`, `ci`, `human`, `none`, and `unknown`.
Optional verifier fields include `agent`, `model`, `runtime`,
`separate_context`, `transcript_ref`, `evidence_ref`, and `created_at`.

Legacy response files with `subagent_confirmed: true` still map to
`verifier.kind: independent_subagent` when `verifier.kind` is omitted. New
response templates prefer the `verifier` object because it can distinguish
independent subagents, main-agent evidence, CI, and human review without asking
agents to fabricate subagent confirmation.

`supplemental_verifiers` preserves additional verifier verdicts without
rewriting the primary verifier result. Each entry records verifier provenance,
verdict, message, evidence, optional transcript reference, and next action.
Supplemental fail, unknown, or disagreement evidence is queryable and should be
held or escalated; it is not hidden behind a primary pass.

`adjudication` is audit metadata. It records `authority`, `summary`, optional
`final_verdict`, and `audit_ref`, but `final_verdict` does not override the
condition's primary `verdict`. A condition with primary `fail` or `unknown`
still blocks a passing artifact even when adjudication records
`final_verdict: pass`.

Reviewer discovery rules:

- No tracked attestation artifact means the commit failed the protocol.
- A passing artifact must satisfy the condition's `verifier_policy`.
- Under the default `independent_required` policy, `verifier.kind: main_agent`,
  `ci`, `human`, `none`, or `unknown` is not enough.
- Missing or unacceptable verifier evidence without a message means the missing
  verifier was not explained.
- `verdict: not_applicable` without a message means the exemption was not justified.
- `verdict: fail` or `verdict: unknown` in a tracked passing artifact means the commit should be rejected even if a hook was bypassed.
- A stale `staged_payload_hash`, `manifest_hash`, `condition_order`, or `condition_file_hash` invalidates the artifact.
