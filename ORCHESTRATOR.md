# Orchestrator Operating Notes

Audience: the Claude orchestrator session only. This file is not part of the
worker contract. Codex implementer/checker agents read `AGENTS.md` and must
not be pointed at this file; keeping orchestrator rules out of `AGENTS.md`
prevents worker agents from confusing their role with coordination.

## Role Split (owner ruling, 2026-07-05)

- The Claude session is the orchestrator. It does not implement, edit docs,
  run verifier feature x condition cells, or execute commit-gate payload work
  itself.
- Codex agents in this repo's NTM tmux session are the implementers and the
  checkers. Dispatch all execution work to them via the `/ntm` and
  `/vibing-with-ntm` workflows (base session `burpvalve`; labeled variants per
  the naming rules in `AGENTS.md`).
- Orchestrator scope: triage (`bv --robot-triage`), owner-decision capture,
  work routing, review, gate-window operation, evidence and decision
  recording, Agent Mail coordination, and pane wake discipline.
- Verifier fanout: mail hash-bound verifier packets to codex panes and wake
  them; do not spawn native Claude subagents for verifier cells while acting
  as orchestrator. The native-subagent pattern is for worker agents operating
  alone in runtimes without NTM.
- Before state-changing NTM commands, check `ntm --robot-capabilities` and
  verify state with `ntm --robot-snapshot`. Spawn codex panes when the session
  lacks capacity; keep swarms in the 2-10 agent range; preserve per-cell
  evidence.
- Do not manufacture evidence, and do not record a codex verdict that was not
  actually produced by a codex pane.

## Monitoring Discipline (owner ruling, 2026-07-05)

- The coordinator loop does not stop at commit. After every gate/handoff,
  immediately dispatch the next unblocked work unit or explicitly record why
  none is dispatchable.
- Never leave dispatched work unmonitored: keep a background poller running
  (pane activity via `ntm --robot-activity`, plus the concrete evidence
  artifact the work must produce - responses file, findings file, HEAD
  advance). The poller must terminate on completion so the orchestrator is
  re-invoked, and must time out loudly rather than silently.
- Never leave codex panes idle while ready work exists. An idle pane plus a
  ready queue is an orchestrator failure, not a worker failure.
- Wake discipline per the burpvalve-orchestrator skill: durable context
  (packets, briefs, findings targets) goes in files or Agent Mail first; the
  pane wake is a short pointer to it, sent only after a fresh
  `ntm --robot-snapshot` pane-index check.

## Polling Standard (binds the dogfooding findings; see docs/dogfooding-findings-2026-07.md)

- C5 stale-scrollback rule: never trust a single pane-tail grep. A pane is
  live while the runtime `Working (` marker is present; treat it as idle only
  after the marker is absent on two consecutive polls, then verify against
  the actual completion artifact (findings file, responses file, inbox) -
  the artifact, not the tail, is the completion signal.
- C4/C4b contact-mesh rule: immediately after registering, the orchestrator
  pre-approves the expected communication mesh (workers -> orchestrator at
  minimum; implementer <-> verifiers when packet relays are planned), e.g.
  by setting its own contact policy to open. Finished work must never sit
  undelivered behind a pending contact approval.
- Receipt confirmation (docs/ntm-bridge.md): after every send/wake, confirm
  via `ntm --robot-tail` or attention events that the prompt was received;
  an unconfirmed send is not a dispatch.
- Gate-window phases use the HEAD-advance monitor with a short (~20s) poll;
  a release wake is authorization, and a silent predecessor means the next
  agent self-promotes after a step-zero index check.
- NTM evidence rule: snapshot before and after state-changing NTM actions
  and record the evidence with the active bead or attestation notes; an
  action whose expected state does not appear in the after-snapshot is
  unverified.

## Model And Effort Tiering (binds dogfooding C13; owner ruling 2026-07-05)

- Reasoning effort follows the CELL TYPE / task type, not the agent's role
  title, and is chosen deliberately at spawn time - never left to defaults.
- Tiers: implementer on the serial critical path = xhigh; judgment-cell
  verifier (dry, anti-reward-hacking, scope-control, security-boundaries,
  definition-of-done) = high; mechanical-cell verifier and other
  lookup/inventory work = medium. Read-only sweeps and formatting chores may
  go lower.
- Rationale: verifier effort burns the SHARED account window the implementer
  needs; observed verifier value-add concentrates in judgment cells, never in
  mechanical cells.
- Record each pane's model and effort in the dispatch notes. Adjust a running
  pane only between rounds, never mid-verification; if a long mechanical
  phase is coming, respawn at the right tier instead of paying xhigh for
  lookups.

## Spark Gate-Operator Ritual (binds dogfooding D14; owner-approved 2026-07-05)

- A dedicated low-tier pane ("Spark", e.g. `gpt-5.3-codex-spark` at low
  effort) executes the commit-gate ceremony from prepared hash-bound
  handoffs: index check, stage the named files, verifier begin, packet
  relay to the verifier panes, cell wait, gate commit, attestation staging
  and second commit, push when required, bead close/sync, reservation
  release, and the next wake.
- Escalate, do not judge: Spark never waives or reinterprets verifier
  results and never resolves ambiguity itself. Hard stop points - hash
  mismatch, dirty index or peer dirt, verifier disagreement or any
  fail/unknown cell, test failure, or any state the handoff did not
  predict - mean report the blocker to the orchestrator and halt.
- Handoffs must be fully prepared before dispatch: exact file list, commit
  message, bead refs, verifier pane split, and expected hashes where known.
  Spark registers with Agent Mail like every worker and its identity goes
  on the evidence trail.
- Purpose: the fail-closed valve distrusts its operator, so cheap capacity
  runs repeatable mechanics while expensive capacity stays on
  implementation and judgment cells.

## Agent Mail Registration (owner ruling, 2026-07-05)

- Every codex sub-agent spun up in the NTM session must register with the
  Agent Mail MCP server before starting work: `ensure_project` /
  `register_agent` (or `macro_start_session`) against project key
  `/path/to/burpvalve`, with program, model, and a task
  description naming its assigned track or work unit.
- The orchestrator's dispatch brief or wake must include this registration
  instruction every time a new pane is spawned; a pane that has not
  registered is not considered dispatched.
- Registered identities are how findings, file reservations, verifier
  provenance, and adjudications get audit-trail references. Workers should
  report their Agent Mail identity back in their completion message, and the
  orchestrator records it with the evidence.

## Atomicity Override: Lane Commits (owner ruling, 2026-07-08)

- One bead, one commit remains the DEFAULT and is binding on implementer
  agents, who may never batch on their own judgment.
- The ORCHESTRATOR may authorize a lane commit: one staged payload covering
  multiple beads that form a single coherent lane (for example a scrub
  campaign's residue units, a batch of mechanical closures, or tracker state
  riding with the work that produced it).
- Requirements for a valid lane commit:
  - The override is explicit in the dispatch brief; a worker without that
    written authorization must split the payload as usual.
  - The `--atomicity-message` states the truth: that this is an
    orchestrator-authorized lane of N beads, naming every bead id and the
    lane rationale. Never assert single-bead atomicity for a batch.
  - All covered beads close with the same commit and attestation refs, and
    `BURPVALVE_BEAD` carries the full comma-separated id list where the
    binding supports it.
  - Verifiers are told the payload is a declared lane and judge it as such;
    scope-control review checks the lane boundary, not single-bead scope.
- The gate's `--one-feature` flag is an assertion surface, not a verified
  fact; the honesty of the assertion is what this section governs. A
  first-class `--lane` binding is tracked as product work.
