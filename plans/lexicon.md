# Burpvalve Lexicon Plan

## Status

Draft 2, 2026-07-03. Owner rulings are recorded below and are binding for
review. This plan must land before the open-source launch scrub so launch docs,
README edits, help text, skill wording, and prompt-bank wording do not get
double-touched.

## Source

Owner directive, 2026-07-03: Burpvalve needs a small, durable vocabulary pass
that keeps backpressure as the anchor concept while making the product language
clearer and more memorable. The vocabulary is adopted where it improves the
surface; it is not a mandate to rename stable paths, schemas, flags, or files.

## Goals

- Keep **backpressure** as the core concept in the README pitch and any short
  product explanation.
- Define the public vocabulary once, then use it consistently in README,
  `docs/vocabulary.md`, CLI help, error text, prompt bank entries, and skill
  wording.
- Make gate refusal language more human: when the gate refuses a commit, it
  **burped** it back.
- Introduce **seal** as the friendly name for the evidence artifact while
  preserving **attestation** as the formal and interchangeable term.
- Use **valve** as the friendly name for the gate itself.
- Replace conceptual use of "bead" with **work unit** in product docs. Beads
  and `br` remain tool vocabulary and should be clearly separated from the
  product concept.
- Avoid a new `explain-terms` command. Terms belong in README and in short
  inline definitions wherever human help text uses them.

## Non-Goals

- No command surface named `explain-terms`, `glossary`, or similar.
- No mass rename of stable file paths, JSON fields, schema keys, attestation
  directories, flags, Go identifiers, or long-lived command names.
- No rename of stable feature-binding surfaces: `--feature`, `feature` JSON
  fields, Go `Feature` structs, response/template fields, `feature_name`, and
  existing feature/diff-cluster binding terminology remain compatibility
  surfaces unless a later compatibility plan explicitly changes them.
- No attempt to remove the word `attestation`. A seal is an attestation, and
  the terms are intentionally interchangeable.
- No replacement of Beads tool names when the user is literally interacting
  with Beads, `br`, `.beads/`, bead IDs, or Beads-specific command behavior.
- No launch scrub, license flip, history rewrite, or public visibility change.
  This plan only prepares the vocabulary layer that the launch scrub will use.

## Owner Rulings

The first implementation unit records these rulings in
`docs/decisions-2026-07-02-review-round.md` as `D-LEX-*` entries before any
surface text changes. The section must cite this 2026-07-03 owner directive as
the provenance for the rulings.

1. **D-LEX-1: Backpressure remains the anchor concept.** Every pitch-level
   explanation must still say Burpvalve installs repo-local backpressure or
   otherwise name backpressure as the reason the product exists.
2. **D-LEX-2: Burp is the official refusal noun and verb.** A gate refusal is
   a burp. "The gate burped the commit back" means the commit was refused.
   Blocked reports and help/error text may use this language when describing a
   refused commit.
3. **D-LEX-3: Seal is an attestation.** A seal is the evidence artifact written
   by Burpvalve. The glossary must define a seal explicitly as a type of
   attestation, and docs may use "seal" and "attestation" interchangeably.
4. **D-LEX-4: Valve is the friendly gate name.** The valve is the commit gate.
   Phrases such as "the valve is fail-closed" are accepted when they clarify
   gate behavior.
5. **D-LEX-5: Work unit is the product concept.** A work unit is the atomic
   piece of work being checked. Beads are one supported tracker/tool vocabulary
   for work units; product docs must not imply every work unit is a bead.
   Product prose may define work unit as the user-facing concept while
   `feature` remains the stable CLI/schema binding term.
6. **D-LEX-6: Glossary, not command.** The glossary lives in README and
   `docs/vocabulary.md`. CLI help that uses a lexicon term must include a
   short inline definition there rather than requiring a separate command.
7. **D-LEX-7: Adopt vocabulary where useful, not obsessively.** New surfaces
   should use the lexicon. Existing surfaces change only where cheap, clear,
   and low-risk. Stable paths, schema fields, flags, persisted evidence names,
   and formal machine-readable surfaces are not renamed merely to match the
   vocabulary.

## Vocabulary Contract

| Term | Meaning | Usage Rule |
| --- | --- | --- |
| Backpressure | The checks, gates, reviews, and evidence requirements that stop agents from self-certifying weak work. | Always appears in the short pitch. Do not replace it with "quality" or "workflow". |
| Valve | Friendly name for the gate that checks staged or committed work and refuses weak evidence. | Use in prose and human help when describing gate behavior. Keep command names stable. |
| Burp | Noun or verb for a refusal by the valve. | Use for blocked/refused commit text: "burped back" means refused. Avoid using it for crashes, panics, setup failures, lint `not_enforced`, usage errors, unavailable commands, or internal errors. |
| Seal | Friendly name for the evidence artifact written by Burpvalve. | Define as an attestation. Do not rename `backpressure/attestations/` or schema fields unless a later compatibility plan explicitly does so. |
| Attestation | Formal term for the evidence artifact and schema. | Remains valid everywhere. First-use phrasing can be "seal/attestation" or "seal, a Burpvalve attestation". |
| Work unit | The atomic unit of user or agent work that the valve checks. | Use in product docs and human help. Say "bead" only for Beads-specific features and examples. |
| Feature | Stable CLI/schema binding term for a work unit or diff cluster in existing Burpvalve APIs. | Preserve `--feature`, JSON fields, Go structs, prompt response fields, and staged/diff binding terminology. |
| Bead | A Beads tracker issue managed by `br`. | Tool vocabulary only. Explain as one possible tracker-backed work unit. |

## Implementation Principles

- The lexicon must reduce ambiguity, not add cuteness. Prefer precise,
  restrained wording over dense metaphor.
- Major surfaces are README, `docs/vocabulary.md`, CLI human help,
  blocked-report/explain text, prompt-bank rendered prompts, and
  `skill/burpvalve/SKILL.md`. First use of a lexicon term in each major
  surface should define it briefly. Each surface may link to the canonical
  glossary, but must not require a separate glossary command.
- CLI errors stay actionable. If "burped" appears, the same output must still
  name the refused feature/work unit, failed condition, blocked report path, or
  recovery command.
- Human help and prose may use lexicon terms with inline definitions. Robot
  JSON help and structured result messages should remain formal unless a term
  materially clarifies recovery. Never replace stable schema descriptions with
  metaphor-only wording.
- Existing JSON contracts stay stable. New JSON fields may add friendly
  aliases only if tests prove backward compatibility and the extra field is
  useful to agents.
- Product docs use "work unit"; Beads-specific docs use "bead" and explain
  that a bead is a tracker-backed work unit.
- README, help, skill, and prompt-bank language must agree on definitions
  before launch scrub begins.

## Work Units

### 1. Record D-LEX decisions and add canonical glossary

Scope:

- Add `D-LEX-*` rulings to
  `docs/decisions-2026-07-02-review-round.md`.
- Cite the 2026-07-03 owner directive as the provenance for the D-LEX section.
- Create `docs/vocabulary.md` with the canonical glossary.
- The glossary must define:
  - backpressure;
  - valve;
  - burp;
  - seal;
  - attestation;
  - work unit;
  - feature;
  - bead.
- Define seal as a type of attestation and say the two are usable
  interchangeably.
- Define feature as the stable CLI/schema binding term for existing
  compatibility surfaces.
- Define bead as Beads/`br` tool vocabulary, not the generic product unit.

Rationale:

The decisions doc makes the vocabulary durable before copy changes begin.
`docs/vocabulary.md` gives all later edits a source of truth and prevents
agents from inventing near-synonyms during the launch scrub.

Acceptance:

- `docs/decisions-2026-07-02-review-round.md` contains D-LEX decisions matching
  the owner rulings in this plan and cites the 2026-07-03 owner directive.
- `docs/vocabulary.md` exists and is linked-ready from README.
- `feature` compatibility surfaces are explicitly protected.
- No code, command behavior, schema field, or path is changed in this unit.

Suggested verification:

- `rg -n "D-LEX|2026-07-03|seal|attestation|work unit|feature|bead" docs/decisions-2026-07-02-review-round.md docs/vocabulary.md`
- `markdownlint` if available; otherwise `git diff --check`.

Dependencies:

- None.

### 2. Add README lexicon section and pitch line

Scope:

- Update the README opening pitch so it continues to anchor on backpressure.
- Add a compact README lexicon section that links to `docs/vocabulary.md`.
- Define the short terms inline:
  - valve: the fail-closed commit gate;
  - burp: a gate refusal;
  - seal: a Burpvalve attestation;
  - work unit: the atomic thing being checked;
  - feature: the stable CLI/schema binding term for an existing work-unit
    payload.
- Update README areas that use "bead" conceptually to "work unit"; preserve
  Beads/`br` references when discussing the tracker.
- README examples for Beads integration should explain a bead as a
  tracker-backed work unit, not erase the tool term.
- Do not perform the OSL public-audience scrub in this unit except where a
  lexicon sentence is being touched anyway.

Rationale:

README is the product entry point. It needs enough vocabulary to make later
help/error text unsurprising, but it should not become a terminology manual.

Acceptance:

- README pitch names backpressure.
- README has a short lexicon section and links to `docs/vocabulary.md`.
- README does not define seal without attestation.
- README uses "work unit" for the product concept and reserves "bead" for
  Beads-specific text.
- README preserves feature/`--feature` as stable CLI/schema language.

Suggested verification:

- `rg -n "backpressure|valve|burp|seal|attestation|work unit|feature|bead" README.md`
- `git diff --check`

Dependencies:

- Unit 1.

### 3. Update blocked-report, help, and error wording

Scope:

- Sweep CLI human help and user-facing error text for:
  - gate refusal language;
  - blocked report descriptions;
  - feature/work-unit prompts;
  - attestation/seal text;
  - Beads/bead phrasing.
- Adopt "burped" only for refusal paths where the valve rejects a staged or
  committed payload.
- Add short inline definitions anywhere human help text uses a lexicon term:
  - "valve (the fail-closed commit gate)";
  - "burped back (refused by the valve)";
  - "seal/attestation (the evidence artifact)";
  - "work unit (the atomic change being checked)".
- Keep command names stable: `commit`, `ci`, `attestations`, `beads`, and
  `verifier` are not renamed.
- Keep feature compatibility surfaces stable: `--feature`, `feature` JSON
  fields, Go `Feature` structs, response/template fields, `feature_name`, and
  feature/diff-cluster binding terminology are not renamed.
- Keep JSON field names stable unless adding a new backward-compatible message
  string is cheap and tested.
- Protect robot JSON help and structured result messages from metaphor-only
  rewrites. They should remain formal unless the lexicon term clarifies a
  recovery path and is defined locally.

Rationale:

The vocabulary must show up where agents experience refusal. The goal is better
recovery language, not a wholesale API rename.

Acceptance:

- Human help text that uses `valve`, `burp`, `seal`, or `work unit` defines
  the term inline.
- Refusal text says the valve burped the commit/work unit back only when the
  action was actually refused.
- `burp` and `burped` do not appear for setup readiness failures, config
  validation failures, lint `not_enforced`, command usage errors, unavailable
  external commands, panics, crashes, or internal errors.
- Robot-help JSON still decodes and retains stable field descriptions for
  `attestations`, `beads`, `feature`, and `staged_payload_hash`.
- If a lexicon term appears in robot help, the same string defines it locally.
- `burpvalve beads ...` human help and robot help continue to say bead/Beads
  where literally about `br`, `.beads/`, bead IDs, bead rationale, or Beads
  close/preflight behavior.
- Existing tests for help, blocked reports, and commit/ci behavior are updated
  without weakening assertions.
- No schema/path/flag-breaking rename is introduced.

Suggested verification:

- `go test ./cmd/burpvalve -run 'Help|Commit|CI|Blocked|Attestation|Bead|Feature|Robot'`
- `go test ./internal/backpressure -run 'Blocked|Attestation|Report|Plan|Feature'`
- `rg -n "burp|burped" cmd internal backpressure README.md docs skill`
- Targeted negative grep or tests showing non-refusal paths do not emit
  `burp`/`burped`.
- `go test ./...`

Dependencies:

- Unit 1.

### 4. Update prompt bank and skill wording

Scope:

- Update prompt-bank content so verifier and orchestrator prompts use:
  - work unit for generic work;
  - feature when referring to stable response fields, staged payload binding,
    or the existing feature x condition matrix;
  - bead only when the prompt is explicitly Beads-aware;
  - seal/attestation for evidence artifacts;
  - valve/burp language for gate refusal and recovery.
- Update `skill/burpvalve/SKILL.md` and related skill docs so incoming agents
  see the same vocabulary.
- Preserve backpressure as the anchor concept in skill descriptions.
- Do not add new prompt names unless the existing prompt-bank structure
  requires a glossary pointer. Prompt names are stable public API.

Rationale:

Agents rely on prompt-bank and skill text more than humans do. Inconsistent
terms there would cause implementation agents to keep writing "bead" for every
unit of work or to erase stable `feature` response fields while trying to adopt
"work unit".

Acceptance:

- Prompt-bank tests still pass.
- Skill wording uses the same definitions as README and `docs/vocabulary.md`.
- Prompt response fields and feature x condition terminology remain stable.
- No prompt name is renamed.
- No claim says NTM or Beads is required for every work unit.

Suggested verification:

- `go test ./internal/backpressure -run 'Prompt|Verifier|Condition'`
- `rg -n "work unit|feature|bead|seal|attestation|valve|burp|backpressure" internal skill`
- `git diff --check`

Dependencies:

- Unit 1.

### 5. Cheap file and directory naming alignment

Scope:

- Review new or still-fluid docs and plan filenames for lexicon alignment.
- Use the new vocabulary for new examples, new headings, and new prose where it
  improves clarity.
- Do not rename existing stable paths such as `backpressure/attestations/`,
  `.beads/`, command names, schema files, generated artifact paths, prompt
  identifiers, or config keys.
- Do not rename compatibility-bearing feature surfaces unless a later plan
  provides migration, aliases, tests, and release notes.

Rationale:

The owner ruling allows vocabulary to affect structure as needed, but rejects
vocabulary obsession. This unit prevents both extremes by limiting structural
changes to cheap, low-risk surfaces.

Acceptance:

- Any rename proposed by this unit is listed with compatibility risk and either
  executed because it is cheap/non-stable or deferred because it is stable.
- Stable paths and schema fields remain unchanged.
- New docs created during this pass use work unit, valve, burp, and
  seal/attestation consistently.

Suggested verification:

- `git diff --name-status`
- `rg -n "attestations|\\.beads|--feature|feature_name|staged_payload_hash" README.md docs cmd internal skill`
- `git diff --check`

Dependencies:

- Units 1-4.

### 6. Final lexicon consistency pass

Scope:

- Run a repo-wide text pass for the adopted vocabulary.
- Check that product prose uses "work unit" for generic atomic work and that
  Beads/`br` surfaces keep "bead" where tool-specific.
- Check that the README pitch still names backpressure.
- Check that help and errors remain actionable after friendly wording.
- Check that robot JSON, schema, and prompt response fields remain stable and
  formal.
- Record any deliberate exceptions in the final bead/issue close reason.

Rationale:

The vocabulary is cross-cutting. A final pass catches accidental drift before
the OSL launch scrub starts from this new baseline.

Acceptance:

- README, `docs/vocabulary.md`, decisions, CLI help/error text, blocked report
  wording, prompt bank, and skill wording agree on the definitions.
- `feature` remains stable in CLI/schema/machine-readable compatibility
  surfaces.
- `burp` language is limited to valve refusals.
- Beads surfaces remain Beads-specific and explain their relationship to work
  units where needed.
- Full tests pass or any unrelated blocker is explicitly documented.

Suggested verification:

- `rg -n "backpressure|valve|burp|burped|seal|attestation|work unit|feature|bead" README.md docs cmd internal skill backpressure`
- `go test ./cmd/burpvalve ./internal/backpressure ./internal/scaffold`
- `go test ./...`
- `git diff --check`

Dependencies:

- Units 1-5.

## Sequencing

1. Land this lexicon plan after adversarial review and owner ruling.
2. Implement Unit 1 first so D-LEX decisions and `docs/vocabulary.md` become
   the source of truth.
3. Implement README, help/error, prompt-bank/skill, and cheap naming units in
   separate small commits unless the owner explicitly batches them.
4. Complete the lexicon pass before the OSL launch scrub begins.
5. Keep the OSL history/docs/beads/demo audits parked until the owner rules on
   history strategy or redirects the scrub phase.

## Review Questions

- Does the plan keep backpressure central enough in the pitch and skill text?
- Are seal and attestation interchangeable without destabilizing existing
  `attestations` paths, commands, and schema fields?
- Is the work unit/bead/feature boundary clear enough for implementation
  agents to avoid accidental API churn?
- Is burp limited to actual valve refusals, with enough negative tests for
  non-refusal failures?
- Are formal robot JSON and machine-readable surfaces protected from
  metaphor-only wording?
- Are the non-renames explicit enough to prevent launch-scrub churn?
