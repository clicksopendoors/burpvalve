# Burpvalve Vocabulary

This glossary is the canonical source for Burpvalve product language. It
records the D-LEX owner rulings from 2026-07-03 and keeps friendly wording
separate from stable command, schema, and tracker vocabulary.

## Backpressure

Backpressure is the checks, gates, reviews, and evidence requirements that stop
agents from self-certifying weak work.

Use backpressure in the short pitch and in product explanations. Do not replace
it with a vague synonym such as "quality" or "workflow"; backpressure is the
reason Burpvalve exists.

## Valve

The valve is the friendly name for Burpvalve's fail-closed commit gate.

Use valve in prose and human help when describing the gate's behavior, such as
"the valve is fail-closed." Do not rename commands or machine-readable fields
to match the friendly term.

## Burp

A burp is a refusal by the valve. As a verb, "the valve burped the commit back"
means the commit was refused.

Use burp or burped only for real gate refusals. Do not use it for setup
readiness failures, config validation failures, lint `not_enforced`, command
usage errors, unavailable external tools, panics, crashes, or internal errors.
Refusal text still needs recovery facts: the work unit, condition, blocked
report path, or next command.

## Seal

A seal is a Burpvalve attestation: the evidence artifact written by the valve
for a checked staged payload.

Seal is the friendly term, not a replacement for the formal attestation model.
Do not rename `backpressure/attestations/`, attestation commands, or schema
fields merely to use this word.

## Attestation

An attestation is the formal evidence artifact and schema record produced by
Burpvalve.

Attestation remains valid terminology everywhere. Seal and attestation are
interchangeable in prose when the text makes clear that a seal is a Burpvalve
attestation.

## Work Unit

A work unit is the atomic piece of user or agent work that the valve checks.

Use work unit in product docs and human help when describing the generic thing
being gated. A work unit may be tracker-backed, but it does not have to be.

## Feature

Feature is the stable CLI and schema binding term for an existing work unit
payload or diff cluster.

Preserve compatibility-bearing feature surfaces, including `--feature`,
feature JSON fields, Go `Feature` structs, response/template fields,
`feature_name`, and staged-payload or diff-cluster binding terminology. Product
prose may explain that a feature is the current machine binding for a work
unit, but this vocabulary pass does not rename those surfaces.

## Bead

A bead is a Beads/`br` tracker issue.

Use bead only when the text is literally about Beads, `br`, `.beads/`, bead
IDs, bead rationale, or Beads-specific preflight and close behavior. A bead is
one possible tracker-backed work unit; it is not the generic Burpvalve product
concept.

## Stability Rules

The lexicon improves prose and human-facing help. It does not by itself rename
stable paths, schema fields, flags, generated artifact directories, prompt IDs,
command names, Go identifiers, or robot JSON contracts.

Robot JSON and structured result messages should remain formal unless a
lexicon term materially clarifies recovery and is defined locally.
