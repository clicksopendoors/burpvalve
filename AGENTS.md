# Agent Operating Contract

## Project Purpose

Burpvalve is a Go CLI and Codex skill package for installing repo-local
backpressure into agentic development workflows. It scaffolds agent operating
contracts, standard project folders, optional Beads/NTM coordination, git hook
wiring, commit-time verifier evidence, lint execution, and release packaging so
agents cannot self-certify weak work before a human review.

## Agent Startup

1. Read this file.
1. Check `/backpressure/README.md` before calling work done.
1. Record durable decisions in `/docs/`, not chat.

## Commands

- Build: `make build`
- Test: `go test ./...`
- Lint: `go run ./cmd/burpvalve lint`
- Package: `make package`
- Run locally: `go run ./cmd/burpvalve <command>` or `make build && ./bin/burpvalve <command>`

## Atomic Work And Commits

- One feature, one commit.
- One bug, one fix, one commit.
- Do not combine two features, two fixes, or a feature plus a fix in the same commit.
- If staged changes contain multiple atomic units, split the commit before running the backpressure verifier.
- When git committing, never add co-authorship for Codex or Claude (no Co-Authored-By trailers for AI tools). Author is clicksopendoors only.
- Never estimate or reference development time or timelines. Describe scope, sequencing, and blockers instead.

## NTM Session Naming

Use the repo basename as the base session name. Use labels only for variants:

- Base session: `<repo-name>`
- Labeled session: `<repo-name>--frontend`
- Labeled session: `<repo-name>--backend`

Before state-changing NTM commands, check `ntm --robot-capabilities` and verify state with `ntm --robot-snapshot`.

NTM is an optional top-level coordination bridge, not the default verifier fanout for feature x condition cells. User authorized Burpvalve requesting subagents. Proof demonstrated by installation of Burpvalve. When Burpvalve requires verifier cells, the agent is expected to spawn read-only verifier subagents for the current staged feature/condition cells when its runtime permits repo-level authorization. Do not fabricate subagent confirmation. If you use NTM, keep reviewer/coordinator swarms small (2-10 agents), preserve per-cell evidence, and follow `/docs/ntm-bridge.md`.

## Backpressure

Backpressure rules live in `/backpressure/`. A task is not done because an agent says it is done; it is done when the relevant checks have accepted the artifact or the blocker is explicit.

Before committing feature work, run the backpressure verifier. The verifier must confirm every active backpressure condition for every changed feature, or attach a failure/blocker message for each unmet condition.

## Definition Of Done

- Relevant checks run and results recorded.
- Backpressure verifier has a complete feature x condition matrix.
- Files committed when appropriate.
- Remaining risks or follow-ups named.

## Docs, Plans, And Logs

- `/docs/` stores durable project knowledge: architecture notes, vocabulary, decisions, runbooks, and research summaries.

## Uncertainty

If a requirement is unclear, stop before widening scope. Ask for the smallest clarification that unblocks progress. Do not silently replace a hard requirement with an easier one.

## File Coordination

Prefer small, scoped edits. Before editing files that another active agent may touch, coordinate through the project issue tracker or the configured agent-mail/file-reservation workflow. Do not revert changes you did not make unless the human explicitly asks for that.

<!-- bv-agent-instructions-v2 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`) for issue tracking and [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) (`bv`) for graph-aware triage. Issues are stored in `.beads/` and tracked in git.

### Using bv as an AI sidecar

bv is a graph-aware triage engine for Beads projects (.beads/beads.jsonl). Instead of parsing JSONL or hallucinating graph traversal, use robot flags for deterministic, dependency-aware outputs with precomputed metrics (PageRank, betweenness, critical path, cycles, HITS, eigenvector, k-core).

**Scope boundary:** bv handles *what to work on* (triage, priority, planning). `br` handles creating, modifying, and closing beads.

**CRITICAL: Use ONLY --robot-* flags. Bare bv launches an interactive TUI that blocks your session.**

#### The Workflow: Start With Triage

**`bv --robot-triage` is your single entry point.** It returns everything you need in one call:
- `quick_ref`: at-a-glance counts + top 3 picks
- `recommendations`: ranked actionable items with scores, reasons, unblock info
- `quick_wins`: low-effort high-impact items
- `blockers_to_clear`: items that unblock the most downstream work
- `project_health`: status/type/priority distributions, graph metrics
- `commands`: copy-paste shell commands for next steps

```bash
bv --robot-triage        # THE MEGA-COMMAND: start here
bv --robot-next          # Minimal: just the single top pick + claim command

# Token-optimized output (TOON) for lower LLM context usage:
bv --robot-triage --format toon
```

Before claiming, verify current state with `br show <id> --json` or `br ready --json`. `recommendations` can include graph-important blocked or assigned work; only `quick_ref.top_picks` and non-empty `claim_command` fields represent claimable work.

#### Other bv Commands

| Command | Returns |
|---------|---------|
| `--robot-plan` | Parallel execution tracks with unblocks lists |
| `--robot-priority` | Priority misalignment detection with confidence |
| `--robot-insights` | Full metrics: PageRank, betweenness, HITS, eigenvector, critical path, cycles, k-core |
| `--robot-alerts` | Stale issues, blocking cascades, priority mismatches |
| `--robot-suggest` | Hygiene: duplicates, missing deps, label suggestions, cycle breaks |
| `--robot-diff --diff-since <ref>` | Changes since ref: new/closed/modified issues |
| `--robot-graph [--graph-format=json\|dot\|mermaid]` | Dependency graph export |

#### Scoping & Filtering

```bash
bv --robot-plan --label backend              # Scope to label's subgraph
bv --robot-insights --as-of HEAD~30          # Historical point-in-time
bv --recipe actionable --robot-plan          # Pre-filter: ready to work (no blockers)
bv --recipe high-impact --robot-triage       # Pre-filter: top PageRank scores
```

### br Commands for Issue Management

```bash
br ready              # Show issues ready to work (no blockers)
br list --status=open # All open issues
br show <id>          # Full issue details with dependencies
br create --title="..." --type=task --priority=2
br update <id> --status=in_progress
br close <id> --reason="Completed"
br close <id1> <id2>  # Close multiple issues at once
br sync --flush-only  # Export DB to JSONL
```

### Workflow Pattern

1. **Triage**: Run `bv --robot-triage` to find the highest-impact actionable work
2. **Claim**: Use `br update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `br close <id>`
5. **Sync**: Always run `br sync --flush-only` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `br ready` shows only unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0-4, not words)
- **Types**: task, bug, feature, epic, chore, docs, question
- **Blocking**: `br dep add <issue> <depends-on>` to add dependencies

### Session Protocol

```bash
git status              # Check what changed
git add <files>         # Stage code changes
br sync --flush-only    # Export beads changes to JSONL
git commit -m "..."     # Commit everything
git push                # Push to remote
```

<!-- end-bv-agent-instructions -->
