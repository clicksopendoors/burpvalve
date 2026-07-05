# Setup Pre-Commit Patterns For Burpvalve

This note captures useful patterns from Matt Pocock's `setup-pre-commit` skill
and how they should translate into Burpvalve.

## Source Pattern

Matt's `setup-pre-commit` skill sets up a Node-oriented commit hook:

- detect the package manager from lockfiles;
- install `husky`, `lint-staged`, and `prettier`;
- create a `.husky/pre-commit` hook;
- run formatting on staged files;
- run typecheck and tests when scripts exist;
- verify the hook and config files;
- commit the setup as a smoke test.

The useful part is the workflow shape, not Husky itself. Burpvalve already owns
the hook layer through `.githooks/pre-commit`, `burpvalve commit`, and
`burpvalve lint`.

## Transferable Patterns

### Ecosystem Detection

Detect the repo's tooling before proposing checks.

Useful signals:

- `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `bun.lockb` for Node.
- `go.mod` for Go.
- `Cargo.toml` for Rust.
- `pyproject.toml`, `requirements.txt`, or `uv.lock` for Python.

Burpvalve use: propose candidate `lint_commands` for
`backpressure/manifest.yaml` based on real project files.

### Only Add Real Checks

The pre-commit skill omits `typecheck` or `test` if the repo has no matching
script. Burpvalve should keep the same discipline.

Burpvalve rule:

- Do not add aspirational commands to `lint_commands`.
- If a command is not known to exist, keep it as a recommendation in
  `backpressure/lint-rules.md`.
- Move it into `backpressure/manifest.yaml` only after the exact command is
  confirmed.

### Manifest-Backed Commands

Husky-specific config should not be copied. Burpvalve's equivalent is a
manifest entry:

```yaml
lint_commands:
  - id: go-test
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 120
```

Scoped commands should use `run_directory` instead of embedding `cd ... &&`
inside generated commands. `run_directory` is repo-relative, cleaned
lexically, and must stay inside the project root. `paths` remain
repo-root-relative even when `run_directory` is set, and
`BACKPRESSURE_LINT_PATHS` uses those same repo-root-relative paths:

```yaml
lint_commands:
  - id: web-lint
    run_directory: "apps/web"
    command: "npm run lint"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 120
```

When a user explicitly declines scoped setup for some roots, record the reduced
coverage without weakening command status:

```yaml
lint_coverage:
  declined_roots: ["apps/web", "services/api"]
  declined_at: "2026-07-02"
```

Good candidate commands by ecosystem:

| Ecosystem | Candidate commands |
|---|---|
| Go | `go test ./...`, `go vet ./...`, `gofmt` check |
| Rust | `cargo fmt --check`, `cargo test`, `cargo clippy` |
| Node | package-manager test script, typecheck script, `prettier --check` |
| Python | `ruff check`, `pytest`, `mypy` or `pyright` if configured |

### Staged Or Touched Path Awareness

`lint-staged` runs tools only on changed files. Burpvalve should not blindly
copy that behavior, but the idea is useful for faster local pressure.

Burpvalve already passes changed command scope through:

```text
BACKPRESSURE_LINT_PATHS
```

Future commands can use that environment variable for narrow checks when the
tool supports path-limited runs. Repo-wide checks should still be preferred for
small projects or high-risk conditions.

### Verification Checklist

The pre-commit skill includes a simple setup verification checklist. Burpvalve
already has `burpvalve setup --json`, but the human-facing summary can borrow
this shape.

Useful setup checks:

- `.githooks/pre-commit` exists and is executable;
- Git `core.hooksPath` points at `.githooks`;
- `burpvalve` is available on `PATH` or `bin/burpvalve` exists;
- `backpressure/manifest.yaml` is valid;
- enabled conditions point to existing files;
- executable `lint_commands` exist when the repo has known checks;
- `.githooks/pre-commit.user` is preserved when a legacy hook existed.

### User Hook Escape Hatch

Burpvalve already has a better version of the "do not erase local hook logic"
pattern:

```bash
USER_HOOK=".githooks/pre-commit.user"
if [[ -x "$USER_HOOK" ]]; then
  "$USER_HOOK" "$@"
fi
```

Keep this pattern. It lets Burpvalve own the gate while preserving project
customization.

### Auto-Fix Should Be Opt-In

Matt's skill uses Prettier with `--write`. That is reasonable for a Node app
pre-commit hook, but Burpvalve should default to check-only behavior.

Reason:

- Burpvalve is an evidence and refusal layer.
- Silent code changes can alter the staged payload and invalidate evidence.
- Agents need clear feedback about what changed and why.

If auto-fix is supported later, make it explicit:

- ask before enabling;
- document it in `backpressure/lint-rules.md`;
- rerun `burpvalve commit` after any auto-fix changes the staged payload.

## Recommended Burpvalve Feature

Add an ecosystem detector that proposes manifest commands without silently
enforcing them.

Suggested flow:

1. Inspect project files.
2. Detect likely ecosystems and existing scripts.
3. Produce candidate `lint_commands`.
4. Ask the user which checks should block local commits, CI, or both.
5. Write confirmed commands into `backpressure/manifest.yaml`.
6. Leave unconfirmed ideas in `backpressure/lint-rules.md`.
7. Run `burpvalve lint` and report exact evidence.

This fits Burpvalve's existing rule:

1. Written rule
2. Attestation prompt
3. Executable command
4. CI gate
5. Structural invariant

## What Not To Copy

- Do not install Husky inside Burpvalve target repos.
- Do not create `.lintstagedrc` as a Burpvalve default.
- Do not auto-install Prettier or other developer dependencies without approval.
- Do not auto-add test/typecheck commands that have not been verified.
- Do not auto-format staged files unless the project explicitly opts in.

## Bottom Line

The best pattern to borrow is not "use Husky." It is:

1. detect the repo's tooling;
2. add only checks that actually exist;
3. wire them into the commit gate;
4. verify the hook setup;
5. preserve user hook logic;
6. prefer explicit evidence over silent fixes.
