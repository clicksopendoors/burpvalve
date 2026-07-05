# Burpvalve Issue Drafts - 2026-07 Post-v0.1.2 Batch

Local draft only. Do not file these issues before v0.1.2 lands. Filing belongs
to the `rel:` issue-hygiene work or a direct owner instruction.

Source: `docs/dogfooding-findings-2026-07.md`, limited to issue candidates for
Burpvalve's own tracker.

## 1. Add a canonical commit choreography explainer command

Ready-to-file title:

```text
Add a canonical commit choreography explainer command
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; this is product follow-up,
not release-blocking cleanup.

Body:

```markdown
## Behavior observed

During the 2026-07 dogfooding landing sequence, the commit gate choreography
was hard enough to explain that the landing plan repeated the full contract in
24 separate beads. The confusing parts were not one command; they were the
ordered interaction between source invocation, staged-slice verification,
first-pass attestation writing, staging the attestation, and the rule that no
new payload files may be staged after the attestation pass.

Session evidence: `docs/dogfooding-findings-2026-07.md` A1 records this as a
finding from EmeraldBrook message 2277.

## Why it matters

Agents and humans need one canonical place to inspect the commit flow. When the
flow is restated in many beads, small wording drift can create bad recovery
instructions or accidental shortcuts. A command also gives future docs and
issues something stable to cite.

## Suggested fix

Add a command such as `burpvalve explain commit` or `burpvalve commit --explain`
that prints the canonical commit choreography as numbered steps. The output
should be paste-safe, should avoid shell-active arrow workflow chains, and
should distinguish:

- the initial gate run that may write an attestation;
- the required step of staging that generated attestation;
- the final gate run that validates the staged attestation;
- the prohibition on adding payload files after attestation generation;
- the difference between running through the pre-commit hook and running an
  explicit validation command by hand.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A1.
- Existing partial coverage: `burpvalve-land-10-recovery-contract-explain-mp8`
  adds the general read-only `burpvalve explain` command, and
  `burpvalve-oxp-prompt-bank-content-f6l` adds a canonical
  `commit-choreography` prompt. This issue tracks a product-facing commit
  choreography command/surface not fully owned by either bead.
- Related acceptance gate: `burpvalve-oxp-final-acceptance-z6e`.
```

## 2. Add paste-safety lint for Burpvalve generated output

Ready-to-file title:

```text
Add paste-safety lint for generated Burpvalve output and templates
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; the landing sequence has
targeted sanitization work, but this issue tracks making the protection
systematic.

Body:

```markdown
## Behavior observed

The July dogfooding round produced repeated evidence that agent-facing text can
turn into shell damage when copied too literally:

- Issue #16's zero-byte files match shell-redirection fallout from a bare ASCII
  arrow workflow chain.
- EmeraldBrook produced a malformed bead when Markdown backticks inside a
  heredoc were interpreted by zsh.
- ScarletMarsh hit the same backtick-in-heredoc hazard independently.

Session evidence: `docs/dogfooding-findings-2026-07.md` A2 records the repeated
paste-safety failures. The landing set also includes residue work for the
generated-output surface through `land: 19 arrow-chain sanitization` and the
separate `land: verify issue #16 arrow workflow sanitization` bead.

## Why it matters

Burpvalve deliberately emits instructions that agents may paste into shells,
commit messages, issue bodies, or heredocs. If generated text contains shell-
active punctuation in a workflow shape, the tool can create the exact class of
accidental artifact it is supposed to prevent.

## Suggested fix

Add a lint-style check over Burpvalve generated output, scaffold templates, help
examples, prompt bank content, and docs that are likely to be copied by agents.
The first increment should catch at least:

- prose workflow chains using the raw ASCII arrow token;
- unquoted Markdown backticks in heredoc-friendly snippets;
- shell examples where punctuation is explanatory but would execute as syntax
  if pasted.

The check should allow legitimate cases, including symlink notation, quoted
hazard examples, and fenced examples explicitly labeled as non-shell text.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A2.
- Landing beads: `burpvalve-land-19-arrow-chain-sanitization-ibp`
  and `burpvalve-land-verify-issue-16-arrow-workflow-qjq`.
```

## 3. Warn when setup would use a stale repo-local binary

Ready-to-file title:

```text
Warn when hooks would run a stale repo-local Burpvalve binary
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; this is a reusable product
guard found during self-hosting.

Body:

```markdown
## Behavior observed

Burpvalve's own repo hit the generic self-hosting problem: a repository cannot
fully gate the commit that first tracks its own gate rules, and an installed
hook can prefer a repo-local binary that is older than the current working
tree. The landing plan used a Phase 0 bootstrap exception and a `go run` gate
contract to avoid validating the first commit with stale code.

Session evidence: `docs/dogfooding-findings-2026-07.md` A3 records the stale
repo-local binary trap.

## Why it matters

If a hook silently runs an old binary while the repo is changing Burpvalve
itself, the gate can produce misleading results. The user sees a gate decision,
but the decision may not reflect the code being landed.

## Suggested fix

Teach `burpvalve setup`, and possibly `burpvalve repair`, to detect when:

- a hook is installed;
- the hook would run a repo-local Burpvalve binary;
- the repo-local binary is absent, ignored, or older than the current source
  tree or installed global binary.

When detected, print a warning that names which binary will be used, where it
was found, and how to intentionally select the current source path or the
installed global binary. The warning should not force a particular development
workflow; it should make the active gate path explicit.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A3.
- Related landing doctrine: `plans/land-working-set.md` Phase 0 bootstrap
  exception.
```

## 4. Make commit-gate next_steps aware of hook context

Ready-to-file title:

```text
Make commit-gate recovery next_steps aware of pre-commit hook context
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; the issue tracks the product
fix even though the first observed case was partly caused by a stale local hook.

Body:

```markdown
## Behavior observed

The first live gate firing in this repo blocked correctly on ambiguous feature
detection, but its recovery text told the agent to pass `--feature` explicitly.
That instruction made sense for direct CLI use, but the failure happened inside
the pre-commit hook where the user could not add CLI flags to the already
running hook invocation.

Session evidence: `docs/dogfooding-findings-2026-07.md` A6 records the first
gate block, the dead-end recovery instruction, and the later diagnosis that the
local hook was stale. The general UX problem remains valid.

## Why it matters

A fail-closed gate is useful only if the recovery path is executable from the
context where the user saw the failure. Hook-time errors should teach users how
to rerun `git commit` with the right environment variables, not only how to run
`burpvalve commit` manually.

## Suggested fix

When Burpvalve is running under an installed hook, tailor `next_steps` to the
hook path. For example, suggest setting `BURPVALVE_FEATURE` before rerunning the
commit when feature detection is ambiguous. For direct CLI runs, keep the flag-
based instructions.

The JSON result contract should expose enough context for agents to choose the
right recovery path without guessing.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A6.
- Covering bead: `burpvalve-oxp-hook-context-next-steps-gff`.
- Related doc: `docs/result-contract.md`.
```

## 5. Add commit-scoped CI validation for attested commits

Ready-to-file title:

```text
Add burpvalve ci --commit for commit-scoped attestation validation
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; this is already specified in
the orchestrator-experience bead set and should be filed as a tracking issue.

Body:

```markdown
## Behavior observed

After land-01, the orchestrator tried to use `burpvalve ci` as a quick reviewer
check for the committed attestation. The command instead re-derived its plan
from the dirty current tree and blocked on multiple diff clusters. There was no
way to say "validate this already-created commit and its attestation."

Session evidence: `docs/dogfooding-findings-2026-07.md` A11 records the failed
reviewer workflow.

## Why it matters

Reviewers need a commit-scoped audit command that ignores unrelated dirty
working tree state. Without it, the tool is good at gating the committing
agent, but less useful to the human or orchestrator trying to audit a landed
commit quickly.

## Suggested fix

Add `burpvalve ci --commit <sha>` to validate one committed attestation against
the named commit. The command should:

- read the committed tree and attestation artifacts from that commit;
- avoid current working tree feature detection;
- report whether the attestation matches the committed payload and active
  condition metadata expected by the policy;
- produce both human-readable output and the existing JSON result contract.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A11.
- Covering plan: `plans/orchestrator-experience.md`.
- Covering bead: `burpvalve-oxp-ci-commit-audit-g8z`.
```

## 6. Forward BURPVALVE_BEAD through installed hooks

Ready-to-file title:

```text
Forward BURPVALVE_BEAD through pre-commit hooks into attestations
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; this is already specified in
the orchestrator-experience bead set and should be filed as a tracking issue.

Body:

```markdown
## Behavior observed

The installed hook forwards `BURPVALVE_FEATURE`, `BURPVALVE_RESPONSES`,
`BURPVALVE_AGENT`, and `BURPVALVE_MODEL`, but it does not forward
`BURPVALVE_BEAD`. That means a hook-gated commit cannot pass `--bead` through
the hook, and attestations can end up with `bead_ids: null`.

Session evidence: `docs/dogfooding-findings-2026-07.md` A12 records the
land-01 traceability workaround: set `BURPVALVE_FEATURE` to the bead ID so the
identifier survives indirectly.

## Why it matters

The bead ID is task traceability, not just a display label. If the hook drops
it, the committed attestation loses the strongest direct link back to the local
work graph.

## Suggested fix

Forward `BURPVALVE_BEAD` through hook templates and parse it exactly as the CLI
`--bead` flag expects. The fix should cover install, setup, repair, template
tests, and any JSON or robot output that explains hook-supported environment
variables.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A12.
- Covering plan: `plans/orchestrator-experience.md`.
- Covering bead: `burpvalve-oxp-burpvalve-bead-hook-forwarding-9ns`.
- Existing partial coverage: `burpvalve-land-16-beads-delivery-preflight-6gy`
  records `bead_ids` and rationale in attestations, but it does not forward
  `BURPVALVE_BEAD` through hooks. Hook forwarding is owned by the OXP bead
  above.
- Related plan area: bead closure mode / issue #14.
```

## 7. Show all staged paths in verifier packets and fix generated-prefix hash over-match

Ready-to-file title:

```text
Show all staged paths in verifier packets and narrow generated-path hash exclusions
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; vorch beads already cover the
fix, and this issue exists for tracker visibility.

Body:

```markdown
## Behavior observed

During land-03, verifier packets listed 18 of 20 staged paths. The omitted paths
were scaffold README files under generated evidence directories:

- `backpressure/attestations/README.md`
- `log/backpressure/failed/README.md`

Those paths were omitted because the same generated-prefix logic used for
payload-hash exclusion leaked into packet path listing. The same prefix rule can
also exclude legitimate tracked scaffold files from the payload hash when a
commit intentionally changes those files.

Session evidence: `docs/dogfooding-findings-2026-07.md` A15 records the
omission and the prefix over-match.

## Why it matters

Verifiers need to see every staged path, even if some generated evidence paths
are excluded from the payload hash. Silent omission weakens independent review:
the verifier cannot know whether a path was absent, generated, or hidden by
tooling.

## Suggested fix

Verifier packets should list:

- all staged paths from the staged index;
- hash-bound paths;
- hash-excluded generated paths, clearly labeled with the exclusion reason.

Separately, narrow generated-path hash exclusion so intentionally tracked
scaffold documentation under generated directories is not excluded merely by a
broad prefix match.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A15.
- Covering beads: `burpvalve-vorch-03-verifier-prompts-bound-contract-6vz`
  and `burpvalve-vorch-fix-generated-path-exclusion-overmat-5qn`.
```

## 8. Add a payload ownership accounting helper

Ready-to-file title:

```text
Add a payload ownership helper for staged files and split-file beads
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; this is a follow-up issue
from the planning and polish workflow.

Body:

```markdown
## Behavior observed

The July planning and landing round repeatedly required agents to prove that
every uncommitted path was owned by exactly one remaining bead or a named
exception. That accounting became fragile when one file was intentionally split
across several beads by hunk, function, or test area.

Session evidence: `docs/dogfooding-findings-2026-07.md` A16 records the
payload-accounting friction and the proposed helper.

## Why it matters

Manual payload accounting is slow and easy to get wrong. It is especially hard
when shared files such as `cmd/burpvalve/main.go`, `cmd/burpvalve/commit_test.go`,
or `internal/backpressure/artifacts.go` are split across multiple planned units.
The current process relies on a QA agent reconstructing ownership from bead
prose and current git status.

## Suggested fix

Add a helper such as `burpvalve account payload` or `burpvalve explain staged-owner`
that reports staged and untracked paths against the known work graph. The helper
should show:

- which bead or unit claims each path;
- where a file is intentionally shared across beads;
- generated evidence exceptions;
- paths with no owner;
- paths claimed by more than one incompatible unit.

The command should be useful to orchestrators before assigning work and to
committers immediately before running the gate.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A16.
- Not fully covered by current beads. `burpvalve-oxp-hash-staged-helper-a48`
  partially overlaps by surfacing hash-included and hash-excluded staged paths
  for verifier payload identity, but it does not map staged/untracked paths to
  owning beads or split-file hunks. This issue remains a tracking issue for the
  broader ownership-accounting helper.
- Related plan area: `plans/orchestrator-experience.md` evidence surfaces.
```

## 9. Support supplemental verifiers and adjudication in attestations

Ready-to-file title:

```text
Record supplemental verifiers and adjudication in attestation schema
```

Filed-after-v0.1.2 note: file after v0.1.2 lands; vorch beads already cover the
schema direction, and this issue exists for tracker visibility.

Body:

```markdown
## Behavior observed

During land-03, two verifiers reviewed shared condition cells and an
orchestrator ruling resolved a verdict disagreement. The response and
attestation schema could only carry one verifier per condition, so the second
verifier's corroborating evidence and the ruling lived only in Agent Mail.

Session evidence: `docs/dogfooding-findings-2026-07.md` A17 records the schema
gap and the live dual-verification case.

## Why it matters

If a commit relies on multiple verifier perspectives or an explicit ruling, the
committed attestation should preserve that audit trail. Otherwise the artifact
looks simpler than the process that actually made it trustworthy.

## Suggested fix

Extend the response and attestation schema so each condition can include:

- the primary verifier result;
- a `supplemental_verifiers` array for additional verifier verdicts and
  evidence;
- an optional adjudication or ruling slot with the adjudicator, rationale, and
  transcript reference.

The submit and prompt flows should explain when supplemental verifiers are
allowed, how disagreements are represented, and how older single-verifier
artifacts remain valid.

## Cross-references

- Finding: `docs/dogfooding-findings-2026-07.md` A17.
- Covering beads: `burpvalve-vorch-01-config-schema-fc3`,
  `burpvalve-vorch-03-verifier-prompts-bound-contract-6vz`, and
  `burpvalve-vorch-05-verifier-submit-ingestion-transcr-t1g`.
```
