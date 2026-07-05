# Orchestrator Experience: Burpvalve For The Agent Who Assigns The Work

## Status

Revision 3, 2026-07-02. Reviews 1 and 2 complete (ScarletMarsh message
2296; WhiteGorge message 2300; both "needs revision" with converging,
mechanical remainders). Revision 3 resolves every remaining item in the
"Revision 3 Resolutions" section below, which is BINDING and overrides any
looser wording elsewhere in this document. Pending a delta-check by
reviewer 2, then bead conversion. Scheduled AFTER the current round's
implementation plans. Findings A1-A15 and C1-C10 are the evidence base.

## Revision 3 Resolutions (binding)

1. **Option A is deferred wholesale.** It is removed from this plan's
   increments entirely and becomes an explicitly-deferred backlog bead
   (p4) at conversion time. No migration truth table is designed now; the
   deferred bead records that the truth table (review-2 ambiguity 3's nine
   cases) is the first deliverable if ever activated. This plan ships
   option B only.
2. **Option B installation rule (exact):** the orchestrator target is NOT
   added to `standardScaffoldTargets`. It is created only when one of:
   (a) explicit positional target (`burpvalve init orchestrator`);
   (b) config `defaults.init.orchestrator` set to a non-off value;
   (c) the init TUI checkbox, which is OFFERED (visible, unchecked by
   default) whenever the Beads or NTM checkbox is selected in the same
   run. Never installed silently.
3. **Config schema (exact):** new string-enum field
   `defaults.init.orchestrator` with values `"off" | "orchestrator-md" |
   "claude-md"` (default `"off"`; `"claude-md"` is rejected with a
   not-yet-supported error until the deferred option-A bead ships), plus
   bool `defaults.repair.orchestrator` (repair touches the file only when
   true or when explicitly targeted). Extends `internal/config`
   Merge/Normalize/Validate and `noteDefaultsSources` for a non-bool
   field — schema work, named as such.
4. **Prompt-bank command contract (exact):** prompt names are a stable
   public API (renames are breaking changes). `prompts show <name>
   [--var key=value ...]`: missing required variables is a usage error
   listing them; `--json` emits `{name, version, variables:
   [{name, required, description}], body}`. `--write` exports to
   `docs/prompts/<name>.md` with a metadata header (burpvalve version,
   prompt name, content hash); it refuses to overwrite an export whose
   content hash shows local modification unless `--force`; `repair` never
   touches `docs/prompts/`. `prompts show` warns when a differing local
   export exists. Rendering is tested with hostile variable values.
5. **Bank addition:** `verifier-bootstrap` (read AGENTS.md +
   backpressure/README.md, register via macro_start_session,
   contact-request the orchestrator, poll inbox, packet-priority rule,
   verdict schema pointer) — review 2 confirmed this was the missing
   arrival prompt.
6. **`ci --commit` CLI contract:** new `--commit <sha>` flag on `ci`,
   `robotCIInput` extended to accept it, help + robot docs updated, and
   JSON output includes artifact path(s) and per-condition provenance
   (satisfying the audit-discovery need without a separate command for
   now). With `--commit`, feature identity defaults to artifact-bound
   discovery from that commit; `--feature` becomes an extra assertion that
   must match, not a selector.
7. **`BURPVALVE_BEAD` parse rules:** comma-separated; tokens trimmed;
   empty tokens rejected with a named error; duplicates deduplicated with
   a warning; commas are not permitted inside bead IDs (validated).
   Mapping to repeated `--bead` flags is covered by shell-level hook
   tests in both template copies.
8. **Drift guard is structural, not grep:** `ORCHESTRATOR.md` must link to
   the `AGENTS.md` gate/Definition-of-Done sections and MUST NOT contain
   its own copies of gate rules, DoD bullets, or verdict vocabulary except
   by reference; the template contract test asserts the forbidden section
   headings are absent, not merely that strings differ.

## Source

Owner direction (2026-07-02): Burpvalve is designed for agent usage in a
repeated two-role pattern — agent as orchestrator, agent as coder — with the
human out of the proposer/approver loop except for irreversible actions.
The orchestrator is a Burpvalve user; if it works for the orchestrator, it
works for the owner. Owner-proposed candidates: orchestrator prompts baked
into Burpvalve (skill additions or a `prompts` command), an
`orchestrator.md`-style contract, and possibly `CLAUDE.md` as the
orchestrator contract (selected at init) instead of a symlink to
`AGENTS.md`, since the Claude agent is consistently the orchestrator.

## Problem

Burpvalve currently addresses one agent role: the committing coder. The
orchestrator role — assigning beads, auditing attestations, running
verifier relays, unblocking gates, rolling back bad work — has no product
surface. Everything the orchestrator needed this round was improvised:
marching-order prompts, verifier rules-of-engagement, review-dispatch
templates, tick discipline, pane-identity checks, findings logging. That
improvisation worked, but it lives in one session's context and a findings
doc instead of in the tool. For a pattern intended to repeat across many
repos, the orchestrator's half of the ecosystem must be as installable as
the coder's half.

## Goals

- A repo scaffolded by Burpvalve gives an incoming orchestrator agent a
  contract and prompt bank as concrete as what `AGENTS.md` gives coders.
- The orchestrator can audit evidence fast: commit-scoped attestation
  checking, attestation queries by bead/feature, drift checks.
- The proven NTM patterns (peer-pane verification, verifier packets over
  Agent Mail, per-pane monitoring, pane-identity verification) are
  documented product guidance, not session folklore.
- Everything stays honest to the ecosystem split: Burpvalve provides
  contracts, prompts, and evidence surfaces; NTM/Agent Mail/beads remain
  the coordination substrate.

## Design Candidates (to be resolved in review)

### 1. The orchestrator contract file

Options, one to be chosen during review:

- **A. `CLAUDE.md` as orchestrator contract (owner's proposal).** At init,
  a new choice: instead of `CLAUDE.md -> AGENTS.md` symlink, write a real
  `CLAUDE.md` containing the orchestrator operating contract (mission
  pattern, tick loop, gate rules, issue protocol, autonomy-by-documentation
  contract) while `AGENTS.md` remains the coder contract. Rationale: the
  Claude agent is consistently the orchestrator in this pattern; the file
  Claude reads should say orchestrator things. Cost: breaks the
  one-canonical-contract simplicity; repos where Claude is a coder need the
  symlink mode; drift risk between the two files for shared rules.
- **B. `ORCHESTRATOR.md` alongside the symlink.** Keep
  `CLAUDE.md -> AGENTS.md`; add a separate `ORCHESTRATOR.md` that any
  orchestrating agent (Claude or otherwise) is told to read. `AGENTS.md`
  gains one line pointing orchestrators at it. Cost: one more file agents
  must know to read; benefit: role-based rather than vendor-based, keeps
  shared rules in one place.
- **C. Orchestrator section inside `AGENTS.md`.** Cheapest; risks bloating
  the coder contract with rules that don't apply to coders.

**Resolved (revision 2, per review 1): option B is the default; option A is
explicit opt-in only, never the default; option C is reduced to a one-line
pointer in `AGENTS.md` when the orchestrator file is active.**

Review 1 established that option A is NOT just a template choice — the
current scaffold treats `CLAUDE.md` as a symlink target everywhere:
`internal/scaffold/targets.go` (`TargetClaude` + alias), `apply.go`
(`ensureClaudeLink` treats a regular `CLAUDE.md` as a conflict), `repair.go`
(`repairClaudeLink` imports regular-file content into `AGENTS.md` and
replaces it with the symlink), `inspect.go` (regular file = conflict), plus
tests asserting all of that. Option A therefore requires: new
inspect/apply/repair semantics for an "orchestrator mode" `CLAUDE.md`
regular file, a migration/repair truth table covering each starting state
(existing symlink: convert or keep? existing regular coder-content file:
import to AGENTS first, then write orchestrator content? orchestrator-mode
file present but stale: append missing sections like AGENTS repair does?),
conflict-language updates,
TUI/robot flow updates, and test rewrites. That work is budgeted as its own
increment and only ships if the opt-in is actually wanted after B exists.

Option B scope (explicit, from review 1): new scaffold target + aliases
(`orchestrator`, `ORCHESTRATOR.md`), `ApplyOptions` + inspect check +
repair support + template contract tests, the `AGENTS.md` pointer line
rendered only when the target is active, and setup/init/repair help +
robot docs. Config: `init.orchestrator` is an enum, which does NOT fit the
current bool-only `ScaffoldDefaults` — this is schema work (validation,
merge, source reporting), called out as such, not a skip flag.

Applicability default (review ambiguity 1): the orchestrator file is NOT
part of the plain default scaffold; init offers it when NTM or Beads
integration is selected (the signals that a repo runs the multi-agent
pattern), and always via explicit target `burpvalve init orchestrator`.

Drift guard (review risk 2): shared gate rules live in `AGENTS.md` only;
`ORCHESTRATOR.md` references them and adds only orchestrator duties (tick
loop, audit, rollback, autonomy contract). A template contract test greps
that core gate language is not duplicated across the two templates.

### 2. `burpvalve prompts` — the prompt bank

A read-only command surfacing canonical prompts, so orchestrators and
coders cite instead of improvising:

```text
burpvalve prompts list
burpvalve prompts show commit-choreography     # finding A1's explainer
burpvalve prompts show verifier-brief          # rules-of-engagement for a verifier agent
burpvalve prompts show verifier-packet-relay   # peer-pane relay instructions
burpvalve prompts show marching-orders         # coder start-of-session brief
burpvalve prompts show orchestrator-tick       # the observe/assign/verify/unblock/log loop
burpvalve prompts show bead-conversion         # plan-to-beads standard
```

- Bank additions from review 1 (all were real repeated orchestrator moves
  this round, not speculation): `plan-review-packet`,
  `cross-review-polish`, `verifier-standby-brief`,
  `packet-not-received-status`, `bead-conversion-assignment`.
- **Storage contract (resolved, revision 2): prompts are embedded in the
  binary** (versioned with the release, consistent across repos), rendered
  with variables (bead id, session name, verifier name) via flags, `--json`
  for robots. `--write` exports to `docs/prompts/` (scaffold-adjacent,
  repair-aware); every rendered/exported prompt carries source metadata
  (burpvalve version + prompt name + "local copy" marker on exports), and
  `prompts show` warns when a local export exists and differs — the bank
  must not recreate the drift it solves (review risk 4).
- Namespacing (review ambiguity 2): the bank lives at `burpvalve prompts`;
  help text explicitly distinguishes it from `burpvalve verifier prompts`
  (canonical templates vs packets generated from the current staged
  payload).
- Prompts embed the paste-safety rules (D4) by construction, and rendering
  is TESTED with hostile variable values (spaces, quotes, backticks,
  `$status`-style reserved words) — static template checks are not enough
  (review risk 5).
- The skill references the bank instead of restating workflows, BUT
  retains enough bootstrap text to guide an agent that has the skill and
  not yet the binary (first-install window; review gap 7 — acceptance
  criterion below).

### 3. Orchestrator evidence surfaces

Mined from this round's frictions, most already logged as findings:

- `burpvalve ci --commit <sha>` — commit-scoped attestation validation
  regardless of tree state (finding A11). The single highest-value item.
  **Implementation boundary (revision 2, from review 1):** current
  `internal/backpressure/ci.go` validates staged state or falls back to
  the latest commit only, and `GitCommittedReader` is hard-coded to
  `HEAD^`/`HEAD` + `git show HEAD:<path>`. The feature requires a
  commit-parametrized reader (`<sha>^..<sha>` diff, `git show <sha>:<path>`
  content), explicit root-commit handling, a stated merge-commit parent
  policy (first parent), and artifact lookup from that commit's tree — not
  a flag on the existing path.
- Companion discovery surface (review gap 6): auditing needs "what
  evidence does this commit carry," not only pass/fail — either
  `attestations list --commit <sha>` or a guarantee that
  `ci --commit --json` returns artifact paths + provenance sufficient for
  audit.
- `burpvalve hash --staged` so verifiers independently confirm the payload
  they verified (finding A9). **Ownership (revision 2):** vorch-03 already
  requires packets to carry a reproduction rule or exact helper command;
  THIS plan owns the helper command only if vorch has not landed it first —
  the increment says "absorb already-landed fixes by documenting them; do
  not duplicate" (review ambiguity 3, applied to every evidence-surface
  item).
- `BURPVALVE_BEAD` forwarding (finding A12). **Plumbing spec (revision 2):**
  both hook template copies (`templates/githooks/pre-commit` and
  `internal/scaffold/templates/githooks/pre-commit`) plus this repo's live
  hook; the variable supports MULTIPLE bead IDs, comma-separated, mapped to
  repeated `--bead` flags; multiple IDs without `BURPVALVE_BEAD_RATIONALE`
  fail exactly as `commit` fails today without `--bead-rationale`; tests
  prove the env-to-flags mapping.
- Hook-context-aware `next_steps` (finding A6).
- `attestations list --feature/--bead` already landed; document them as the
  orchestrator's audit query surface.
- Audit-trail boundary (review ambiguity 5): Agent Mail is the orchestrator
  audit trail; orchestrator messages are never evidence for feature x
  condition cells unless a verifier policy explicitly names that
  actor/kind. Stated in the orchestrator contract template.

### 4. NTM patterns as documented guidance

Extend `docs/ntm-bridge.md` (and the skill) with the patterns proven here,
replacing its blanket discouragement of per-cell NTM fanout with the
nuanced position the owner holds: peer-pane verification is a first-class
independent-verifier pattern (D12, finding A8), with the known costs and
the pane-identity/monitoring disciplines (findings C7, C10) stated.

## Non-Goals

- No workflow engine inside Burpvalve: it does not spawn panes, send mail,
  or monitor sessions. NTM owns coordination; Burpvalve owns contracts,
  prompts, and evidence.
- No new enforcement claims: prompt banks and contracts are guidance
  surfaces; the gate remains the only refusal surface.
- Not part of the current round's completion definition; `osl:` launch does
  not depend on it.

## Increments (provisional until review)

1. Decide contract-file design (grill options A/B/C); implement chosen init
   scaffold + config key + repair support.
2. `burpvalve prompts` with the initial bank (commit-choreography,
   verifier-brief, marching-orders, orchestrator-tick).
3. Evidence surfaces: `ci --commit`, `hash --staged`, `BURPVALVE_BEAD`
   forwarding (each maps to an existing finding; some may instead land as
   standalone issue fixes before this plan — that is fine, the plan absorbs
   whatever remains).
4. NTM pattern documentation pass (`docs/ntm-bridge.md`, skill references).
5. Skill refactor to cite the prompt bank instead of restating workflows.

## Acceptance Criteria (draft)

- A fresh repo + one init run gives an orchestrator agent: a contract file
  telling it the tick loop, gate rules, and autonomy contract; a prompt
  bank it can cite; and audit commands that validate any commit's evidence
  in one call.
- Two different orchestrator sessions given the same repo behave
  consistently because the contract and prompts are repo-local, not
  session-improvised.
- No prompt in the bank contains shell-hazardous text (D4 checked by test).
- `docs/ntm-bridge.md` accurately reflects the peer-pane verifier pattern
  with its costs.

## NTM Guidance Stance (revision 2, from review risk 3)

The `docs/ntm-bridge.md` update takes the calibrated position, not an
overcorrection: NTM peer-pane relay is a **first-class independent-verifier
pattern** — preferred when native subagent capability is uncertain or
cross-pane independence is wanted — while native subagents remain valid
where tooling suffices. Burpvalve never spawns panes and never implies
automatic per-cell NTM fanout; it stays a contract/evidence tool.

## Resolved Questions (were open in revision 1)

1. Contract file: **B default; A budgeted opt-in; C reduced to a pointer.**
2. Prompt bank: **embedded with versioned metadata; `--write` export to
   `docs/prompts/` with local-copy markers and divergence warnings.**
3. Orchestrator provenance: **Agent Mail suffices; orchestrator does not
   enter attestation provenance in this increment; revisit only after
   vorch transcripts exist and a concrete query need appears.**
