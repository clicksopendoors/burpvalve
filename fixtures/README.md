# Fixtures

Fixture repository blueprints for setup workflow tests live here.

The Go tests copy these directories into temporary workspaces before running
`check`, `init`, or `repair`. Symlinks, executable bits, and fake `.git`
directories are created by the test harness so the source fixtures stay
portable and simple.

- `empty-dir`: no scaffold files.
- `git-empty`: an empty git repository shape.
- `existing-agents`: user-owned `AGENTS.md` content.
- `regular-claude`: user-owned regular `CLAUDE.md`.
- `existing-beads`: pre-existing `.beads` state.
- `partial-backpressure`: user-owned partial backpressure content.
