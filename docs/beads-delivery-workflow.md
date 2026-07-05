# Beads Delivery Workflow

This policy covers Beads-backed work in Burpvalve repositories. It separates
delivery work from tracker-only administration so agents do not close a bead
before the staged payload, verifier evidence, attestation, and commit boundary
all describe the same work.

## Delivery Versus Admin

Delivery work is any closure with a staged payload outside `.beads/`. That
includes implementation, tests, user-facing docs, templates, generated
contracts, and config. Delivery beads close through the commit gate and need
real verifier evidence for the final staged payload.

Admin work is tracker-only work whose staged payload contains only `.beads/`
paths. Admin beads may be batched when the batch is issue maintenance, triage,
or planning state that does not deliver implementation payload. Admin closure
does not require code attestation evidence and must not ask agents to fabricate
one.

When bead metadata and staged paths disagree, the staged payload decides the
path. For example, a `docs` bead with `docs/` changes is delivery, while a
`docs` bead with only `.beads/` changes is admin.

## Safe Delivery Order

Use this order for delivery beads. Keep the steps numbered in docs, generated
contracts, and command output so the sequence stays auditable.

1. Stage the delivery payload: implementation, docs, tests, or config.
   `beads close` does not scan for dirty files and does not stage payload
   files.
2. `beads close` runs preflight checks by reusing the existing preflight report
   code.
3. `br close <id> --reason "<reason>"` runs. The reason references the bead and
   feature, not a commit SHA.
4. `br sync --flush-only` runs.
5. `beads close` stages exactly `.beads/issues.jsonl`; the staged payload is
   now final.
6. Verifier evidence is collected against the final staged payload, either
   through `verifier begin` plus `verifier submit` or through a caller-supplied
   `--responses` file bound to the current staged hash.
7. `burpvalve commit --bead <id>` runs the gate and may write the attestation
   as an expected intermediate state.
8. The attestation is staged and the gate revalidates.
9. `git commit` runs only with explicit confirmation. Interactive default is
   No. Robots require `confirm: true` plus a message. Without confirmation,
   the command stops after step 8 and prints the exact commit command in
   structured `next_steps`.

Evidence gathered before step 5 is stale by construction because the final
payload includes `.beads/issues.jsonl`. The gate binds responses and
attestations to the staged payload hash, so pre-final evidence must be
replaced.

## Journal And Resume

`burpvalve beads close` creates `log/backpressure/closures/` lazily on the
first closure that needs a journal. Repositories that never use Beads do not
need the directory scaffolded during init.

Each closure journal records ordered steps, command lines, captured output
references, statuses, and timestamps. On resume, Burpvalve recomputes reality
from `br`, Git staged paths, response files, and attestation files before it
trusts a journal entry. Reality wins over journal claims.

## Drift Check

`burpvalve beads drift` is advisory. It looks for recently closed beads that do
not appear in attestation `bead_ids` or commit messages while the tree is
dirty, and reports possible drift. The command does not prove that a dirty file
belongs to a bead; it only names a risky closure boundary for follow-up. A
project may wire the drift command into lint or CI when it wants advisory
findings to become enforced policy.
