# Verifier Orchestration: Spawn Profiles, Prompt Contracts, And Result Ingestion

## Status

Planning, revision 2. Revised 2026-07-02 after two independent Codex reviews
(Agent Mail messages 2270/2271; both "needs revision" — the submit contract,
hash binding, atomicity handling, and config schema were underspecified
against the real code). Convert to beads only after this revision is
accepted. Depends on `plans/land-working-set.md` (the verifier policy model
and `verifier prompts` command are this plan's foundation).

## Source

GitHub issue #13 and comments; issue #11's comment (explicit command +
success-criteria response contract); issue #12 and decision D7 (standing
pre-authorization, never simulated evidence); decision D8 in
`docs/decisions-2026-07-02-review-round.md`.

## Problem

The gate demands independent verification per feature x condition cell, but
gives the committing agent little help producing it. Observed failures (#13):
agents don't spawn verifiers, or spawn arbitrary ones with improvised prompts
and summarize loosely into the response file — verification theater.

**What already exists** (this plan extends, it does not rebuild):
`internal/backpressure/verifier_prompts.go` already generates per-cell packets
with native/ntm/hermes/manual profiles, authorization text, read-only
expectations, `VerifierPolicy`, a `ResponseCondition` schema object (JSON
output), and profile notes. `attestations.Verifier` already carries
`TranscriptRef`/`EvidenceRef`. What's missing: a recorded authorization fact,
hash bindings, a submission/ingestion path, atomicity handling, evidence
requirements on pass, transcript rules, and preference storage.

## Goals

- Verifier preferences and a recorded standing authorization at init/config.
- Per-cell contracts a verifier can execute with zero improvisation.
- A deterministic ingestion path (`verifier begin` + `verifier submit`) that
  assembles a commit-acceptable responses file with per-condition provenance.
- Attestations whose `subagent_confirmed` is auditable via transcript refs.
- Report-only runtime-limits doctor.

## Non-Goals

- Burpvalve does not spawn subagents. It is a contract generator, blocking
  checklist, and evidence ledger.
- No fabricated or simulated authorization or evidence (D7). The recorded
  authorization is permission to spawn; it is never per-cell evidence, and
  `verifier submit` must never accept it as such.
- One atomic feature per staged payload remains the scope
  (`BuildVerifierPrompts` uses the single detected feature; no multi-feature
  support is implied here).
- No hard dependency on NTM/Hermes; the manual profile always works.

## Design

### 1. Config schema unit (explicit, because the parser is strict)

`internal/config` uses `DisallowUnknownFields`; a `defaults.verifier` block
breaks config load today. This is therefore a full schema unit, not a side
effect: add `Verifier` to `Defaults` with strict validation, merge semantics,
`noteDefaultsSources` entries, `config show`/`config init` (TUI + `--robots`)
support, and tests — mirroring how completion/init defaults are wired.

```json
{
  "defaults": {
    "verifier": {
      "authorized": true,
      "authorized_at": "2026-07-02",
      "authorization_scope": "read-only verifier subagents for backpressure checks",
      "spawn_method": "native",
      "default_model": "sonnet",
      "condition_models": { "security-boundaries": "high-reasoning" },
      "read_only_tools": true,
      "max_parallel_verifiers": 4,
      "transcript_dir": "log/backpressure/verifiers",
      "transcripts": "summary"
    }
  }
}
```

Validation rules (binding): `spawn_method` in `native|ntm|hermes|manual`;
`max_parallel_verifiers` in 1..32 (advisory — help text and robot output must
label it "guidance to the runtime; Burpvalve does not enforce or observe
spawn counts"); `transcript_dir` repo-relative, cleaned, no traversal;
`transcripts` in `summary|full|committed`; model names are free-form local
alias strings (runtime resolves them; Burpvalve never validates against a
provider list); `condition_models` keys are validated against manifest
condition IDs **at use time** with a warning (not config-load failure, since
config is global and manifests are per-repo). Project `.burpvalve.json`
overrides global per existing precedence; a project may set
`"authorized": false` to revoke.

### 2. Standing pre-authorization (D7, issue #12) — a recorded fact, not embedded text

Today `VerifierAuthorizationText` is embedded unconditionally in packets, and
similar text sits in the AGENTS template's NTM section. Embedded boilerplate
is not a recorded human answer. Changes:

- Init asks once, explicitly: "Are agents in this repo authorized to spawn
  read-only verifier subagents for backpressure checks?" The answer is stored
  in the config `authorized`/`authorized_at`/`authorization_scope` fields.
- `verifier prompts` embeds the authorization text **only when a recorded
  authorization exists**; otherwise it prints a warning and the packet says
  "authorization not recorded — obtain it via `burpvalve config init` or ask
  the repo owner" instead of claiming authority.
- The generated `AGENTS.md` gains a composable verifier section (rendered
  only when verifier preferences were configured; independent of the NTM
  section, since a repo can use verifiers without NTM) recording the same
  fact in prose.
- Hard rule, stated in code comments and docs: the authorization string, by
  itself, is never acceptable evidence for any cell. `verifier submit`
  rejects submissions whose evidence consists only of authorization language.

### 3. Per-cell verifier contract (extends the existing generator)

Additions to the existing packet, per feature x condition cell:

1. **Bindings**: `staged_payload_hash`, `manifest_hash`,
   `condition_file_hash` — the packet now carries what the submission must
   echo (today `VerifierPromptPacket` has none of these, which is why staleness
   can't be checked).
2. **Content**: condition file contents inline (not just the path), staged
   paths with git status (existing), and — bounded — staged content excerpts:
   packets cap inline content at a documented size; beyond it, the packet
   lists paths and instructs the verifier to read them (read-only). Full
   "zero repo spelunking" is thereby scoped honestly: zero *improvisation*,
   minimal reading. Secret-conscious: no environment, no config values, no
   credentials in packets.
3. **Task**: success criteria ("pass only if...") from the condition file,
   verdict vocabulary, per-verdict evidence requirements.
4. **Submission**: the exact `burpvalve verifier submit` command line with
   bindings pre-filled, plus the verdict JSON schema **rendered inline in
   human/manual output too** (today only JSON output carries
   `response_schema`; `renderVerifierPromptPacket` just says "match the
   response_schema").
5. **Hygiene**: profile-specific text (native: close the subagent thread when
   done; ntm: batch cells per reviewer pane per `docs/ntm-bridge.md`).

### 4. Ingestion: `verifier begin` + `verifier submit`

Review round established a single per-cell submit cannot satisfy
`validateResponses` (which requires top-level atomicity plus one entry per
enabled condition). The flow is therefore two commands:

- **`burpvalve verifier begin`** creates
  `log/backpressure/responses/<staged-payload-hash>.json` from
  `BuildResponsesTemplate`: all enabled conditions stubbed as `unknown`, a
  top-level binding block (`staged_payload_hash`, `manifest_hash`, condition
  file hashes), and atomicity **explicitly set by the caller** via required
  flags (`--one-feature --atomicity-message "..."`) — atomicity is the
  committing agent's own attestation, made once at begin time, and preserved
  by submits. Refuses to run if the staged payload is empty.
- **`burpvalve verifier submit --condition dry --json < verdict.json`**
  updates exactly one condition entry in the begin-created file:
  - Verdict JSON must echo the binding hashes from the packet; mismatch with
    the file's binding block (or with the *current* staged payload hash) is a
    rejection with a result-contract error naming which binding went stale.
  - Requires verifier provenance (kind/model/runtime/separate_context per the
    landed #11 model) and — for ALL verdicts including `pass` — non-empty
    `evidence`; `not_applicable`/`fail`/`unknown` additionally require
    `message` (pass-needs-evidence is stricter than today's
    `validateResponseCondition`; see Compatibility).
  - Merge semantics: replace-in-place for that condition ID; unknown
    condition IDs rejected; condition order preserved from the template;
    atomic write (temp + rename) with an exclusive file lock so parallel
    submits do not corrupt the file; resubmission of an already-passing cell
    is allowed with a printed warning.
  - Transcript: accepts `--transcript <path|->`; by default stores a
    **compact summary** under `transcript_dir` (config `transcripts: full`
    stores raw; `committed` additionally means the project intends to track
    the dir). Filenames: `<staged-hash-short>-<condition-id>.md`. Paths in
    `transcript_ref` are repo-relative. Missing transcript at commit time is
    a warning, not a block.
- **`commit --responses` auto-discovery** (resolved open question): when no
  `--responses` flag is given and no TTY questionnaire is in play,
  `runPreCommit` looks for `log/backpressure/responses/<current-staged-hash>.json`.
  Keyed by hash, a stale file simply doesn't match and is reported ("found
  responses for a different staged payload; re-run verifier begin"). The
  explicit flag always overrides. Incomplete files (any `unknown` cells) are
  reported per existing gate behavior.

### Compatibility (binding decision)

Response files carrying the new top-level binding block are validated
strictly (bindings + evidence-on-pass). Legacy unbound `--responses` files
keep today's validation unchanged, with a printed notice that bound flows
exist. Rationale: breaking every existing caller in one release is
unnecessary; the gate's `ExpectedBinding(plan)` check still protects
attestation integrity for legacy files. Tightening legacy validation can be a
later, separately-decided step.

The honest limitation stays documented: Burpvalve cannot prove a separate
context ran; it requires provenance, evidence, and transcript refs, and makes
false claims reviewable. Main agent runs `begin`/`submit`; the subagent stays
read-only and returns verdict JSON (resolved: this is the default; a
direct-subagent submit mode is a later opt-in).

### 5. Runtime limits doctor: `burpvalve verifier doctor` (read-only)

Resolved naming: `verifier doctor` (not folded into `setup`, which should not
grow runtime-config scope). First increment is report-only with a fixed check
list: presence/paths of known runtime config files (Claude Code settings
JSON, Codex config, NTM config), the subagent limit/depth values it can read
from formats it recognizes, and `unknown` for anything it cannot parse — it
never invents "exact manual changes" for unrecognized formats, and its JSON
report schema includes `supported: false` outcomes. Acceptance must prove it
writes nothing. Mutation, if ever, is a separate later increment behind
explicit confirmation.

### 6. NTM prompt storage — downgraded to a deferred optional increment

Review found no robot API for registering prompts into NTM palette/storage,
and writing `.ntm/templates` collides with D2 (`.ntm/` is gitignored runtime
state). Therefore: the first-class ntm profile behavior is just packet
formatting suitable for `ntm --robot-send`. Optional template installation
into `~/.config/ntm/templates` (user config, not repo state) is a deferred
increment, behind explicit confirmation defaulting to No, and only if the
`ntm template` contract is verified at implementation time.

## Increments

1. Config schema unit (`defaults.verifier`, validation, merge, sources,
   config init/show/robots, tests) + init authorization question + composable
   AGENTS.md verifier section.
2. Contract upgrade to `verifier prompts`: binding hashes, condition file
   contents, bounded staged content, submission command text, inline schema
   in human output, hygiene text, no-recorded-authorization behavior.
3. `verifier begin` + `verifier submit` with bindings, evidence rules, merge
   semantics, locking, transcripts; `commit --responses` hash-keyed
   auto-discovery; compatibility notice for legacy files.
4. `verifier doctor` report-only.
5. (Deferred) NTM template installation; (deferred) direct-subagent submit
   mode; (deferred) legacy response-file strictness tightening.

## Acceptance Criteria

- A `.burpvalve.json` with the documented verifier block loads, merges,
  validates, and reports sources; malformed values fail with named errors.
- `verifier prompts` without recorded authorization warns and does not claim
  authority; with it, packets embed the recorded scope.
- Full loop on a fresh repo: `verifier begin` → N x `verifier submit` →
  `burpvalve commit` (no `--responses` flag, auto-discovered) → passing
  attestation where every condition carries provenance and a
  `transcript_ref` resolvable **on the originating workspace** (committed
  evidence only under `transcripts: committed`).
- `verifier submit` rejects: binding mismatch (named), missing evidence on
  any verdict, missing message on non-pass, unknown condition ID,
  authorization-only evidence, and concurrent-write corruption (lock test).
- Legacy `--responses` files validate exactly as before, with the notice.
- `verifier doctor` never writes; unknown formats yield `supported: false`,
  not invented instructions.
- All new surfaces have result-contract JSON with `next_steps`.

## Open Questions For Grilling

1. Should `transcripts: committed` also flip `.gitignore` handling of the
   transcript dir automatically, or only document the intent and leave
   tracking to the project? (Recommendation: document only; no automatic
   gitignore edits.)
2. Packet inline-content cap: fixed bytes (e.g. 64 KiB) vs per-file cap vs
   config knob. (Recommendation: fixed default with a config override later;
   pick the number during implementation against real packet sizes.)
