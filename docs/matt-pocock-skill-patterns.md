# Matt Pocock Skill Patterns For Burpvalve

This note captures patterns from `mattpocock-skills` that are useful for
Burpvalve. The goal is not to copy those skills wholesale. The goal is to
translate their strongest concepts into Burpvalve's existing model: repo-local
contracts, backpressure conditions, attestations, deterministic commands, and
agent-safe CLI flows.

## Highest-Value Transfers

### 1. Router Flow: `ask-matt`

Matt's `ask-matt` skill works because it is a router over many workflows. Users
do not need to remember every command; they can ask one command which path fits.

Burpvalve already has many related commands:

- `setup`
- `init`
- `repair`
- `commit`
- `lint`
- `ci`
- `config`
- `completion`

Useful Burpvalve translation:

- Add or improve a "what should I run?" decision table in the skill and README.
- Consider a future `burpvalve guide` or `burpvalve doctor` command that routes
  users to setup, repair, lint, commit, or CI based on current repo state.
- Keep the command model split by intent: inspect, mutate, verify, publish.

Why it matters: Burpvalve is a coordination product. Routing confusion creates
bad usage before the actual gate ever runs.

### 2. Grilling Plus Domain Modeling

Matt's grilling pattern forces one unresolved branch at a time. The domain
modeling pattern captures precise vocabulary and durable decisions while the
conversation is still fresh.

Burpvalve translation:

- In human setup flows, ask one decision at a time instead of dumping every
  option.
- When configuring backpressure, inspect the repo first and answer what can be
  answered from files before asking the user.
- Use canonical terms consistently: condition, feature, cell, attestation,
  staged payload, manifest, lint command, blocked attempt.
- When users define project-specific pressure, write it directly into
  `backpressure/*.md` or `backpressure/manifest.yaml`, not only into chat.

Useful product idea:

- Add a domain glossary for Burpvalve docs, likely `docs/GLOSSARY.md` or
  `CONTEXT.md`, so agents use the same language across skill, README, CLI help,
  attestation schema, and backpressure docs.

### 3. Tight Feedback Loops: `diagnosing-bugs`

The debugging skill's core idea is that work starts by creating a tight,
red-capable loop. No loop, no real diagnosis.

Burpvalve translation:

- A backpressure condition is strongest when it has a concrete command or
  artifact that can go red.
- Treat prose-only conditions as the first rung, not the final state.
- Make `backpressure/manifest.yaml` the place where tight loops become
  executable.
- When a condition cannot be made executable, require honest attestation about
  the limitation instead of pretending it is deterministic.

This reinforces the existing Burpvalve ladder:

1. Written rule
2. Attestation prompt
3. Executable command
4. CI gate
5. Structural invariant

Potential improvement:

- Add "red-capable" language to `skill/burpvalve/references/deterministic-backpressure.md`.
- When proposing new `lint_commands`, ask: "What exact failure should this
  command catch?"

### 4. Tracer Bullets And Vertical Slices: `tdd`, `to-prd`, `to-issues`

Matt's TDD and issue-slicing guidance rejects horizontal work like "write all
tests, then all code." It prefers one thin vertical slice at a time.

Burpvalve translation:

- Future Burpvalve features should be sliced vertically across:
  - CLI flag or command behavior;
  - internal package behavior;
  - template or scaffold output;
  - docs/help text;
  - tests;
  - release/package implications when relevant.
- Avoid beads that are only "update all docs" or "write all tests" unless they
  are cleanup tasks after vertical behavior exists.
- A completed slice should be demoable with one command.

Example slice shape:

1. Add ecosystem-detected Go lint proposal.
2. Inspect detects `go.mod`.
3. Setup JSON reports candidate command.
4. Init can write confirmed `lint_command`.
5. Docs explain the behavior.
6. Tests cover the full path.

### 5. Deep Modules And Seams: `codebase-design`

Matt's architecture vocabulary is useful for Burpvalve because the tool already
has clear module candidates:

- CLI command tree
- scaffold inspection and mutation
- manifest and condition planning
- attestations
- lint command execution
- config loading
- NTM bridge

Burpvalve translation:

- Keep these modules deep: small public interfaces, meaningful behavior behind
  them.
- Use the module interface as the test surface.
- Apply the deletion test: if deleting a helper just moves trivial code
  elsewhere, it was not earning its existence.
- Prefer adapters only when at least two implementations exist or are imminent.

Potential improvement:

- Add an architecture-maintenance checklist to `docs/ARCHITECTURE.md` or a
  separate design note:
  - Is this package deep?
  - Is this interface the test surface?
  - Does complexity gain locality?
  - Is a seam real or hypothetical?

### 6. Two-Axis Review: `review`

Matt's in-progress review skill separates two questions:

- Standards: does the code follow documented project rules?
- Spec: does the code implement the requested behavior?

Burpvalve translation:

- Do not let a passing linter imply the feature is correct.
- Do not let a spec-passing implementation excuse violations of repo policy.
- Preserve this separation in verifier prompts and future reviewer-agent support.

Potential product idea:

- Model verifier evidence as separate lanes:
  - condition compliance evidence;
  - feature/spec evidence;
  - optional standards evidence.

This would make attestations clearer and reduce overloading one "pass" verdict.

### 7. Triage State Machine

Matt's triage skill moves incoming issues through explicit roles:

- needs triage;
- needs info;
- ready for agent;
- ready for human;
- wontfix.

Burpvalve translation:

- Backpressure conditions can have a lifecycle:
  - wishlist;
  - attestation-only;
  - executable command;
  - CI gate;
  - structural invariant;
  - retired.
- Condition changes should have explicit state, not just prose edits.

Potential product idea:

- Add optional metadata to condition files or the manifest to record condition
  maturity.
- Report condition maturity in `burpvalve setup --json`.

### 8. Prototype As Throwaway Question-Answering

Matt's prototype skill treats prototypes as throwaway code that answers one
question and then gets deleted or absorbed.

Burpvalve translation:

- Use small prototypes for tricky terminal flows before committing to Bubble Tea
  implementation.
- Prototype matrix prompts, setup wizard state, and verifier UX as isolated
  terminal examples.
- Record the answer in docs, tests, or an ADR-like note; do not let prototype
  code become production by accident.

Good fit areas:

- verifier matrix interaction;
- init wizard option grouping;
- setup/doctor summary output;
- recovery flow after blocked commit attempts.

### 9. Handoff And Context Hygiene

Matt's handoff skill captures just enough context for a fresh agent, without
duplicating artifacts that already exist.

Burpvalve translation:

- Blocked-attempt reports should point to relevant files and artifacts instead
  of copying everything.
- A future `burpvalve commit` blocked report could include "next-session hints":
  - staged payload summary;
  - failed condition cells;
  - exact commands to rerun;
  - artifacts to inspect;
  - suggested next action.

This matches Burpvalve's existing `log/backpressure/failed/` idea.

### 10. Teaching Workspace

Matt's `teach` skill is useful because it treats learning as stateful. It keeps
mission, resources, lessons, reference material, and learning records.

Burpvalve translation:

- Burpvalve adoption needs teaching artifacts, not only reference docs.
- Consider a tutorial path for new users:
  - what backpressure is;
  - how setup works;
  - why attestations bind to staged payloads;
  - how to recover from a blocked commit;
  - how to make a condition more deterministic.

Potential product idea:

- Add `docs/tutorials/` or `docs/lessons/` with short, task-shaped walkthroughs.
- Keep reference docs separate from lessons. Reference is for lookup; lessons
  are for first-time learning.

### 11. Predictable Skill Design: `writing-great-skills`

This pattern applies directly to the packaged Burpvalve skill.

Useful concepts:

- Predictability: the skill should make agents follow the same process every
  time, not merely sound helpful.
- Description discipline: trigger wording should name distinct branches without
  synonym padding.
- Information hierarchy: `SKILL.md` should route; references should hold deep
  detail.
- Completion criteria: each workflow should tell the agent when it is done.
- No-op pruning: remove lines that do not change behavior.

Burpvalve translation:

- Keep `skill/burpvalve/SKILL.md` as the short operational router.
- Keep install detail in `INSTALL.md`.
- Keep deterministic-pressure guidance in
  `references/deterministic-backpressure.md`.
- Add completion criteria where workflows still end vaguely.
- Periodically prune repeated explanation across README, skill, and docs.

### 12. Git Guardrails

Matt's git guardrails skill blocks dangerous commands before the agent can run
them.

Burpvalve translation:

- Burpvalve already has a `destructive-operations` condition. That is the right
  conceptual home.
- The stronger version is deterministic enforcement:
  - protected command wrappers;
  - approval tokens;
  - dry-run evidence;
  - explicit destructive-operation artifacts.

Do not quietly install editor-specific hooks. Offer them as optional pressure
when the user wants stronger safety.

## Recommended Implementation Candidates

### Candidate A: Burpvalve Guide Router

Add a command or skill section that routes users to the right workflow.

Small version:

- Add a decision table to `skill/burpvalve/SKILL.md` and README.

Larger version:

- Add `burpvalve guide` or expand `burpvalve setup` output with next-action
  recommendations.

### Candidate B: Condition Maturity Lifecycle

Make the backpressure ladder a first-class lifecycle.

Possible states:

- `wishlist`
- `attestation`
- `command`
- `ci`
- `structural`
- `retired`

This would make `setup --json` and docs more honest about what is enforceable.

### Candidate C: Ecosystem-Detected Lint Command Proposals

Use the already-documented setup-pre-commit pattern:

- detect repo ecosystem;
- propose exact `lint_commands`;
- ask before writing;
- never add fake commands.

This is the most immediately actionable product improvement.

### Candidate D: Red-Capable Condition Guidance

Extend deterministic-backpressure docs and verifier language with the debugging
idea that every executable condition should say what failure it can catch.

This keeps commands from becoming ritual checks with unclear value.

### Candidate E: Tutorial/Lesson Docs

Add short lessons for first-time users and agents:

- "Your first blocked commit"
- "Turning a prose condition into a command"
- "Why changing staged files invalidates evidence"
- "Adding a repo-local fallback binary"

This borrows `teach` without adding teaching machinery to the CLI.

## What Not To Copy

- Do not copy Matt's slash-command structure wholesale.
- Do not add wrappers that only compose two other workflows unless they solve a
  real user-memory problem.
- Do not make Burpvalve ask many setup questions at once.
- Do not let prose policies masquerade as deterministic gates.
- Do not merge all docs into one giant skill file.
- Do not auto-install editor-specific hooks or language dependencies without
  explicit user approval.

## Bottom Line

The most useful Matt Pocock patterns for Burpvalve are:

1. a router flow for choosing the right workflow;
2. one-question-at-a-time setup and clarification;
3. immediate capture of domain terms and durable decisions;
4. tight, red-capable feedback loops;
5. vertical feature slices;
6. deep modules with testable seams;
7. separated standards/spec review;
8. condition lifecycle state;
9. throwaway prototypes for terminal interactions;
10. stateful onboarding lessons;
11. predictable skill design and progressive disclosure.

The unifying idea is the same as Burpvalve's core thesis: agents behave better
when the workflow is structured, stateful, checkable, and hard to fake.
