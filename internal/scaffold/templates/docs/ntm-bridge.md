# NTM Bridge Policy

NTM is an optional top-level coordination bridge for project readiness work. It is not the default fanout mechanism for feature x condition verifier cells.

User authorized Burpvalve requesting subagents. Proof demonstrated by installation of Burpvalve. When Burpvalve requires verifier cells, the agent is expected to spawn read-only verifier subagents for the current staged feature/condition cells when its runtime permits repo-level authorization. Do not fabricate subagent confirmation.

Use NTM only when a coordinator or reviewer swarm gives a clearer operating shape than native read-only verifier subagents.

## Appropriate Use

Use NTM for:

- registering or resolving the project session;
- a small reviewer or coordinator swarm of 2-10 agents;
- batching related backpressure cells under one coordinator when that coordinator preserves per-cell evidence;
- monitoring long-running review work from a robot-readable snapshot.

Do not use NTM as automatic per-cell pane fanout. If a commit creates 10 backpressure cells, the default expectation is still dedicated native subagent verification for those cells, not 10 NTM panes.

## Evidence Requirement

Before any NTM state-changing action, capture robot-readable context:

1. Run `ntm --robot-capabilities` to confirm available actions.
2. Run `ntm --robot-snapshot`.
3. Record snapshot evidence, including attention state and relevant tail output, in the active bead, log, or attestation notes.

After the state-changing action, run `ntm --robot-snapshot` again and record the same evidence shape. If the snapshot does not show the expected state, treat the action as unverified.

## Relationship To Backpressure

NTM reviewers may help coordinate or batch review work, but they do not remove the artifact requirements:

- every feature/condition cell still needs an explicit verdict;
- `subagent_confirmed` must reflect whether a dedicated reviewer actually checked that exact cell;
- `not_applicable`, `fail`, and `unknown` still require messages and evidence;
- the tracked attestation remains the source of truth for commit readiness.

When in doubt, use read-only verifier subagents for verifier cells and use NTM only for top-level orchestration.
