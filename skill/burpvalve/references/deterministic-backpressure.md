# Wiring Deterministic Backpressure

Use this reference when a user asks to add optional backpressure, tune lint rules,
tighten anti-reward-hacking checks, or make any Burpvalve condition more
deterministic.

## Operating Rule

Move each backpressure as far right as the project can support:

1. Written rule
2. Attestation prompt
3. Executable command
4. CI gate
5. Structural invariant

Do not quietly add dependencies, change CI, tighten destructive-operation gates,
or block new classes of work without user approval. Burpvalve is useful because
it makes pressure explicit; hidden pressure just creates surprise failures.

For agent automation, inspect with `burpvalve setup --json` and
`burpvalve config --json`. Apply mutations with explicit flags plus
`--force --json`, or with `--robots` and stdin JSON containing
`"confirm": true`. Do not use an interactive Bubble Tea flow as an agent unless
the user explicitly asked for a human-style walkthrough.

Do not fabricate verifier or subagent confirmation. If a deterministic check is
not wired yet and no read-only verifier actually ran for the current staged
payload, record the blocker instead of claiming a pass.

`burpvalve gate run` can collapse the mechanical gate ceremony when an
orchestrator or human supplies a prepared, hash-bound handoff. It does not make
prose policy deterministic by itself: the command still requires real verifier
responses, exact stage paths, and a current staged-payload hash. In v1, a
skipped `executable_conditions` phase means no extra condition command ran; add
explicit `lint_commands`, CI jobs, or later executable-condition configuration
before treating a policy as command-backed evidence.

## First Questions

Ask only the questions needed for the requested scope. For a broad setup, ask:

1. Which lint, format, type-check, and test commands should be authoritative for
   this repo?
2. Should these commands block local commits, CI, or both?
3. May I add or configure developer dependencies to make the checks executable?
4. Should formatting auto-fix happen automatically, or should the agent stop and
   report the command to run?
5. Which reward-hacking shortcuts should be explicitly forbidden here?
6. How deterministic should each condition be: prose only, attestation, command,
   CI gate, or structural invariant?

For anti-reward-hacking, offer concrete modes:

| Mode | Meaning | Example |
|---|---|---|
| evidence-only | Require proof for claims but avoid extra blockers. | A commit touching tests must include the exact test command and result. |
| no-shortcuts | Block known fake-completion patterns. | No deleting tests, relaxing assertions, using placeholders, or marking work done with failing checks. |
| high-assurance | Add deterministic scanners and CI checks where possible. | Search for skipped tests, placeholder code, generated fake fixtures, or changed thresholds. |

If the user gives a preference like "be strict" or "keep it light", translate it
into one of these modes and state what will change before editing files.

## Workflow

1. Inspect the target repo and run `burpvalve setup`.
2. Read `backpressure/manifest.yaml` and the relevant condition files.
3. Identify which rules are currently prose, attestation-only, executable, or CI-gated.
4. Ask the user the smallest useful set of questions from this reference.
5. Propose the exact files, commands, and gates you will add.
6. Edit non-destructively: append to condition files, preserve local project notes,
   and keep existing CI behavior unless the user approved a change.
7. Run the new commands locally when possible.
8. Run `burpvalve setup`, `burpvalve lint`, and any repo-native tests that were
   added or changed.
9. Summarize what is now deterministic, what still depends on attestation, and
   what remains a human review judgment.

## Manifest Commands

Executable lint, format, type-check, scanner, and test checks go in
`backpressure/manifest.yaml` under `lint_commands`.

Each command uses this shape:

```yaml
lint_commands:
  - id: go-test
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 120
```

Guidelines:

| Field | Guidance |
|---|---|
| `id` | Stable lowercase name, such as `go-test`, `tsc`, `ruff-check`, or `secret-scan`. |
| `command` | Exact shell command Burpvalve should run from the repo root. |
| `required` | `true` blocks `burpvalve lint`; `false` reports failure without blocking lint mode. |
| `paths` | Paths the command is meant to cover. Use `["."]` for repo-wide checks. |
| `timeout_seconds` | Positive timeout. Keep it long enough for normal local runs, short enough to catch hangs. |

Do not put aspirational commands in `lint_commands`. If the tool is not installed
or the command is not agreed on, keep it in a condition file as a proposed check.

## Default Backpressures

Use this table to turn Burpvalve's default conditions into stronger gates.

| Condition | Deterministic wiring | Still needs judgment |
|---|---|---|
| `lint-rules` | Add formatters, linters, type checks, static analysis, and scanner commands to `lint_commands`. | Whether the lint rule set is the right rule set. |
| `dry` | Add clone detectors, duplicate-test checks, package-boundary checks, or dependency graph checks. | Whether similar code is intentionally duplicated. |
| `anti-reward-hacking` | Add scanners for skipped tests, weakened assertions, placeholder code, changed thresholds, disabled hooks, fake fixtures, or "TODO instead of fix" patterns. | Intent. A shortcut can be legitimate if the user asked for it. |
| `one-function-one-test` | Add coverage, changed-function test mapping, or touched-package test commands. | Whether the test meaningfully covers the behavior. |
| `definition-of-done` | Require exact evidence fields in attestations and CI summaries. | Whether the product outcome is actually done. |
| `evidence-log` | Require log path, command output, screenshots, or artifact references in attestation. | Whether the evidence is sufficient for the risk. |
| `scope-control` | Add changed-path allowlists, branch or issue naming checks, max file count warnings, or generated-code exclusions. | Whether a larger change is justified. |
| `destructive-operations` | Require dry-run output, approval tokens, protected command wrappers, or review files before destructive commands. | Whether the destructive operation is the right operational call. |
| `data-integrity` | Run migrations forward and backward, seed validation, idempotency checks, and backup checks. | Whether the migration semantics are correct. |
| `security-boundaries` | Run secret scanning, SAST, auth fixtures, tenant-isolation tests, and dependency audits. | Whether the threat model is complete. |
| `visual-regression` | Run Playwright screenshots, image diff tools, mobile and desktop viewport checks. | Whether a visual change is desirable. |
| `performance-budget` | Run benchmarks, bundle-size checks, query-plan checks, or latency budget tests. | Whether a slower path is acceptable for the product. |
| `autonomy-boundary` | Add allowlists, deny lists, approval-file checks, or command wrappers for actions agents may not take alone. | When escalation is warranted. |

## Optional Backpressures

These are common additions. Add them only when they match the repo and the user
has agreed to the extra pressure.

| Optional pressure | How to wire it | Good fit |
|---|---|---|
| Compiler and type checks | Add `go test`, `cargo check`, `tsc --noEmit`, `mypy`, or equivalent to `lint_commands`. | Interface and shape failures. |
| Full test suite | Add the exact test command and require attestation output. | Behavior regression risk. |
| Property tests and fuzzing | Add fuzz/property commands with time limits and seeds. | Parsers, encoders, algorithms, state machines. |
| Browser automation | Add Playwright/Cypress smoke checks and artifacts. | Web flows and UI regressions. |
| Visual diff | Add screenshot capture and diff thresholds. | User-visible layout or rendering work. |
| Schema and contract checks | Add OpenAPI, protobuf, database schema, or migration contract validators. | API and data boundary changes. |
| Secret scanning and SAST | Add scanners as required local or CI commands. | Security-sensitive repos. |
| Reviewer agents | Require independent review notes or model-diverse review for risky changes. | Large diffs, security changes, migrations. |
| PR and CI monitoring | Require CI URL, conclusion, and failing-job summary before marking done. | Remote workflows. |
| Deploy, canary, rollback | Require deployment command, canary observation, rollback plan, and monitoring link. | Production release paths. |
| Formal specs or generated guards | Generate tests or validators from schemas, invariants, or policies. | Stable domains with strict contracts. |

## Anti-Reward-Hacking Examples

Good anti-reward-hacking pressure names the shortcut and the evidence that would
prove it did not happen.

Examples to propose to the user:

| Shortcut to forbid | Deterministic check | Attestation evidence |
|---|---|---|
| Deleting failing tests instead of fixing behavior. | Alert when test files are deleted or assertion counts drop. | Explain each removed test and link to replacement coverage. |
| Relaxing assertions to make tests pass. | Search diff for broad matchers, empty assertions, or changed thresholds. | Show before/after assertion intent. |
| Skipping or quarantining tests without approval. | Search for `.skip`, `skip`, `xit`, `describe.skip`, `t.Skip`, or quarantine tags. | Include user approval or issue link. |
| Claiming a check passed without running it. | Require exact command, exit code, and output path in attestation. | Paste the command result or artifact path. |
| Replacing real behavior with placeholders. | Search for TODO placeholders in changed runtime paths. | Explain why any placeholder is intentionally out of scope. |
| Disabling hooks or CI. | Alert on `.githooks`, workflow, or config changes. | Explain why the gate changed and who approved it. |

Prefer explicit examples from the repo over generic policy. A useful condition
file says "do not disable `payments_integration_test.go` without approval"; a
weak one says "do not cheat".

## Condition File Pattern

When making a condition more deterministic, append a short section like this to
the relevant `backpressure/*.md` file:

```markdown
## Project-Specific Deterministic Checks

- Required command: `go test ./...`
- Required evidence: paste the command, exit code, and relevant output path in
  the Burpvalve attestation.
- Blocked shortcuts: do not delete tests, weaken assertions, or skip test cases
  unless the user explicitly approved that scope change.
- Current limitation: this check proves the known test suite passes; it does not
  prove the implementation covers untested behavior.
```

The last line matters. A backpressure with no stated limitation invites false
confidence.

## Common User-Facing Explanation

When asked what this adds, say:

```text
I can make this pressure more deterministic in three layers:
1. written policy in backpressure/*.md;
2. executable commands in backpressure/manifest.yaml;
3. CI or hook enforcement so the repo rejects missing evidence.

The tradeoff is speed and setup cost. Stricter gates reduce review burden, but
they also make local work fail earlier and more often.
```

Keep the explanation concrete. Name the files, commands, and failure modes.
