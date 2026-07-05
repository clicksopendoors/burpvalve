# Changelog Research

## Scope

- Requested window: full repository history through current working tree.
- Repo: `https://github.com/clicksopendoors/burpvalve`
- Local path: `<repo>`
- Research date: 2026-06-21

## Version Spine

- Tags found at research time: none.
- GitHub releases found at research time: none.
  `gh release list --repo clicksopendoors/burpvalve --limit 100` returned no
  rows.
- Later update: `v0.1.0` was published on 2026-06-21 after this research pass.
- Current branch: `main`.
- HEAD at research time: `5d1c1cd89c5f5d3000aeec42d98fe3753dece9ee`.
- Working tree at research time: dirty, with 42 status lines.

## Evidence Commands

```bash
git remote -v
git for-each-ref refs/tags --sort=creatordate --format='%(refname:short)|%(creatordate:short)|%(objectname:short)|%(subject)'
git log --date=iso-strict --pretty=format:'%H|%ad|%s' --reverse --all
gh release list --repo clicksopendoors/burpvalve --limit 100
git show --stat --oneline --decorate --date=short --no-renames <commit>
git diff --stat
git diff --name-status
rg -n "burpvalve|burpvalve-commit|install-burpvalve|setup-project|registry preview|project registry|registry" -S .
```

## Commit Spine

| Commit | Date | Subject | Changelog Role |
|--------|------|---------|----------------|
| [`fb1d62f`](https://github.com/clicksopendoors/burpvalve/commit/fb1d62fc11072af3183091119140e2c4383c43a9) | 2026-06-20 | Initialize Burpvalve | Planning and Beads bootstrap. |
| [`bd617e0`](https://github.com/clicksopendoors/burpvalve/commit/bd617e03eddbfdf10e2bb34c54d90274210b30b7) | 2026-06-20 | Add final compile and commit-gate verification bead | Terminal verification workstream. |
| [`abf3234`](https://github.com/clicksopendoors/burpvalve/commit/abf3234f6d517330a2565cfe6d27a62f94bdea5b) | 2026-06-20 | Rename human approval backpressure to autonomy boundary | Vocabulary correction. |
| [`5d1c1cd`](https://github.com/clicksopendoors/burpvalve/commit/5d1c1cd89c5f5d3000aeec42d98fe3753dece9ee) | 2026-06-20 | Build burpvalve agent backpressure gate | First complete implementation landing. |

## Working Tree Themes

- CLI consolidation: deleted `cmd/burpvalve-commit`, moved commit/lint/CI routing
  into `cmd/burpvalve`, added Cobra, and expanded help tests.
- Packaging consolidation: `Makefile`, `scripts/package-skill.sh`, and
  `install.sh` now build, package, and shim one `burpvalve` binary.
- Init flexibility: `ApplyOptions` and CLI flags can skip Beads, NTM, Claude
  symlink, or `AGENTS.md`.
- Registry removal: deleted `internal/projectregistry`, removed registry fields
  from scaffold reports/results, and added an inspect regression test proving a
  local project registry is ignored and not mutated.
- Documentation alignment: release install docs and skill self-test now describe
  the one-binary package and `burpvalve -h` verification.

## Coverage Ledger

| Area | Evidence | Covered In Changelog |
|------|----------|----------------------|
| Tags/releases | `git for-each-ref` and `gh release list` returned none | Yes |
| Initial plan | `fb1d62f`, `plans/GOAL.md`, `.beads/issues.jsonl` | Yes |
| Verification bead | `bd617e0`, `.beads/issues.jsonl` | Yes |
| Vocabulary rename | `abf3234`, `plans/GOAL.md` | Yes |
| First implementation | `5d1c1cd` stat and Beads closure records | Yes |
| Current rename/CLI consolidation | `git diff --stat`, `git diff --name-status` | Yes, later released as `v0.1.0` |
| Registry removal | deleted files and regression test in working tree | Yes, later released as `v0.1.0` |

## Open Questions

- The next release tag was not known at research time. `v0.1.0` was published
  later on 2026-06-21.
- Authenticated GitHub release asset access depends on the caller's authentication
  path. The installer now prefers authenticated `gh release download` for
  access-controlled release assets, then falls back to direct release URLs.

## v0.1.2 Live Landing Note

- Added on 2026-07-02 from live release-window evidence: `git log
  v0.1.1..HEAD`, committed Burpvalve attestations, release-chain beads, and
  REST issue reads for #6, #8-#16. This note intentionally does not rebuild the
  earlier v0.1.0/v0.1.1 research.

## v0.1.3 Live Release Note

- Added on 2026-07-03 from live post-v0.1.2 evidence: `git log --oneline
  v0.1.2..HEAD`, closed Beads, committed attestations, and the owner-directed
  v0.1.3 release scope.
- Dogfood mode is a release blocker for the tag/package gate; final release
  validation must recheck the source window after that commit lands.
