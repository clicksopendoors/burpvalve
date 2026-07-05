# Claude Orchestrator Skill Plan

## Background

Burpvalve currently has three related but distinct surfaces for agent guidance:

1. `AGENTS.md` is the project-local operating contract for ordinary coding agents. It is generated from `internal/scaffold/templates/AGENTS.md.tmpl` and is the canonical place for repo rules, Beads usage, file coordination, backpressure, and definition of done.
2. `CLAUDE.md` is currently treated as the Claude Code compatibility route for ordinary agents. The scaffold creates it as a symlink to `AGENTS.md`; setup and repair inspect it as a symlink, and regular files are conflicts or are imported into `AGENTS.md` before replacement.
3. `ORCHESTRATOR.md` is an opt-in sidecar target. It gives coordinator notes, but it is not part of the standard scaffold and it is not installed as a Claude skill.

The owner wants a first-class Claude orchestrator route installed under `.claude/skills/` using `SKILL.md` format. That route may collide with the existing `CLAUDE.md -> AGENTS.md` route, so this plan makes the route choice explicit rather than silently treating a real `CLAUDE.md` as repair debris.

This is planning only. No implementation work should happen until this plan is reviewed, converted into beads, and assigned.

## Design Goals

- Make the Claude route choice obvious to humans and agents during first-time setup.
- Preserve the current default: Claude as an ordinary coding agent reads `AGENTS.md` through the `CLAUDE.md` symlink.
- Add a deliberate Claude-orchestrator route that installs a local Claude skill at `.claude/skills/burpvalve-orchestrator/SKILL.md`.
- Prevent silent route collisions. A repo must not end up with both an ordinary-agent symlink route and a real orchestrator `CLAUDE.md` by accident.
- Keep `AGENTS.md` as the canonical coding-agent contract. The orchestrator skill coordinates work; it does not replace the project operating contract.
- Package enough background into generated files so Codex agents and Claude orchestrators understand Burpvalve, Beads, attestations, verifier fanout, Agent Mail, and NTM wake/pane discipline.
- Package the generated Claude skill using the skill-authoring layout: `SKILL.md` as routing and procedure only, deeper material in `references/`, worked scenarios in `examples/`, an operating `SELF-TEST.md`, and super-lightweight Python helpers in `scripts/`.

## Route Contract

### Routes

Introduce a Claude route enum separate from the existing `defaults.init.orchestrator` sidecar setting:

```json
{
  "defaults": {
    "init": {
      "claude_route": "agent-symlink"
    },
    "repair": {
      "claude_route": "preserve"
    }
  }
}
```

Allowed `defaults.init.claude_route` values:

- `agent-symlink`: create or require `CLAUDE.md -> AGENTS.md`. This is the default and preserves current behavior.
- `orchestrator-skill`: install `.claude/skills/burpvalve-orchestrator/SKILL.md` and create a real `CLAUDE.md` bootstrap file for orchestrator mode.
- `none`: do not create or require any Claude route.

Allowed `defaults.repair.claude_route` values:

- `preserve`: repair whichever recognized route already exists; if no route exists, use `agent-symlink`. This is the default to avoid surprising existing repos.
- `agent-symlink`: repair toward `CLAUDE.md -> AGENTS.md`.
- `orchestrator-skill`: repair toward the local Claude orchestrator skill route.
- `none`: skip Claude route repair.

The existing `defaults.init.orchestrator` remains about the sidecar `ORCHESTRATOR.md` target only:

- `off`: do not create `ORCHESTRATOR.md` unless explicitly targeted.
- `orchestrator-md`: create or require `ORCHESTRATOR.md`.
- `claude-md`: remove the current rejected placeholder once the new route lands. It should not become the orchestrator route. The new route is `defaults.init.claude_route=orchestrator-skill`.

The existing `defaults.repair.orchestrator` remains a boolean for `ORCHESTRATOR.md` repair only.

### Files Per Route

`agent-symlink` route:

- `AGENTS.md`: generated or repaired ordinary-agent contract.
- `CLAUDE.md`: symlink to `AGENTS.md`.
- `.claude/skills/burpvalve-orchestrator/`: absent unless a user created it manually. Setup should not require it.

`orchestrator-skill` route:

- `AGENTS.md`: generated or repaired ordinary-agent contract. It remains required because implementers and verifiers still need the project contract.
- `.claude/skills/burpvalve-orchestrator/SKILL.md`: generated local Claude skill.
- `.claude/skills/burpvalve-orchestrator/references/`: generated reference files for bulk detail. These are required, not optional, because `SKILL.md` must stay as the routing and procedure layer.
- `.claude/skills/burpvalve-orchestrator/examples/`: worked orchestrator scenarios.
- `.claude/skills/burpvalve-orchestrator/scripts/`: small Python helpers that agents can run directly instead of writing ad hoc scripts.
- `.claude/skills/burpvalve-orchestrator/SELF-TEST.md`: concise operating self-test for agents using the skill in real orchestration scenarios.
- `CLAUDE.md`: regular generated bootstrap file, not a symlink. It tells Claude Code that this repo uses the Burpvalve orchestrator skill, points at `.claude/skills/burpvalve-orchestrator/SKILL.md`, and names `AGENTS.md` as the canonical coding-agent contract.
- `ORCHESTRATOR.md`: optional sidecar. It is created only when `orchestrator` is explicitly targeted or `defaults.init.orchestrator=orchestrator-md`.

`none` route:

- No `CLAUDE.md` route is created or required.
- `AGENTS.md` remains governed by the existing `defaults.init.agents` and `--no-agents` controls.

### Precedence

Command-line targets and flags win over config defaults.

1. Explicit `--claude-route` wins over config.
2. Explicit route target aliases win over config:
   - `CLAUDE.md`, `claude`, `claude-symlink`, `agent-symlink` mean `agent-symlink`.
   - `claude-orchestrator`, `claude-skill`, `orchestrator-skill`, `.claude/skills/burpvalve-orchestrator` mean `orchestrator-skill`.
3. `--no-claude` and `--no-claude-symlink` map to `claude_route=none` for compatibility. Help text should say these skip all Claude route files, not only symlink creation.
4. If two explicit route choices are present in one command, fail before mutation with a route-choice conflict.
5. Config defaults apply only when no explicit route was requested.

The `ORCHESTRATOR.md` sidecar target is independent. Requesting `orchestrator` does not change `claude_route`; requesting `orchestrator-skill` does not automatically create `ORCHESTRATOR.md`.

### Legacy Boolean Compatibility

The repo already has `defaults.init.claude` and `defaults.repair.claude` booleans through `ScaffoldDefaults`. They remain supported, but the route enum becomes the clearer source of truth.

Rules:

- If `defaults.init.claude=false` and `defaults.init.claude_route` is unset, effective route is `none`.
- If `defaults.repair.claude=false` and `defaults.repair.claude_route` is unset, effective repair route is `none`.
- If a legacy Claude boolean is `true` and route is unset, use the route default (`agent-symlink` for init, `preserve` for repair). Boolean `true` does not imply orchestrator mode.
- If a legacy Claude boolean is `false` and route is explicitly `agent-symlink`, `orchestrator-skill`, or `preserve`, config validation must fail with a conflict message naming both keys.
- If a legacy Claude boolean is `true` and route is explicitly `none`, config validation must fail with a conflict message naming both keys.
- Command-line `--no-claude` and `--no-claude-symlink` still win for that invocation and map to route `none`, even if config defaults request a route.

Tests must cover global/project merge and source tracking for these conflicts so an agent can see whether the conflict came from global config, project config, or command flags.

### AGENTS.md Dependency

Both active Claude routes depend on `AGENTS.md` as the canonical coding-agent contract.

Rules:

- `agent-symlink` requires `AGENTS.md` to exist or be created in the same operation.
- `orchestrator-skill` requires `AGENTS.md` to exist or be created in the same operation because the generated bootstrap and skill point to it.
- `--no-agents --claude-route agent-symlink` fails before mutation when `AGENTS.md` is missing.
- `--no-agents --claude-route orchestrator-skill` fails before mutation when `AGENTS.md` is missing.
- If `AGENTS.md` already exists and `--no-agents` is set, either active Claude route may proceed while reporting that `AGENTS.md` was used but not repaired.
- `claude_route=none` does not require `AGENTS.md`.

This prevents a generated symlink, bootstrap, or skill from pointing at a missing contract.

### Conflict Detection

Setup, init, and repair must report route state in machine-readable output. Suggested states:

- `agent_symlink_present`
- `orchestrator_skill_present`
- `none_present`
- `route_conflict`
- `user_owned_claude_file`
- `generated_claude_pointer_drift`
- `generated_skill_drift`

Rules:

- `agent-symlink` requested and `CLAUDE.md -> AGENTS.md` exists: pass.
- `agent-symlink` requested and `CLAUDE.md` is missing: create the symlink.
- `agent-symlink` requested and `CLAUDE.md` is a generated orchestrator bootstrap: replace it with the symlink only when the route was explicitly requested or repair is running with `claude_route=agent-symlink`.
- `agent-symlink` requested and `CLAUDE.md` is an unmarked regular file: conflict by default.
- `orchestrator-skill` requested and `CLAUDE.md` is missing: create the generated bootstrap and skill.
- `orchestrator-skill` requested and `CLAUDE.md -> AGENTS.md` exists: replace the symlink with the generated bootstrap because the route was explicit and no content is lost. Report the replacement.
- `orchestrator-skill` requested and `CLAUDE.md` is the generated bootstrap: repair/update it when scaffold markers match.
- `orchestrator-skill` requested and `CLAUDE.md` is an unmarked regular file: conflict. Do not overwrite.
- `orchestrator-skill` requested and `.claude/skills/burpvalve-orchestrator/SKILL.md` is missing: create it.
- `orchestrator-skill` requested and the skill file has scaffold markers: repair/update it.
- `orchestrator-skill` requested and the skill file lacks scaffold markers or has user edits outside the managed section: conflict or preserve-and-report, depending on the existing scaffold generated-file pattern chosen by the implementation. The behavior must be tested and documented.
- `none` requested: do not create or repair Claude route files. Existing route files may be reported but must not be modified.

Repair must stop treating every regular `CLAUDE.md` as content to import into `AGENTS.md`. A generated orchestrator bootstrap is an intentional route artifact, not abandoned Claude notes.

### Legacy `CLAUDE.md` Adoption

Current repair behavior can import an existing regular `CLAUDE.md` into `AGENTS.md` before replacing it with the symlink route. That adoption behavior should remain available, but it must become explicit once generated regular `CLAUDE.md` files are valid route artifacts.

Plan the adoption selector as a named repair option, for example `--adopt-claude-md`, plus a robot field such as `"adopt_claude_md": true`. Exact flag naming can be refined during bead conversion, but the behavior must be explicit and tested.

Rules:

- Adoption is only valid for an unmarked regular `CLAUDE.md`.
- Adoption is invalid for a generated orchestrator bootstrap; generated bootstrap files are repaired as route files.
- Adoption imports into `AGENTS.md` using the existing marker style, then applies the requested route.
- Without the adoption selector, unmarked regular `CLAUDE.md` remains a conflict.
- The TUI must explain adoption separately from route selection if it offers the repair.

## Human Setup Contract

Interactive `burpvalve init` and `burpvalve repair` must include an obvious route choice when Claude is not skipped:

Prompt title:

```text
Claude Code route
```

Choices:

- `Ordinary agent`: create `CLAUDE.md -> AGENTS.md`; Claude reads the same contract as other coding agents.
- `Orchestrator`: install `.claude/skills/burpvalve-orchestrator/` and create a real `CLAUDE.md` bootstrap for coordinating agents.
- `No Claude route`: do not create Claude-specific files.

Prompt copy must say that the orchestrator route changes `CLAUDE.md` from a symlink into a generated regular file. If a conflicting existing route is detected, the TUI must show the conflict before the final confirmation screen.

The final confirmation screen should include the selected route in its piece list, for example:

```text
Pieces: AGENTS.md, Claude route: orchestrator-skill, hooks, backpressure
```

## Agent And Robot Setup Contract

Agents should be able to discover and select the route without opening source code.

Required command surfaces:

- `burpvalve init --help` documents `--claude-route agent-symlink|orchestrator-skill|none`.
- `burpvalve repair --help` documents `--claude-route preserve|agent-symlink|orchestrator-skill|none`.
- `burpvalve setup --json` reports the detected route, expected route, and conflicts.
- `burpvalve config init --robots` documents `defaults.init.claude_route` and `defaults.repair.claude_route`.
- `burpvalve init --robots` input accepts `claude_route`.
- `burpvalve repair --robots` input accepts `claude_route`.
- JSON results include `claude_route`, `claude_route_source`, `created`, `repaired`, `skipped`, and `conflicts` entries precise enough for an agent to decide the next command.

Robot examples should show both routes:

```json
{"target":".","confirm":true,"claude_route":"agent-symlink"}
```

```json
{"target":".","confirm":true,"claude_route":"orchestrator-skill"}
```

Agent-facing help must also state that `ORCHESTRATOR.md` is still a separate optional sidecar target.

## Generated Skill Package Contract

Install the Claude skill package at:

```text
.claude/skills/burpvalve-orchestrator/
  SKILL.md
  references/
    burpvalve-gate-choreography.md
    verifier-fanout-and-attestations.md
    agent-mail-and-file-coordination.md
    ntm-pane-wake-discipline.md
    beads-and-gate-window-operations.md
  examples/
    gated-implementation-handoff.md
    verifier-disagreement-hold.md
    gate-window-release.md
  scripts/
    pane_wake.py
    attestation_summary.py
    append_finding.py
  SELF-TEST.md
  .MAINTAINER-CHECKS.md
```

The generated `SKILL.md` must be a routing and procedure layer only. It should not carry the long Burpvalve manual, full NTM bridge policy, long verifier examples, or copied prompt-bank content. Bulk detail belongs in linked files under `references/`; concrete worked runs belong in `examples/`.

### Upload-Safe Frontmatter

The generated skill must use upload-safe frontmatter even though it is installed project-locally:

```markdown
---
name: burpvalve-orchestrator
description: >-
  Coordinate Burpvalve-backed multi-agent work with Beads assignment, file reservations, verifier fanout, attestation-aware gates, Agent Mail, and NTM wakes.
category: other
tags:
  - ctx-cli
  - tool-git
license: MIT
distribution: public
---
```

Rules:

- `name` must match the directory exactly: `burpvalve-orchestrator`.
- `description` must parse to fewer than 500 characters and use folded YAML.
- `category` is `other`.
- Tags are limited to known JSM tags. For this package use `ctx-cli` and `tool-git`.
- `license` and `distribution` are explicit. Use `license: MIT` because the generated scaffold content ships from the MIT-licensed Burpvalve repo; use `distribution: public` so the scaffold metadata matches the public launch package.
- No binaries are allowed inside the JSM-validated portion. Scripts must be text Python files.
- Package validation checks belong in `.MAINTAINER-CHECKS.md`; `SKILL.md` should not link that file because it is maintainer-facing, not operating guidance.

The project-local `.claude/skills/burpvalve-orchestrator/` output is the deliverable. Do not introduce a separate canonical `<workspace>/jsm-skills/skills/burpvalve-orchestrator` source tree in this plan. Instead, validate the generated scaffold output directly.

### SKILL.md Body

Required `SKILL.md` shape:

- H1: `# burpvalve-orchestrator`.
- Core-rule block at the top:
  - coordinate evidence flow, do not manufacture evidence;
  - one atomic payload per gated commit;
  - terminal wakes route attention only and are not verifier verdicts.
- Quick Start with the shortest safe coordinator loop:
  - read `AGENTS.md`;
  - run `git status --short`;
  - inspect ready work with `br ready --json` or `bv --robot-triage`;
  - inspect Agent Mail and reservations;
  - assign or unblock one bead;
  - ensure verifier packets, wakes, and gate-window handoffs are explicit.
- Decision tables:
  - choose ordinary implementer, verifier, or coordinator action;
  - choose native subagent vs peer-pane verifier relay;
  - choose when to hold the gate;
  - choose when to use each bundled script.
- Reference map linking every `references/`, `examples/`, `scripts/`, and `SELF-TEST.md` file by concrete relative path.
- Short validation checklist for the orchestrator's current action.

Required reference files:

- `references/burpvalve-gate-choreography.md`: stage one atomic payload, generate packets from staged content, collect response JSON, run the gate, stage the emitted attestation, rerun commit, push only when the repo protocol says to push.
- `references/verifier-fanout-and-attestations.md`: verifier roles, provenance fields, authorization-is-not-evidence rule, disagreement holds, transcript summaries, attestation query/check commands.
- `references/agent-mail-and-file-coordination.md`: registration, inbox polling, file reservations, thread replies, handoffs, stale reservation etiquette, no clobbering.
- `references/ntm-pane-wake-discipline.md`: `ntm --robot-capabilities`, `ntm --robot-snapshot`, pane identity checks, scoped `ntm --robot-send`, wake evidence limits.
- `references/beads-and-gate-window-operations.md`: `br` claim/close/sync commands, dependency checks, gate-window queue, index-empty check, release handoff shape.

Required examples:

- `examples/gated-implementation-handoff.md`: owner assignment through commit, attestation, push, bead closure, and handoff.
- `examples/verifier-disagreement-hold.md`: conflicting verdicts, owner ruling, delta re-check, and corrected gate.
- `examples/gate-window-release.md`: acquire, confirm index empty, stage, commit, release, wake next holder.

Required scripts:

- `scripts/pane_wake.py`: wrapper that prints and optionally runs the pane-scoped NTM wake sequence after checking a supplied session, pane, and agent label. It must call `ntm --robot-capabilities` and `ntm --robot-snapshot` before printing or running a wake, and it must not guess pane indices.
- `scripts/attestation_summary.py`: read one or more Burpvalve attestation JSON files and print feature, payload hash, verdict counts, verifier provenance summary, and transcript refs.
- `scripts/append_finding.py`: append a dated finding skeleton to a project findings log using supplied ID/title/category/body fields; it must refuse to write if the target file does not exist or contains no recognizable findings heading.

These are agent-run helpers shipped as skill resources. Burpvalve scaffolds them but does not call them automatically, so they do not turn Burpvalve itself into an NTM, Agent Mail, or findings-log automation engine.

Script constraints:

- Python standard library only.
- No network access.
- No binary payloads.
- No destructive filesystem operations.
- `--help` supported by every script.
- Dry-run is the default wherever a script can write or run an external command.
- Write or execute behavior requires an explicit flag such as `--write` or `--execute`.
- Scripts print commands before executing them when they invoke external tools.

Required `SELF-TEST.md`:

- It is an operating self-test, not package QA.
- It asks whether the orchestrator actually applied the skill well:
  - Did I identify the active bead and owner order?
  - Did I preserve one-payload-one-commit boundaries?
  - Did I reserve or coordinate shared files?
  - Did verifier evidence remain hash-bound and separate from authorization?
  - Did I wake only verified panes?
  - Did I hold on disagreements or route conflicts?
  - Did I record durable decisions in `docs/` when required?
- It must not be a maintainer checklist for JSM upload. Package validation can live in implementation bead acceptance criteria or an unlinked maintainer note if needed.

Required `.MAINTAINER-CHECKS.md`:

- It is generated but not linked from `SKILL.md`.
- It covers package validation only:
  - `jsm validate .claude/skills/burpvalve-orchestrator`;
  - parsed description under 500 characters;
  - every file in the `SKILL.md` reference map exists;
  - every `scripts/*.py` helper supports `--help`;
  - no binaries or generated archives are present.
- It must not replace the operating self-test.

Operational content should be distributed as follows:

- Startup checklist:
  - short form in `SKILL.md`;
  - full command examples in `references/beads-and-gate-window-operations.md` and `references/agent-mail-and-file-coordination.md`.
- Role boundaries:
  - short decision table in `SKILL.md`;
  - examples in `examples/gated-implementation-handoff.md` and `examples/verifier-disagreement-hold.md`.
- Burpvalve gate choreography:
  - core rule and quick start in `SKILL.md`;
  - full detail in `references/burpvalve-gate-choreography.md`.
- Verifier fanout:
  - decision table in `SKILL.md`;
  - full detail in `references/verifier-fanout-and-attestations.md`.
- Agent Mail and file coordination:
  - quick start in `SKILL.md`;
  - full detail in `references/agent-mail-and-file-coordination.md`.
- NTM wake discipline, drawing on `docs/ntm-bridge.md`:
  - decision table and script pointer in `SKILL.md`;
  - full detail in `references/ntm-pane-wake-discipline.md`;
  - helper in `scripts/pane_wake.py`.
- Durable decisions:
  - short rule in `SKILL.md`;
  - finding-log helper in `scripts/append_finding.py`.
- Failure handling:
  - core rule in `SKILL.md`;
  - disagreement example in `examples/verifier-disagreement-hold.md`.

## Generated `CLAUDE.md` Bootstrap Contract

For `orchestrator-skill`, `CLAUDE.md` becomes a generated regular file with scaffold markers. It should be short and direct:

- state that this repo uses the Burpvalve Claude orchestrator route;
- point to `.claude/skills/burpvalve-orchestrator/SKILL.md`;
- tell Claude to read `AGENTS.md` before coordinating;
- state that `AGENTS.md` remains the source of truth for coding-agent rules and commit gates;
- state that this file intentionally is not a symlink.

It must not duplicate the whole skill body. Duplicating would create drift between `CLAUDE.md`, `AGENTS.md`, `ORCHESTRATOR.md`, and the generated skill.

## AGENTS.md Template Update

Update `internal/scaffold/templates/AGENTS.md.tmpl` so the generated ordinary-agent contract teaches agents enough to operate in a coordinated Burpvalve repo even when no separate orchestrator is present.

Required additions:

- A short "Taking Orders And Handoffs" section:
  - accept owner/orchestrator assignments through the active coordination channel;
  - claim the bead before work;
  - announce scope before editing shared files;
  - use file reservations when configured;
  - do not widen scope when a requirement is unclear.
- A short "Burpvalve Gate Choreography" section:
  - one atomic payload per commit;
  - stage only owned files;
  - run the backpressure verifier before commit;
  - keep responses honest and hash-bound;
  - stage generated attestations;
  - do not use bypass flags.
- A short "Verifier Work" section:
  - verifiers are read-only;
  - verdicts must name evidence;
  - authorization is never evidence for a condition cell;
  - disagreements and unknowns block until resolved.
- A route pointer:
  - if the Claude orchestrator route is installed, coordinators should read `.claude/skills/burpvalve-orchestrator/SKILL.md`;
  - if `ORCHESTRATOR.md` is present, it is supplemental coordinator guidance.

Do not make `AGENTS.md` a full orchestrator manual. Keep it useful to ordinary agents and refer to the skill for coordinator-specific details.

## Contract Units And File-Touch Map

### Unit 1: Config Schema And Route Model

Purpose: add a durable route enum without overloading `defaults.init.orchestrator`.

Files:

- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/burpvalve/config_test.go`
- `cmd/burpvalve/help_test.go`

Work:

- Add `InitDefaults.ClaudeRoute string`.
- Add `RepairDefaults.ClaudeRoute string`.
- Validate the enum values listed in this plan.
- Preserve source tracking for `defaults.init.claude_route` and `defaults.repair.claude_route`.
- Update config robot help notes.
- Remove or revise the current `defaults.init.orchestrator=claude-md is not yet supported` placeholder so users are directed to `defaults.init.claude_route=orchestrator-skill`.

Tests:

- load/merge/source tracking for global and project route defaults;
- invalid enum rejection;
- compatibility with existing configs that only set `defaults.init.orchestrator`;
- legacy `defaults.init.claude` and `defaults.repair.claude` compatibility when route is unset;
- validation failures for inconsistent legacy boolean plus route combinations;
- robot help includes both new keys and states that `ORCHESTRATOR.md` remains separate.

### Unit 2: Scaffold Targets And Generated Templates

Purpose: make the route addressable by target names and embed the generated files.

Files:

- `internal/scaffold/targets.go`
- `internal/scaffold/targets_test.go`
- `internal/scaffold/templates/CLAUDE.md.orchestrator.tmpl`
- `internal/scaffold/templates/claude/skills/burpvalve-orchestrator/SKILL.md.tmpl`
- `internal/scaffold/templates/claude/skills/burpvalve-orchestrator/references/*.tmpl`
- `internal/scaffold/templates/claude/skills/burpvalve-orchestrator/examples/*.tmpl`
- `internal/scaffold/templates/claude/skills/burpvalve-orchestrator/scripts/*.py.tmpl`
- `internal/scaffold/templates/claude/skills/burpvalve-orchestrator/SELF-TEST.md.tmpl`
- `internal/scaffold/templates/claude/skills/burpvalve-orchestrator/.MAINTAINER-CHECKS.md.tmpl`
- `internal/scaffold/template_contract_test.go`

Work:

- Add internal route constants and generated package templates.
- Do not expose public target aliases yet unless Unit 3 route behavior lands in the same commit. This avoids a release where `burpvalve init claude-skill` normalizes but has undefined mutation semantics.
- Keep `CLAUDE.md` and `claude` aliases mapped to the symlink route.
- Ensure `standard` still resolves to the ordinary default route.
- Add generated file markers to the orchestrator `CLAUDE.md` bootstrap and every generated skill package file.
- Keep sidecar `ORCHESTRATOR.md` target separate.
- Ensure scripts are generated with executable or readable mode according to the repo's existing scaffold conventions; tests must lock whichever mode is chosen.
- Ship the complete valid Claude skill package content in this unit. Later units may adjust surrounding AGENTS/ORCHESTRATOR/Codex docs, but they should not be required to make the generated skill package structurally valid.

Tests:

- target normalization proves current public aliases remain unchanged;
- internal route/package template tests cover the future aliases without exposing them publicly unless behavior exists;
- `standard` does not include the orchestrator skill route;
- template contract validates required upload-safe frontmatter, required package layout, required reference map links, required self-test intent, and required text snippets.
- template contract rejects binary files under the generated skill package.
- template contract verifies `SKILL.md` links every generated `references/`, `examples/`, `scripts/`, and `SELF-TEST.md` resource exactly once and does not link `.MAINTAINER-CHECKS.md`.
- template contract verifies every linked resource path exists.
- generated script tests verify each `scripts/*.py` helper supports `--help`, defaults to dry-run when it can mutate, and requires explicit write/execute flags.

### Unit 3: Init, Repair, And Inspect Route Semantics

Purpose: make scaffold behavior match the route contract.

Files:

- `internal/scaffold/targets.go`
- `internal/scaffold/apply.go`
- `internal/scaffold/repair.go`
- `internal/scaffold/inspect.go`
- `internal/scaffold/targets_test.go`
- `internal/scaffold/apply_test.go`
- `internal/scaffold/repair_test.go`
- `internal/scaffold/inspect_test.go`
- `internal/scaffold/fixture_workflow_test.go`

Work:

- Expose public target aliases for `claude-orchestrator`, `claude-skill`, `orchestrator-skill`, and `.claude/skills/burpvalve-orchestrator` only when the behavior is implemented.
- Extend `ApplyOptions` and `InspectOptions` with a route field.
- Implement route-aware `ensureClaudeRoute`, `repairClaudeRoute`, and `inspectClaudeRoute` helpers.
- Preserve the existing symlink behavior for the default route.
- In orchestrator route mode, create/update the generated regular `CLAUDE.md` bootstrap and local skill files.
- Stop importing generated orchestrator bootstrap content into `AGENTS.md`.
- Report route conflicts and planned changes in JSON and text output.

Tests:

- default init creates only `CLAUDE.md -> AGENTS.md`;
- orchestrator route init creates `AGENTS.md`, regular `CLAUDE.md`, and the full `.claude/skills/burpvalve-orchestrator/` package;
- orchestrator route init does not create `ORCHESTRATOR.md` unless the sidecar target/default is active;
- repair preserves a healthy symlink route;
- repair converts symlink to orchestrator bootstrap only when the route is explicit;
- repair converts generated orchestrator bootstrap back to symlink only when the route is explicit;
- unmarked regular `CLAUDE.md` remains a conflict;
- explicit adoption imports unmarked regular `CLAUDE.md` into `AGENTS.md` before applying the selected route;
- adoption is rejected for generated orchestrator bootstraps;
- active Claude routes fail before mutation when `--no-agents` is set and `AGENTS.md` is missing;
- active Claude routes proceed when `--no-agents` is set and `AGENTS.md` already exists, while reporting that `AGENTS.md` was not repaired;
- generated skill drift is repaired or reported according to the chosen generated-file policy;
- setup inspect reports expected, detected, missing, and conflicting route state.
- explicit route conflict normalization fails before mutation.

### Unit 4: CLI, TUI, Robot, And Help Wiring

Purpose: make the route choice obvious on every setup surface.

Files:

- `cmd/burpvalve/main.go`
- `cmd/burpvalve/init_test.go`
- `cmd/burpvalve/config_test.go`
- `cmd/burpvalve/help_test.go`
- `internal/charmui/init.go` or equivalent TUI files
- `internal/charmui/*test.go`

Work:

- Add `--claude-route` flags to `init` and `repair`.
- Add the explicit regular-`CLAUDE.md` adoption selector to `repair` only.
- Add robot input fields and output fields.
- Add TUI route selection with clear descriptions.
- Include route in final confirmation descriptions.
- Wire config defaults into setup, init, and repair.
- Keep existing `--no-claude` aliases as route `none`.

Tests:

- CLI forced init with each route;
- robot init with each route;
- repair robot with `preserve`, `agent-symlink`, `orchestrator-skill`, and `none`;
- repair robot and CLI adoption of an unmarked regular `CLAUDE.md`;
- CLI and robot route failures when `--no-agents` or equivalent skip settings would create a route pointing to missing `AGENTS.md`;
- help text contains the explicit route choices;
- TUI result maps route choices to init/repair options;
- setup JSON reports route facts.

### Unit 5: Generated Agent Guidance And Prompt-Bank Alignment

Purpose: make generated ordinary-agent and coordinator-adjacent docs align with the new route, without taking ownership of the skill package templates already completed in Unit 2.

Files:

- `internal/scaffold/templates/AGENTS.md.tmpl`
- `internal/scaffold/templates/ORCHESTRATOR.md.tmpl`
- `internal/backpressure/prompt_bank.go`
- `cmd/burpvalve/prompts_test.go`
- `skill/burpvalve/SKILL.md`
- `docs/ntm-bridge.md` if a small clarification is needed

Work:

- Update `AGENTS.md` template with coordination, gate choreography, verifier, and route pointer sections.
- Update `ORCHESTRATOR.md` template only if it should point to the Claude skill route when both are present.
- Ensure prompt-bank entries referenced by generated AGENTS/ORCHESTRATOR/Codex docs exist and are named correctly.
- Update the Codex Burpvalve skill so Codex agents know both Claude routes and do not mis-describe repair behavior.

Tests:

- scaffolded `AGENTS.md` contains the required coordination and gate sections;
- generated AGENTS/ORCHESTRATOR/Codex docs reference existing prompt-bank names only;
- prompt tests cover any new or changed prompt-bank text;
- Codex skill validation still passes.

### Unit 6: End-To-End Route Acceptance

Purpose: prove the routes work from user-facing commands.

Files:

- `cmd/burpvalve/init_test.go`
- `cmd/burpvalve/setup_test.go` if present, otherwise the current setup test file
- `internal/scaffold/fixture_workflow_test.go`
- `docs/` release or user-facing docs if needed

Work:

- Add end-to-end tests for fresh repos using both routes.
- Add a repair-after-drift scenario for each route.
- Add acceptance docs or examples only if the help text is not enough.
- Validate the generated Claude skill package shape with `jsm validate` against a staged temporary package path when `jsm` is available. If `jsm` is not available, tests must still check the required upload-safe frontmatter and no-binaries rule directly.

Tests:

- `go test ./internal/config ./internal/scaffold ./internal/charmui ./cmd/burpvalve`
- `go test ./...`
- `go run ./cmd/burpvalve init --force --json --target <tmp> --claude-route agent-symlink`
- `go run ./cmd/burpvalve init --force --json --target <tmp> --claude-route orchestrator-skill`
- `go run ./cmd/burpvalve setup --json --target <tmp>` for both routes
- `jsm validate <tmp>/.claude/skills/burpvalve-orchestrator` if `jsm` is available in the environment
- `jsm validate skill/burpvalve` if `jsm` is available in the environment

## Commit Sequence

One bead should map to one commit.

1. `cos: config schema for Claude route`
   - Unit 1 only.
   - Acceptance: config load/merge/validation/source tests pass; help notes name the new route keys.
2. `cos: scaffold targets and Claude skill templates`
   - Unit 2 only.
   - Acceptance: generated package templates and internal route constants pass contract tests; no public alias or CLI behavior changes yet.
3. `cos: route-aware init repair inspect`
   - Unit 3 only.
   - Acceptance: scaffold package tests prove both routes and conflicts.
4. `cos: CLI TUI robot route selection`
   - Unit 4 only.
   - Acceptance: users and agents can select the route on all public setup surfaces.
5. `cos: generated agent guidance and route docs`
   - Unit 5 only.
   - Acceptance: generated AGENTS/ORCHESTRATOR/Codex docs teach coordination and Burpvalve choreography; Codex skill docs no longer describe regular `CLAUDE.md` as always invalid.
6. `cos: end-to-end Claude route acceptance`
   - Unit 6 only.
   - Acceptance: full route matrix and end-to-end commands pass; final docs/examples are consistent.

If a later bead conversion finds that Unit 3 and Unit 4 cannot be split without broken intermediate CLI behavior, merge only after owner sign-off. The intended split is: Unit 3 exposes route behavior through internal APIs and tests; Unit 4 wires public command surfaces.

## Acceptance Criteria

- Fresh default init still creates `CLAUDE.md -> AGENTS.md`.
- Fresh orchestrator init creates a generated regular `CLAUDE.md` and the full `.claude/skills/burpvalve-orchestrator/` package.
- Setup and repair distinguish symlink route, orchestrator route, no route, and conflicts.
- Humans see an explicit route choice in the TUI.
- Agents see an explicit route choice in help text, robot schemas, and JSON output.
- The optional `ORCHESTRATOR.md` sidecar remains independent from the Claude route.
- Unmarked user-owned `CLAUDE.md` is not overwritten silently.
- Generated orchestrator `CLAUDE.md` is not imported into `AGENTS.md` as if it were abandoned content.
- Scaffolded `AGENTS.md` explains coordination, taking orders, file reservations, Burpvalve gates, verifier evidence, and honest blockers.
- The generated Claude skill explains Burpvalve usage, attestations, verifier fanout, Agent Mail handoffs, and NTM wake discipline.
- The generated Claude skill package uses `SKILL.md` for routing and procedure only, with bulk detail in required `references/`, worked scenarios in `examples/`, ready-to-use Python helpers in `scripts/`, and an operating `SELF-TEST.md`.
- Generated skill frontmatter is upload-safe: matching name, folded description under 500 parsed characters, `category: other`, tags limited to `ctx-cli` and `tool-git`, explicit license, explicit distribution.
- `jsm validate` passes for the generated Claude skill package when `jsm` is available; no binaries are present in the validated package.
- `go test ./...` passes.
- The final implementation can be gated through Burpvalve one atomic commit at a time.

## Non-Goals

- Do not change Claude Code's global skill installation. This plan only scaffolds project-local `.claude/skills/` files.
- Do not ship binaries inside `.claude/skills/burpvalve-orchestrator/`; helper scripts must be lightweight text Python.
- Do not make Burpvalve spawn NTM panes or send Agent Mail directly.
- Do not treat orchestrator messages, terminal wakes, or init-time authorization as verifier evidence.
- Do not replace `AGENTS.md` as the canonical coding-agent contract.
- Do not auto-delete user-owned Claude files.
- Do not create public launch, release, or visibility-flip work.
- Do not convert this plan into beads until the owner rules on the reviewed plan.

## Open Questions For Review

1. Should `repair --claude-route preserve` create `agent-symlink` when no route exists, or should it leave no route untouched? This plan recommends creating the default symlink to preserve current repair behavior.
2. Should the generated skill include an unlinked `.MAINTAINER-CHECKS.md` for package validation, or keep package validation only in implementation bead acceptance criteria? This plan recommends generating the unlinked `.MAINTAINER-CHECKS.md` so package checks stay out of `SKILL.md` and `SELF-TEST.md`.
3. Should the current rejected `defaults.init.orchestrator=claude-md` value remain as a compatibility error, or should it be removed from the accepted enum entirely? This plan recommends rejecting it with a migration hint to `defaults.init.claude_route=orchestrator-skill`.
4. Should `--no-claude-symlink` remain as an alias after the route model lands? This plan recommends keeping it for compatibility but documenting that it now means no Claude route.
