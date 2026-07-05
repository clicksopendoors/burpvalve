# Upstream `br` Feedback Bundle - 2026-07 Dogfooding Draft

This is a local draft only. Filing issues on `Dicklesworthstone/beads_rust`
is an outward-facing action and waits for owner approval.

These proposed upstream issues come from real multi-agent dogfooding during
Burpvalve's 2026-07 planning and landing round. Multiple agents used `br` and
`bv` to convert plans into issue graphs, cross-review bead sets, edit long
self-documenting issue descriptions, and coordinate launch-readiness work. The
friction below affected audit reliability, agent safety, or handoff latency in
that live workflow. This revision incorporates an adversarial QA pass against
live `br 0.2.15`.

## Proposed Issue 1: Add machine-readable truncation warnings for `br list --json`

Ready-to-file title: Add JSON/robot-mode truncation warnings when `br list` hits its limit

Body:

Observed behavior:
`br list` defaults to `--limit 50`. In `br 0.2.15`, text mode now warns when
output is truncated, and JSON mode includes pagination fields. The remaining
agent-facing gap is that JSON/robot consumers must remember to inspect
`has_more`, `limit`, and `total`; there is no explicit warning field.

Why it matters for agent workflows:
Agents often use `br list --json` as an audit primitive. During this repo's
round, one reviewer initially counted a partial bead set as missing work, and a
later QA pass hit the same trap. In robot mode, a first-class warning is easier
to propagate into reports than an implicit pagination interpretation.

Concrete reproduction from this repo's session:

```bash
cd <repo>
br list --json | jq '.issues | length'
# observed: 50

br list --json --limit 400 | jq '.issues | length'
# observed: 102

br list --json --limit 1 | jq '{has_more, limit, total}'
# observed: pagination metadata exists, but no explicit warning field
```

Suggested improvement:
When `total > limit` in JSON/robot output, include a machine-readable warning
or diagnostics field that says the result set is truncated and names the exact
follow-up flag, such as `--limit 0` or `--limit <total>`. Keep the text-mode
warning as-is.

Severity/effort guess:
Severity: medium for agent audits. Effort: low, since pagination metadata
already exists.

## Proposed Issue 2: Promote and stabilize robot schema documentation

Ready-to-file title: Promote `br schema` / command shape docs to a stable robot-facing contract

Body:

Observed behavior:
`br 0.2.15` has useful schema and command-shape tooling:

```bash
br schema --help
br schema commands --json
```

The command currently warns that `br schema` is not a stable API. It also
documents many common output envelopes, including `list`, `show`, `ready`,
`blocked`, `dep tree`, `comments list`, and `robot-docs guide`.

Why it matters for agent workflows:
Burpvalve uses `br` as an agent-facing coordination substrate. Agents need to
write reliable `jq` probes under time pressure. The existing schema command is
exactly the right direction, but robot users need to know which parts are
stable enough to build against and which command outputs remain undocumented.

Concrete reproduction from this repo's session:

```bash
br list --json --limit 1 | jq 'keys'
# observed: has_more, issues, limit, offset, total

br show <legacy-prefix>-osl-decide-repo-home-kh4 --json | jq 'type'
# observed: "array"

br schema commands --json | jq '.commands.list, .commands.show, .commands["dep tree"]'
# observed: documented shapes and jq filters
```

Suggested improvement:
Promote a documented subset of `br schema` and `br schema commands` as the
robot-facing contract, or version the schema output so agents can detect
breaking changes. Keep the warning for unstable or experimental fields, but
give stable access paths such as `.issues[]` for `list` and `.[0]` for `show`
an explicit support policy.

Severity/effort guess:
Severity: medium for robot users. Effort: medium if backed by tests; low if the
first step is documentation.

## Proposed Issue 3: Document label grammar and suggest normalized alternatives

Ready-to-file title: Document label syntax and suggest normalized alternatives when labels are rejected

Body:

Observed behavior:
Labels containing periods, such as `v0.1.2`, are rejected. Live `br 0.2.15`
reports the exact grammar as alphanumeric, hyphen, underscore, and colon.

Why it matters for agent workflows:
Agents tend to derive labels mechanically from versions, plan names, and branch
names. Late label failures interrupt mechanical issue creation, and agents may
retry with inconsistent labels unless the valid grammar and normalization
pattern are obvious.

Concrete reproduction from this repo's session:

```bash
br create --dry-run --title label-test -l 'v0.1.2' --json
# observed error:
# label: invalid characters
# only alphanumeric, hyphen, underscore, colon allowed
```

ScarletMarsh hit this while creating release and launch bead sets. The local
guidance now uses labels like `v0-1-2`.

Suggested improvement:
Document the exact label grammar anywhere labels are accepted:
`[A-Za-z0-9_:-]+` in practical terms, matching the live error's "alphanumeric,
hyphen, underscore, colon" wording. When validation fails, include a suggested
normalized label such as `v0-1-2` for `v0.1.2`. If automatic normalization is
undesirable, a "try: ..." hint is enough.

Severity/effort guess:
Severity: low to medium. Effort: low.

## Proposed Issue 4: Clarify `br sync --flush-only` output when export is already current

Ready-to-file title: `br sync --flush-only` should distinguish "DB already exported" from "no file changes"

Body:

Observed behavior:
After successful `br update` operations, `br sync --flush-only` printed
`Nothing to export (no dirty issues)` even though `.beads/issues.jsonl` was
modified relative to git. The message was technically about DB-to-JSONL state,
but it read like a filesystem no-op.

Why it matters for agent workflows:
Agents commonly follow a sync-then-commit checklist. If `br sync --flush-only`
says "Nothing to export" while `git status` still shows `.beads/issues.jsonl`
modified, agents can misreport the handoff state.

Concrete reproduction from this repo's session:

```bash
br sync --flush-only
# observed: Nothing to export (no dirty issues)

git status --short .beads/issues.jsonl
# observed: M .beads/issues.jsonl

br sync --status --json
# observed: dirty_count: 0, db_newer: false, jsonl_newer: false
```

Suggested improvement:
Change the message to distinguish export state from git state. Example:
`DB already exported to .beads/issues.jsonl; no DB changes pending. JSONL may
still differ from git HEAD.` In JSON mode, include fields such as
`export_performed`, `db_dirty_count`, and `jsonl_path`.

Severity/effort guess:
Severity: medium for agent audit clarity. Effort: low.

## Proposed Issue 5: Add file-based input for long description/update fields

Ready-to-file title: Add file-based input for long issue descriptions, notes, design, and acceptance text

Body:

Observed behavior:
Long Markdown descriptions currently go through shell arguments for
`br create --description` and `br update --description`. Related capabilities
exist elsewhere: `br create --file` supports bulk markdown import,
`br update --agent-context` accepts inline JSON or `@path`, and
`br comments add --file <file>` can safely read comment text from a file. The
main long prose fields do not advertise equivalent file input.

Why it matters for agent workflows:
Burpvalve's bead conversion workflow intentionally creates long,
self-contained issue descriptions. Passing those through shell quoting,
heredocs, and command substitution is hazardous for agent-authored Markdown.
In this round, agents independently hit shell hazards around backticks and zsh
special variables while composing issue content.

Concrete reproduction from this repo's session:
EmeraldBrook specifically requested `--description-file`. ScarletMarsh and
EmeraldBrook both encountered shell interpretation hazards while creating or
editing bead descriptions. The live workaround was careful quoting and full
description replacement via shell variables, which is fragile.

Suggested improvement:
Extend the safe file-input pattern already present in `br comments add --file`
to long issue fields on `create` and `update`: `--description-file`,
`--notes-file`, `--design-file`, `--acceptance-file`, and/or a consistent
`@path` convention accepted by the existing text flags.

Severity/effort guess:
Severity: high for agent-heavy projects. Effort: medium.

## Proposed Issue 6: Add safe append or patch operations for long issue prose

Ready-to-file title: Support append or patch-style edits for long issue text fields

Body:

Observed behavior:
`br update --description` replaces the full description. For review-polish
work, a small correction requires retrieving the entire description, appending
or editing locally, and sending the whole block back. `br comments add --file`
is a useful safe append-like path, but comments are not the same as updating
the canonical description or acceptance text.

Why it matters for agent workflows:
Long descriptions are the source of truth for self-contained beads. All-or-
nothing replacement increases the chance of accidental content loss, especially
when multiple agents are polishing different parts of a plan. It also makes
small review fixes look like wholesale rewrites in JSONL diffs.

Concrete reproduction from this repo's session:
During the OSL polish pass, the reviewer needed to append binding
clarifications to existing beads. The safe pattern was:

```bash
current=$(br show "$id" --json | jq -r '.[0].description')
br update "$id" --description "${current}

Cross-review polish binding clarification:
..."
```

Suggested improvement:
Add explicit partial-edit operations for canonical issue fields, for example
`--append-description-file`, `--prepend-description-file`, or a patch mode with
clear conflict behavior. Keep `br comments add --file` documented as a safe
comment path, but do not require agents to use comments when the canonical
description needs a binding correction.

Severity/effort guess:
Severity: medium. Effort: medium.

## Proposed Issue 7: Add focused changed-ID/field diffs for review handoffs

Ready-to-file title: Add focused issue field diffs for changed IDs or time windows

Body:

Observed behavior:
`br 0.2.15` has `br history diff`, and that should be acknowledged as the
existing history-level diff path. The remaining gap is a focused, agent-friendly
changed-ID or field-level diff for recent issue edits. `git diff
.beads/issues.jsonl` is noisy because each issue is a large JSONL line, and
`br show` only shows final state.

Why it matters for agent workflows:
Review agents need to report exactly what they changed and let coordinators
audit those changes quickly. A field-level diff for selected IDs would make
multi-agent plan polish much easier to verify than raw JSONL or whole-history
diffs.

Concrete reproduction from this repo's session:
The OSL polish pass updated six issue descriptions. `git diff -- .beads/issues.jsonl`
showed the changed issues, but the output was large and noisy enough to require
manual truncation. The final report reconstructed changes from command history
and `br show` output. `br history diff` exists, but it is not a focused
`these IDs and fields changed` review surface.

Suggested improvement:
Add a focused diff command or history mode, for example:

```bash
br history diff --ids id1 id2 --fields title,description,labels
br history diff --since <timestamp> --format json
br diff --ids id1 --fields description --json
```

The output should show field-level before/after for title, description, notes,
labels, status, priority, dependencies, and timestamps where history is
available.

Severity/effort guess:
Severity: medium for multi-agent review. Effort: medium to high, depending on
history snapshot availability.

## Proposed Issue 8: Add compact or transitive-reduced dependency tree views

Ready-to-file title: Add compact/transitive-reduced modes for `br dep tree`

Body:

Observed behavior:
Dense plan graphs make `br dep tree` hard to read. In `br 0.2.15`,
`br dep tree --help` documents `--format <FORMAT>` with possible values
`text` and `mermaid`. The global `--json` flag produces a flattened pre-order
array of tree nodes with `depth`, `parent_id`, and `truncated`; there is no
compact or transitive-reduction view.

Why it matters for agent workflows:
Plan conversion and cross-review depend on graph inspection. Dense dependency
trees can bury the terminal blockers or make a graph look more complex than it
is. Agents then either over-report risk or avoid graph validation beyond
`br dep cycles`.

Concrete reproduction from this repo's session:

```bash
br dep tree <legacy-prefix>-vorch-03-verifier-prompts-bound-contract-6vz --help
# observed --format possible values: text, mermaid

br dep tree <legacy-prefix>-vorch-03-verifier-prompts-bound-contract-6vz --json
# observed flattened JSON tree node array
```

Suggested improvement:
Add a compact view that collapses repeated subtrees and/or a
transitive-reduction mode that hides edges implied by other paths. JSON output
could include both raw edges and reduced edges so tools can choose.

Severity/effort guess:
Severity: low to medium. Effort: medium.

## Proposed Issue 9: Document historical issue ID prefixes without implying migration semantics

Ready-to-file title: Document why existing issue IDs can keep old project prefixes

Body:

Observed behavior:
New beads in this repo still use a legacy pre-Burpvalve prefix even though the
repo is now Burpvalve. This proves prefix persistence in this repository's
history. It does not, by itself, validate any prefix migration behavior.
`br sync --help` exposes `--rename-prefix`, which should be documented
carefully because ID changes affect audit references.

Why it matters for agent workflows:
Agents often infer ownership and scope from IDs. A stale-looking prefix can be
misread as evidence that the wrong database is loaded or that issues should be
renamed. In an audit trail, unnecessary ID renaming can break references in
commits, docs, and mail threads.

Concrete reproduction from this repo's session:
The OSL and OXP beads created on 2026-07-02 still have IDs like
`<legacy-prefix>-osl-...` and `<legacy-prefix>-oxp-...`. The local
decision was to keep historical IDs immutable. No migration test was performed.

Suggested improvement:
Add docs explaining:
- why existing IDs can keep an old prefix;
- how the configured prefix affects new IDs;
- what `sync --rename-prefix` is intended to do and when it is safe;
- why agents should not rename IDs merely for aesthetics.

Severity/effort guess:
Severity: low. Effort: low.

## Proposed Issue 10: Add agent-safe shell examples to the robot-docs guide

Ready-to-file title: Add shell-safe Markdown authoring guidance to `br robot-docs guide`

Body:

Observed behavior:
Agents authored large Markdown issue descriptions through shells. Backticks,
unquoted heredocs, redirection-looking prose, and shell-reserved names caused or
nearly caused corruption. One related Burpvalve finding involved zero-byte files
created by shell-redirection fallout from pasteable workflow prose; another
agent hit zsh's read-only `$status` variable while generating command text.

Why it matters for agent workflows:
`br` is often used by agents as a planning substrate, not just by humans typing
short issue titles. Agent prompts frequently include Markdown code spans,
variables, and command snippets. Unsafe examples invite subtle data loss or
accidental command execution before `br` receives the intended text.

Concrete reproduction from this repo's session:
Section B of `docs/dogfooding-findings-2026-07.md` records the shell-authoring
hazard, and multiple agents independently reported it while converting plans to
beads. `br schema commands --json` identifies `robot-docs guide` as an existing
agent-facing documentation surface.

Suggested improvement:
Add an "agent-safe examples" subsection to the `br robot-docs guide`:
- prefer file-based body input when available;
- use single-quoted heredoc delimiters for generated Markdown;
- avoid putting raw Markdown containing backticks directly in shell command
  substitution;
- avoid prose snippets that look like shell redirection in copy/paste contexts;
- mention zsh/bash differences only where they affect examples.

This complements, but does not replace, the file-input feature request.

Severity/effort guess:
Severity: medium for agent users. Effort: low as docs.

## Proposed Issue 11: Document `-s deferred` as a first-class planning pattern

Ready-to-file title: Document `br create -s deferred` for explicit future work

Body:

Observed behavior:
`br create -s deferred` worked well during plan conversion for work that was
intentionally out of the current implementation path. The positive behavior is
worth documenting because agents otherwise tend to create open low-priority
tasks for deferred work, making `ready` and triage noisier.

Why it matters for agent workflows:
Good planning requires preserving future work without making it look
immediately actionable. A clear deferred-status pattern lets agents keep
non-goals visible while protecting the current ready queue.

Concrete reproduction from this repo's session:
The orchestrator-experience conversion created a deferred P4 option-A bead for
future wholesale migration/repair work. The status pattern kept it out of the
current executable plan while preserving the required first deliverable.

Suggested improvement:
Add a short docs example:

```bash
br create "future: investigate option A" -t task -p 4 -s deferred \
  --description "Deferred until option B ships; first deliverable is ..."
```

Clarify how deferred issues appear in `br list`, `br ready`, and `bv` triage,
and when to use `--defer <date>` versus `-s deferred`.

Severity/effort guess:
Severity: low. Effort: low.

## Proposed Issue 12: Validate `br dep tree --format` enum values

Ready-to-file title: Reject invalid `br dep tree --format` values instead of silently falling back

Body:

Observed behavior:
`br dep tree --help` says `--format` accepts `text` or `mermaid`. Passing
`--format json` does not fail in live `br 0.2.15`; it prints the normal text
tree. The actual JSON path is the global `--json` flag.

Why it matters for agent workflows:
Agents infer command contracts from help text. If an invalid enum value is
accepted and silently treated as text, an agent may believe it produced JSON or
may build a parser around an accidental fallback.

Concrete reproduction from this repo's session:

```bash
br dep tree <legacy-prefix>-vorch-03-verifier-prompts-bound-contract-6vz --format json
# observed: text tree output, exit success

br dep tree <legacy-prefix>-vorch-03-verifier-prompts-bound-contract-6vz --json
# observed: JSON array of tree nodes
```

Suggested improvement:
Validate `--format` against the documented enum and reject invalid values with a
structured error. Optionally include a hint: "Use global `--json` for JSON
output."

Severity/effort guess:
Severity: medium for robot correctness. Effort: low.

## Proposed Issue 13: Fill command coverage gaps in `br schema commands`

Ready-to-file title: Add missing sync command envelopes to `br schema commands`

Body:

Observed behavior:
`br schema commands --json` is useful and already covers many commands, but it
does not currently document `sync --status`, `sync --flush-only`, `sync
--witness`, or other sync-mode output envelopes. This is separate from the
broader stability question: even as an experimental schema, the coverage gap
matters for agents because sync is part of the standard Beads handoff loop.

Why it matters for agent workflows:
Agents regularly run `br sync --flush-only` and `br sync --status --json` at
handoff time. If those shapes are missing from `br schema commands`, agents
fall back to ad hoc parsing for the exact commands that determine whether the
DB and JSONL are synchronized.

Concrete reproduction from this repo's session:

```bash
br schema commands --json | jq '.commands | keys'
# observed many command shapes, but no sync entries

br sync --status --json
# observed object with dirty_count, last_export_time, last_import_time,
# jsonl_content_hash, jsonl_exists, jsonl_newer, db_newer
```

Suggested improvement:
Add command-shape entries for sync modes, at minimum:
- `sync --status`
- `sync --flush-only`
- `sync --import-only`
- `sync --merge`
- `sync --witness`

For modes that primarily emit human text unless `--json` is present, document
both the JSON shape and whether errors use the structured error envelope.

Severity/effort guess:
Severity: medium for agent handoffs. Effort: low to medium.
