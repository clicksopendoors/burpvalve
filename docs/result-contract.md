# Burpvalve Result Contract

Burpvalve commands that produce JSON keep their older command-specific fields,
but newer outputs also include a small shared recovery contract.

Common fields:

- `schema_version`: version of the JSON result shape.
- `command`: command that produced the result, such as `setup`, `init`, `repair`, or `lint`.
- `status`: short machine-readable result state.
- `fatal`: true when the result blocks the requested operation.
- `partial_success`: true when a mutating command made some changes but still ended with conflicts.
- `next_steps`: ordered recovery actions. Setup/init/repair use structured steps with an `id`, `message`, optional exact `command`, and `fatal`. Lint and commit currently expose ordered strings for direct next actions.

`burpvalve setup --json` is non-mutating. It reports readiness with checks,
planned changes, and `next_steps`. Missing Git metadata, missing hook wiring,
missing `br`, and missing `burpvalve` on `PATH` are fatal setup blockers.
Optional tools such as NTM may appear in `next_steps` without making the setup
fatal.

Setup and repair may include `repo_local_binary` when `bin/burpvalve` exists or
cannot be inspected cleanly. The object makes hook provenance explicit without
forcing a workflow change:

- `hook_command_source`: `path`, `repo-local`, `repo-local-conflict`, or
  `missing`.
- `repo_local_path`: normally `bin/burpvalve`.
- `repo_local_exists`: whether the repo-local fallback path exists.
- `repo_local_ignored`: whether Git ignore rules hide the fallback.
- `path_command`: the Burpvalve command found on `PATH`, when available.
- `freshness_status`: exactly `fresh`, `stale`, `unknown`, or
  `not_applicable`.
- `comparison_basis`: concise facts used for the freshness decision.
- `warning_code`: non-empty only when the repo-local fallback deserves a
  diagnostic warning, for example `repo_local_stale`,
  `repo_local_freshness_unknown`, `repo_local_ignored`, or
  `repo_local_fallback_active`.

These warnings are diagnostic. A valid repo-local fallback can make
`readiness_severity` report `warning` while `status` remains `ready`. Recovery
text must present choices such as running from source, installing/using the
`PATH` command, or intentionally keeping the repo-local fallback; it must not
command deletion or replacement.

Setup may include `claude_route` when the Claude surface is governed by the
selected defaults. The object reports:

- `expected`: `agent-symlink`, `orchestrator-skill`, or `none`.
- `source`: `default`, `global`, `project`, `input`, or `prompt`.
- `detected`: the route currently present in the target repo.
- `missing_pieces`, `drift`, and `conflicts`: concrete route files that still
  need creation, repair, or human adoption.

`burpvalve init --force --json` and `burpvalve repair --force --json` are
mutating. They report created, repaired, preserved, skipped, and conflict lists.
They also report top-level `claude_route` and `claude_route_source` when the
invocation resolved a Claude route through flags, robot input, config defaults,
or an interactive prompt.
If conflicts occur after some work succeeded, `status` is `partial_success`,
`partial_success` is true, and `next_steps` describes how to inspect or recover.

`burpvalve commit` reports recovery actions for blocked commit gates. Missing or
invalid verifier responses point the agent back to `--responses`, blocked
feature detection points to staging one atomic feature or passing `--feature`,
and a generated passing attestation tells the agent exactly which JSON file to
stage before rerunning `git commit`.

`burpvalve hash --staged --json` and `burpvalve verifier prompts --json` use
the same staged-path accounting contract. `staged_payload` contains only
hash-included staged path records. `hash_excluded_staged_payload` contains
recognized generated evidence JSON that verifiers must still see, but that does
not contribute to the staged payload hash. Every staged path appears exactly
once across those two arrays, with no omissions and no duplicate all-path field.
Generated evidence JSON under `backpressure/attestations/` and
`log/backpressure/failed/` is excluded from the hash; scaffold or documentation
files in those directories remain hash-included unless they match the generated
evidence artifact rules.

`burpvalve account payload --json` reports read-only ownership accounting for
the current staged payload. Ownership claims come only from explicit C1 records
passed on stdin and/or `--ownership-file`; stdin records override file records
for the same unit/path/kind claim. Status values are `owned`,
`shared_declared`, `conflict`, `unowned`, `generated_exception`,
`ignored_untracked`, and `covered_exception`. Ownership records use structured
`expires_or_scope` values: `single_bead`, `plan_round`, or `until_commit`, with
optional free-form `expires_note`.

When `--include-beads` is set, the optional `beads` object and per-path
`beads_context` arrays are display-only context. They may show active Beads
metadata for `open` and `in_progress` issues referenced by explicit ownership
records, but they never create ownership claims, never change path status, and
never make closed, deferred, tombstoned, or historical Beads conflict
participants. In repositories without `.beads/issues.jsonl`, the command still
completes and reports Beads context as unavailable with a warning. The command
must not mutate `.beads`, reserve files, stage files, or change the worktree.

`burpvalve beads close` is a mutating state machine. It reports a closure
journal path, ordered step records, `partial_success`, and structured
`next_steps`. The normal passing-attestation bounce is reported as
`attestation_written_unstaged` with `fatal: false`, then resume or the active
flow stages exactly the named attestation and reruns the gate. If confirmation
is missing after gate revalidation, the command stops with
`awaiting_commit_confirmation` and prints the exact `git commit` command.

Verifier response files and attestation artifacts may include
`supplemental_verifiers` and `adjudication` on each condition. Supplemental
verifiers preserve additional non-primary evidence and can record disagreement.
Adjudication records owner/ruling audit metadata only; `final_verdict` never
turns a primary `fail` or `unknown` into an accepted passing artifact. Explain
and attestation query surfaces must expose supplemental disagreement with a
hold/escalate recovery step.

`burpvalve lint` distinguishes real executable enforcement from scaffold-only
policy text:

- `enforced: false`, `command_count: 0`, `evidence_strength: "none"`, and
  `enforcement_level: "scaffold-only"` means no lint command ran.
- `enforced: true` and `evidence_strength: "command-output"` means at least one
  declared lint command executed.

Do not treat skipped wishlist policy as proof that lint passed. Add exact
`lint_commands` entries to `backpressure/manifest.yaml` when the project is
ready for deterministic lint enforcement.
