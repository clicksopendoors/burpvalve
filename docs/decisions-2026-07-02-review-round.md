# Decisions: 2026-07-02 Review Round

Source: orchestrator review of the repo, badges, plans, docs, the 11 open GitHub
issues (#6, #8-#17) including comments, and a full decomposition of the
uncommitted working set sitting on top of the v0.1.1 release commit.

These decisions are binding project direction until a later decision record
supersedes them. Plans in `/plans/` implement them.

## D1. Land the uncommitted working set as atomic, gated commits

The ~6,900-line working set decomposes into ~16 logical units addressing 10 of
the 11 open issues. It must land as a sequence of atomic commits, each passing
through Burpvalve's own commit gate with real verifier evidence. This is the
repo's first genuine dogfooding: as of this decision, `backpressure/attestations/`
has never held a single attestation.

See `plans/land-working-set.md`.

## D2. Tree cleanup before any landing commit

- `.ntm/` is runtime state (rate limits, PIDs, crash reports, logs). It must be
  gitignored and never committed.
- `bin/burpvalve` is **dropped from tracking**. The global-PATH model is the
  default; the repo-local fallback remains an opt-in feature for target repos,
  but this repo does not commit its own binary. (It was tracked at ~8 MB,
  already past the plan's own 8 MiB warning budget.)
- `AGENTS.md` must be filled in: real project purpose, real build/test/lint
  commands (`make build`, `go test ./...`, etc.). The tool that exists to make
  expectations explicit does not ship its own contract as TBD stubs.

## D3. Issue #6 exit-code inversion gets an explicit verification cell

`burpvalve init` in a non-git directory exits 0 while leaving the commit gate
silently inert (no `core.hooksPath`), while the *optional* NTM conflict (#9)
exits 1. The working set claims to fix both. The fix is verified explicitly
against the reproduction in the #6 comment (A/B: non-git dir vs git dir), not
assumed from code review.

## D4. Issue #16 working hypothesis: shell-pasted arrow workflows

The six zero-byte files (`Burpvalve`, `close`, `commit`, `stage`, `sync`,
`verify`) exactly match the tokens after `->` in the workflow line
`finish work -> verify -> close bead -> sync beads -> stage payload + bead
state -> Burpvalve -> commit`. In a shell, each `->` parses as `-` plus a
redirection (`> verify`, `> close`, ...), creating exactly those files.

Consequences:

- Treat agent-facing docs/CLI output containing bare `->` chains as a defect.
  Replace with numbered steps or quoted text.
- The staleness guard in the working set is kept, but it solves a different
  problem than #16 reported. #16 stays open until the hypothesis is confirmed
  or refuted by reproduction.

## D5. Licensing: open-source public launch, later

Burpvalve will be launched publicly as an open-source product **after** the
feature set is complete, working, and tested. Until then:

- No launch LICENSE file is added at this point.
- License selection and the badge/skill-metadata launch flip happens in the launch plan,
  which is explicitly the **last** plan to execute.

See `plans/open-source-launch.md`.

## D6. Next release is v0.1.2

After the working set lands: rebuild `docs/CHANGELOG.md`, bump the static
release badge, package, and release as v0.1.2. The verifier-policy schema
additions are backward compatible (legacy `subagent_confirmed: true` responses
still validate), so this is a patch/minor release, not 0.2.0.

See `plans/release-v0.1.2.md`.

## D7. Issue #12 means standing pre-authorization, never simulated evidence

The #12 comment "simulate an approval ... send a mimic of the subagent
authorization" is clarified to mean: **standing pre-authorization recorded at
init time** (the human authorizes verifier subagent spawning once, durably),
which the gate then honors as policy. It does **not** mean an agent may
fabricate, mimic, or replay authorization or verification evidence when
blocked. Fabricated verification evidence is forbidden, full stop. The
`VerifierPolicy` model in the working set is the mechanism.

## D8. Issues #13 and #14 extensions are in scope for this planning round

- #13 grows into a verifier orchestration plan (spawn profiles, prompt storage,
  subagent thread hygiene, optional runtime config for subagent limits/depth).
  See `plans/verifier-orchestration.md`.
- #14 grows into a bead-closure mode plan (delivery vs admin beads, closure
  gate, uncommitted-work warning). See `plans/bead-closure-mode.md`.

## D9. Goroutine adoption

Production code currently uses zero goroutines. Approved concurrency work, in
priority order:

1. Parallel execution of declared `lint_commands` (bounded, deterministic
   output order, opt-out for cache-contending tools).
2. Parallel setup/inspect probes (git, `br doctor`, `ntm --robot-capabilities`).
3. Concurrent attestation directory scan for `attestations list/latest` once
   evidence volume grows.

All three are scoped inside `plans/issue-17-guided-lint-setup.md` (the lint
wizard is the main beneficiary of scoped parallel commands). Everywhere else
the CLI is a sequential state machine and goroutines would add complexity
without benefit.

## D11. Autonomy by documentation (owner directive, 2026-07-02)

The orchestrator does not wait for owner pre-approval on double-checked,
well-documented work. Waiting for a human to approve a suggestion that was
independently reviewed and would have been approved anyway is the exact
backpressure failure Burpvalve exists to solve. The contract: independent
double-checking (reviews, cross-polish, verifier attestations) plus
documentation sufficient for post-hoc reconstruction and **rollback**
(atomic gated commits are the rollback unit) replaces pre-approval. The
owner vetoes after the fact. Hard exceptions that still stop for the owner:
irreversible or outward-facing actions (publishing, visibility flips,
external sends, spend, deleting releases/history) and explicit
owner-decision beads. This supersedes the pre-approval language in the
original orchestration goal prompt and narrows the D10 escalation posture.

## D12. The orchestrator is a Burpvalve user; orchestrator-facing features are a sanctioned direction

Burpvalve's users are both the agent receiving backpressure (coder) and the
agent relying on its evidence (orchestrator). The orchestrator's frictions
are first-class product signal. Sanctioned direction, seeded by owner
proposals: an orchestrator operating contract distinct from the coder
contract (e.g. `CLAUDE.md` as an orchestrator file rather than a symlink to
`AGENTS.md`, selectable at init), orchestrator prompt banks baked into the
skill or a `prompts` command, and the NTM orchestration patterns proven in
this round documented as part of the Burpvalve ecosystem. See
`plans/orchestrator-experience.md`. Rationale: this agent-orchestrator +
agent-coder pattern is intended to repeat across repos and must work
excellently for the agents themselves.

## D10. Workflow discipline

The standing workflow, in order: first discuss; then write detailed
Markdown plans in `/plans/`; then convert plans into detailed beads; only
then assign work to agents.
A plan does not exist unless it exists in Markdown. A decision does not exist
unless it is recorded in `/docs/`. The 11 open GitHub issues were largely
implemented without ever becoming beads; that drift is what this record and
the current plan set correct.

## OSL owner decisions, 2026-07-03

These owner rulings close the five open-source-launch decision beads. They
record launch direction only. They do not authorize launch execution,
visibility changes, old-release deletion, external issue filing, or any other
outward-facing action.

### D-OSL-1. Public repo home

The public repo home stays `clicksopendoors/burpvalve`.

### D-OSL-2. License

The project license for open-source launch is MIT. The owner chose MIT over
the Apache-2.0 recommendation, so there is no Apache `NOTICE` question to
resolve.

### D-OSL-3. History audit thresholds

The launch history-audit thresholds are stricter than the plan's draft
recommendation:

- Secrets or tokens always block launch.
- Personal data always blocks launch.
- Local machine paths always block launch.
- Legacy pre-Burpvalve naming always blocks launch and must be
  scrubbed.
- Agent names are accepted pending owner veto; they are not a launch blocker
  by default.

### D-OSL-4. Old releases

Deleting the v0.1.0 and v0.1.1 GitHub releases and their assets before any
visibility flip is approved. This decision records approval only; do not delete
the releases or assets until the launch plan reaches that execution step.

Owner reaffirmation, 2026-07-04: old-release deletion belongs in the final
pre-flip execution window, not earlier. The releases and assets stay untouched
until the ordered launch checklist reaches that step.

### D-OSL-5. Launch scope and hard public-launch gate

Launch scope is quiet and README-only.

Hard gate: there is no public launch and no repo visibility flip until the
owner gives explicit permission. This gate supersedes any launch-readiness
state. Downstream `osl:` work remains blocked on the preflip gate until that
explicit permission exists.

### D-OSL-6. Local-only issue bundles

The owner denied filing both issue bundles:

- `docs/upstream-br-feedback-2026-07.md`, the upstream `beads_rust`/`bv`
  feedback bundle.
- `docs/burpvalve-issue-drafts-2026-07.md`, the Burpvalve tracker issue draft
  bundle.

Both drafts stay local-only and informational unless a later owner decision
explicitly authorizes filing.

### D-OSL-7. Public history strategy: squash-relaunch

The public repository will be created from a scrubbed current tree as a fresh
initial commit at visibility-flip time. The source repository keeps its full
history. There is no source-history rewrite, and no historical Burpvalve seal
or attestation binding is rewritten.

Rationale:

- Zero leak surface: old commits, deleted blobs, old release artifacts,
  machine-local paths, legacy project names, and pre-launch metadata do not
  become public history.
- Trivially verifiable: the public initial commit can be audited as a current
  tree snapshot instead of requiring public consumers to trust a complex
  history scrub.
- Historical seal bindings stay valid: historical attestations remain meaningful
  in the source repository because commit IDs and staged-payload bindings are
  not rewritten.
- Provenance loss is retroactive-only: public history starts at the launch
  snapshot, while future public commits are real, gated commits with normal
  provenance from that point forward.

Implications:

- The full-history audit bead's public-history BLOCK verdict is resolved by
  strategy, not by rewriting or publishing the existing source history.
- The remaining launch obligation is a current-tree scrub before the fresh
  public initial commit. Current-tree scrub work must remove launch-blocking
  legacy naming, local paths, stale launch-policy wording, stale demo wording,
  and tracked metadata that would appear in the initial public tree.
- Renaming the legacy Beads ID prefix in tracked Beads data is
  disruptive while launch work is still in flight, so any such prefix scrub is
  the last pre-flip step.
- This decision does not authorize visibility changes, public repo creation, or
  launch execution. All work remains behind D-OSL-5's hard no-launch gate until
  the owner explicitly authorizes the flip.

### D-OSL-8. Public repo name and source repo rename

The public repository takes the canonical name `clicksopendoors/burpvalve`.
At flip-prep time, the current source repository is renamed to
`private-source-repo`, to free the `burpvalve` name. The scrubbed public tree is
then initialized as the fresh `clicksopendoors/burpvalve` repository described
in D-OSL-7. The exact source-repo rename target was approved by the owner on
2026-07-04.

The source-repo rename is a flip-time action behind the hard no-launch gate.
Do not rename this repository, create the public repository, or change
visibility until the owner explicitly authorizes the flip.

### D-OSL-9. Git authorship and AI co-authorship

All commits use the sole author identity `clicksopendoors
<michael-bltzr@users.noreply.github.com>`. Do not add Claude, Codex, or other
AI-tool co-authorship. Do not include `Co-Authored-By` trailers for AI tools in
commit messages.

Implementation requirements:

- This repository's local git config is set to `user.name=clicksopendoors` and
  `user.email=michael-bltzr@users.noreply.github.com`.
- `AGENTS.md` and the symlinked `CLAUDE.md` record the no-AI-coauthor rule for
  agents working in this repository.
- A follow-up scaffold-template bead must add the same instruction to generated
  Burpvalve operating contracts, including `AGENTS.md.tmpl`,
  `ORCHESTRATOR.md.tmpl`, and generated Claude orchestrator content.
- Existing history was checked for AI `Co-Authored-By` trailers and found
  clean; keep it that way.

### D-OSL-10. Public fixture email

Test fixtures that currently use the legacy placeholder email should be
scrubbed to use the account's anonymous GitHub email,
`michael-bltzr@users.noreply.github.com`, as part of the current-tree scrub.
This is a public-tree consistency rule, not a standalone launch authorization.

### D-OSL-11. Public GitHub About metadata

The public repository About metadata is approved as follows:

- Description: `Repo-local backpressure for agentic development workflows.`
- Topics: `go`, `cli`, `developer-tools`, `git-hooks`, `ai-agents`,
  `verification`, `backpressure`.
- Homepage: blank unless the owner later supplies a real public URL.

This records the text to apply during the flip checklist. It does not authorize
changing repository visibility or creating the public repository.

### D-OSL-12. Public GitHub issue surface

GitHub issue #18 may mention the non-public `clicksopendoors/lingo` fork and the
agent workflow context that produced the report. The lingo fork is a
personal language-learning project, not sensitive infrastructure. Public
references to Beads, NTM, Agent Mail, Codex sessions, verifier workflows,
blocked-report counts, and `--no-verify` pressure are acceptable when they help
explain the product problem.

The launch blocker is security-sensitive detail, not ordinary workflow context.
Before the visibility flip, issue #18 and the other open GitHub issues must be
scrubbed for secrets, tokens, credentials, IP addresses, hostnames, non-public
network or local-infrastructure details beyond normal Linux/OS/kernel context,
and local absolute paths that disclose machine layout. Linux kernel and OS
details are acceptable unless paired with sensitive infrastructure data.

Plain-language rule: do not hide the story of how Burpvalve is used; only
remove details that create a security threat vector or expose local
infrastructure unnecessarily.

### D-OSL-13. Public Beads policy

The owner prefers keeping Beads data in the public tree, provided it is scrubbed
enough to be boring. "Boring" means the public `.beads/` data can show normal
project task tracking, including agent workflow and scaffolding context, but
not security-sensitive infrastructure detail.

The public Beads scrub must remove or neutralize launch-blocking material:
local machine paths, personal data, secrets or credentials, IP addresses,
hostnames, non-public network or local-infrastructure details, legacy
pre-Burpvalve IDs, and any stale evidence that would confuse a public
reader. References to the lingo fork, Beads, NTM, Agent Mail, and agent
coordination are accepted unless a specific reference exposes a security threat
vector. Agent names remain accepted unless the owner later vetoes them.

The exact replacement bead-ID prefix is still a final pre-flip scrub input.
Perform that rename late, after all other launch work is stable.

## Lexicon owner decisions, 2026-07-03

These rulings come from the owner's 2026-07-03 lexicon directive and the
approved `plans/lexicon.md` Draft 2. They define product vocabulary for
README, help text, prompt-bank wording, skill wording, and future launch scrub
work. They do not authorize command behavior changes, schema changes, stable
path renames, launch scrub, history rewrite, license changes, or a public
visibility flip.

### D-LEX-1. Backpressure remains the anchor concept

Burpvalve's pitch-level explanations must still name backpressure as the reason
the product exists. Friendly terms may clarify the system, but they do not
replace backpressure as the core concept.

### D-LEX-2. Burp is the refusal noun and verb

A gate refusal is a burp. Saying the valve burped a commit back means the
commit was refused. Use this wording only for real gate refusals; do not use it
for setup failures, config validation failures, lint `not_enforced`, command
usage errors, unavailable tools, panics, crashes, or internal errors.

### D-LEX-3. Seal is an attestation

A seal is the evidence artifact written by Burpvalve, and it is explicitly a
type of attestation. Seal and attestation are usable interchangeably in prose.
Do not erase attestation, and do not rename `backpressure/attestations/` or
attestation schema fields merely to match the friendly word.

### D-LEX-4. Valve is the friendly gate name

The valve is the commit gate. Phrases such as "the valve is fail-closed" are
accepted when they clarify gate behavior. Command names remain stable.

### D-LEX-5. Work unit is the product concept

A work unit is the atomic piece of work being checked. Bead is Beads/`br` tool
vocabulary, not the generic product concept. A bead can be one tracker-backed
work unit. The existing `feature` vocabulary remains the stable CLI/schema
binding term for compatibility-bearing surfaces such as `--feature`, feature
JSON fields, Go `Feature` structs, response/template fields, `feature_name`,
and feature/diff-cluster binding terminology.

### D-LEX-6. Glossary, not command

The glossary lives in README and `docs/vocabulary.md`. Burpvalve will not add
an `explain-terms`, `glossary`, or similar command for this vocabulary. Human
help that uses a lexicon term should include a short inline definition.

### D-LEX-7. Adopt vocabulary where useful, not obsessively

New surfaces should use the lexicon where it improves clarity. Existing
surfaces change only where the change is cheap, clear, and low-risk. Stable
paths, schema fields, flags, persisted evidence names, prompt IDs, command
names, and formal machine-readable contracts are not renamed merely to match
the vocabulary.

### D-LEX-8. Do not estimate development time or timelines

Agents must never estimate or reference development time or timelines. Examples
such as "two weeks", "five minutes", and "by end of day" are forbidden. Agents
should describe scope, sequencing, dependencies, blockers, and readiness state
instead.

This is an absolute conduct rule because agent estimates in this area are not
reliable enough to publish, plan around, or use for orchestration. The rule must
be carried into this repository's operating contract and into Burpvalve
scaffold templates so initialized repositories inherit it.

## OSL owner decisions, 2026-07-04 (launch-input round)

These owner rulings close the remaining launch-input questions. They authorize
launch preparation only. The visibility flip, public repo creation, and
source-repo rename remain behind the D-OSL-5 hard gate and require separate
explicit owner authorization.

### D-OSL-14. Feature-complete declaration and launch-prep authorization

The owner declares Burpvalve feature-complete for the open-source launch. All
eight deferred P4 backlog beads (vorch-08/09/10, bcm-07/08, the two lint
deferrals, and the oxp option-A truth table) stay post-launch backlog; none is
pulled back before launch. Launch preparation is authorized to proceed
in order: GitHub-surfaces audit, old-release handling, preflip gate, launch
release. This does not authorize the visibility flip.

### D-OSL-15. Replacement bead-ID prefix

The replacement for the legacy pre-Burpvalve bead-ID prefix is
`burpvalve`. Per D-OSL-7/D-OSL-13, the prefix rename remains the last pre-flip
scrub step, performed only after all other launch-prep work is stable.

### D-OSL-16. Launch version

The public launch release version is `v0.2.0`. This signals a meaningful step
past the pre-launch v0.1.x releases without a 1.0 stability promise.

### D-OSL-17. Security reporting channel

SECURITY.md names GitHub Private Vulnerability Reporting as the channel. The
current source-repo HTTP 404 from the PVR API remains a pre-launch deferral.
At flip time, enabling and verifying PVR on the public repository is a required
runbook step, fixed forward immediately if broken. No personal security email
fallback is published.

### D-OSL-18. Local-path historical issue comments

The owner does not waive local-path residue in historical GitHub issue
comments. Comments containing local absolute paths that disclose machine layout
must be edited or deleted before the flip, consistent with D-OSL-12.

### D-OSL-19. Launch-scope amendment: orchestrator-context templates

The owner amends D-OSL-14: the four orchestrator-experience template beads
(burpvalve-nc0z C5 polling standard, burpvalve-fg5l
contact-mesh pre-approval, burpvalve-j97h Agent Mail registration
mandate at dispatch, burpvalve-lzze orchestrator no-self-execution
role split) join the launch scope and must land before the preflip gate. The
first public release ships the full pre-built orchestrator context so
first-time Burpvalve users get an orchestrator that already knows the
documented polling, contact-mesh, registration, and role-split rules. No other
feature work is reopened by this amendment.

### D-OSL-20. Flip execution window authorized

On 2026-07-05 the owner explicitly authorized both remaining launch stages:

1. Immediate current-tree scrub under D-OSL-14 launch-prep authorization:
   legacy pre-Burpvalve naming, local-path residue, fixture emails
   (per D-OSL-10), and stale launch-policy wording across the tracked files
   enumerated in the preflip-gate residue checklist. The Beads ID prefix
   rename remains the last pre-flip scrub step per D-OSL-13/D-OSL-15.
2. The flip execution window itself, superseding the D-OSL-5 hold: old
   release deletion (D-OSL-4), prefix rename plus the 19 deferred issue
   reference edits, v0.2.0 launch release (D-OSL-16), source repo
   rename to private-source-repo and fresh-history public repo creation
   (D-OSL-7/8), About metadata apply (D-OSL-11), PVR enablement and
   verification (D-OSL-17), visibility flip, and the anonymous verification
   suite, executed strictly in the recorded order via the flip-day runbook
   and the ordered launch chain (8mt, preflip gate 557, release juq, flip
   3ab, umbrella dla).

Execution remains fail-closed: any blocker, verifier disagreement, or
unpredicted state stops the sequence and escalates to the owner rather than
improvising forward.

### D-OSL-21. Launch-window gate relaxation with compensating re-verification

Owner ruling, 2026-07-05: for the launch execution window only, every
condition in `backpressure/manifest.yaml` is set to
`verifier_policy: main_agent_allowed` so remaining mechanical launch commits
self-attest through the gate with truthful `main_agent` provenance instead of
independent pane rounds. This is honest risk acceptance for non-critical
mechanical work, not evidence fabrication: attestations continue to be
written and continue to name who verified.

Compensating controls, tracked in bead `burpvalve-mfuc`:

1. Immediately after the flip and anonymous verification, the manifest
   reverts to independent verification.
2. Every launch-window commit then receives retroactive independent
   codex-pane verification, recorded as supplemental evidence; findings are
   fixed forward publicly.

The public repository therefore ships the strict configuration.

### D-OSL-22. v0.3.0 public snapshot scope and evidence boundary

Owner-directed release task, 2026-07-09: publish the week's product work as
v0.3.0 in the public repository. The public snapshot includes the source and
documentation for `burpvalve gate run`, first-class lane commits, PXPACK
orchestrator context packets, the orchestrator toolbox and poll scripts, Spark
gate-operator ritual templates, and the installer skills-directory/copy fix.

The snapshot boundary follows D-OSL-7. The public repository receives the
product tree, docs, scrubbed tracker state, and its own public commit evidence
line. It does not receive private-source commit attestations or private
backpressure evidence artifacts. Private attestations remain in the source
repository because they bind to private commit ids, local verifier rounds, and
operational context that is not part of the public history.

This decision authorizes the v0.3.0 public snapshot and docs update only. It
does not authorize a GitHub release tag or binary asset publication unless the
owner asks for that separate release step.
