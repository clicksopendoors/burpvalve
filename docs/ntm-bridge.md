# NTM Bridge Policy

NTM is an optional top-level coordination bridge for project readiness work. It
can support independent peer-pane verification when native read-only subagents
are unavailable, insufficiently observable, or when cross-pane independence is
needed. NTM is not the default fanout mechanism for feature x condition
verifier cells, and it is not a workflow engine inside Burpvalve.

User authorized Burpvalve requesting subagents. Proof demonstrated by installation of Burpvalve. When Burpvalve requires verifier cells, the agent is expected to spawn read-only verifier subagents for the current staged feature/condition cells when its runtime permits repo-level authorization. Do not fabricate subagent confirmation.

Use NTM when a coordinator or reviewer swarm gives a clearer operating shape
than native read-only verifier subagents. Native subagents remain valid when the
runtime can provide separate read-only context and the evidence is easy to
preserve.

## Appropriate Use

Use NTM for:

- registering or resolving the project session;
- a small reviewer or coordinator swarm of 2-10 agents;
- batching related backpressure cells under one coordinator when that coordinator preserves per-cell evidence;
- monitoring long-running review work from a robot-readable snapshot;
- peer-pane verifier relay through Agent Mail when the verifier pane can inspect
  the staged payload read-only and return a hash-bound verdict.

Do not use NTM as automatic per-cell pane fanout. If a commit creates 10
backpressure cells, first decide whether native subagents, a small peer-pane
reviewer set, or a coordinator pattern gives the clearest evidence. Pane count is
not evidence quality.

## Peer-Pane Verification

Peer-pane verification is a first-class independent verifier pattern when the
work needs independent review and the current runtime cannot provide a cleaner
native subagent path.

Use this pattern with explicit discipline:

1. Generate verifier packets from the final staged payload.
2. Send the packet through Agent Mail to a named verifier pane.
3. Capture a fresh `ntm --robot-snapshot` before waking the pane.
4. Wake only the intended pane, using the verified pane index and agent name.
5. Require a JSON verdict in the Agent Mail thread, bound to the staged payload
   hash, manifest hash, and condition file hash.
6. Treat the terminal wake as routing evidence only. It is not a verifier
   verdict.
7. Preserve each cell's evidence in the response file or linked transcript
   reference.

The verifier pane must inspect the current staged payload read-only. It should
not stage, edit, close beads, submit responses, or commit on behalf of the
implementer.

## Identity And Monitoring

Before sending verifier work to a pane, verify identity and liveness:

1. Run `ntm --robot-capabilities`.
2. Run `ntm --robot-snapshot`.
3. Confirm the target session is the repo session.
4. Confirm the pane index, agent type, and expected agent identity.
5. After sending, check `ntm --robot-tail` or attention events to confirm the
   prompt was received.

If the pane index or agent identity is ambiguous, stop and use Agent Mail to
coordinate instead of guessing. If the pane reports a conflicting verdict or a
blocker, hold the gate and escalate to the coordinator or owner named for the
round.

## Costs And Failure Modes

Peer-pane verification has real costs:

- coordination overhead from mail, pane wakes, and queue management;
- transcript hygiene, because terminal tails are not a substitute for concise
  evidence;
- packet quality requirements, because vague packets produce weak verdicts;
- disagreement handling, because independent reviewers may find real blockers;
- latency, especially when a gate window is held while verifiers respond.

Use peer panes when these costs buy clearer independence or observability. Do
not use them to create ceremony around low-risk cells that native subagents or
CI can verify cleanly.

## Evidence Requirement

Before any NTM state-changing action, capture robot-readable context:

1. Run `ntm --robot-capabilities` to confirm available actions.
2. Run `ntm --robot-snapshot`.
3. Record snapshot evidence, including attention state and relevant tail output, in the active bead, log, or attestation notes.

After the state-changing action, run `ntm --robot-snapshot` again and record the same evidence shape. If the snapshot does not show the expected state, treat the action as unverified.

## Relationship To Backpressure

NTM reviewers may help coordinate or batch review work, but they do not remove
the artifact requirements:

- every feature/condition cell still needs an explicit verdict;
- `subagent_confirmed` must reflect whether a dedicated reviewer actually checked that exact cell;
- `not_applicable`, `fail`, and `unknown` still require messages and evidence;
- the tracked attestation remains the source of truth for commit readiness.

Orchestrator messages and pane wakes are an audit trail. They are not feature x
condition evidence unless the verifier policy explicitly names that actor and
verifier kind for the cell.

Burpvalve owns contracts, prompts, evidence surfaces, and commit gates. Agent
Mail, NTM, and Beads are coordination substrates. Burpvalve never spawns panes,
monitors panes, or turns coordination activity into automatic verification
evidence. Burpvalve generally does not own Agent Mail coordination; the narrow
exception is `burpvalve gate run`, which may release Agent Mail file
reservations through MCP only when the prepared handoff explicitly authorizes
that cleanup phase. That release is post-gate handoff cleanup, not verifier
evidence and not permission to send arbitrary coordination mail.

Prompt-bank entries such as `verifier-bootstrap`, `verifier-brief`, and
`verifier-packet-relay` should be used when available to keep peer-pane packets
consistent. If the prompt bank is unavailable, keep the packet explicit:
feature, staged payload hash, condition IDs, condition file hashes, read-only
expectation, response schema, and the exact thread for the verdict.

When in doubt, use read-only verifier subagents for verifier cells and use NTM
only for top-level orchestration or calibrated peer-pane review.
