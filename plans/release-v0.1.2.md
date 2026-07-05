# Release v0.1.2

## Status

Planning, revision 2. Revised 2026-07-02 after two independent Codex reviews
(Agent Mail messages 2266 and 2267; both verdicts "needs revision"). Convert
to beads only after this revision is accepted. Depends entirely on
`plans/land-working-set.md` completing: nothing here starts until the working
set is committed and `git status` is clean.

## Source

Decision D6 in `docs/decisions-2026-07-02-review-round.md`.

## Problem

`docs/CHANGELOG.md` ends at v0.1.1, but the landed working set adds a
substantial post-v0.1.1 surface (verifier policy/provenance, `verifier
prompts`, `explain`, `attestations list/show/latest/browse`, `beads
preflight`, `config`/`config init`, completion verify, lint enforcement
levels, recovery result contract). The changelog, badges, docs, and release
artifacts must catch up, and the release mechanics have sharp edges the
reviews identified: tracked `dist/` artifacts, tag-push triggering, and smoke
tests that can falsely pass against v0.1.1.

## Resolved Decisions (from review round)

- **Tracked `dist/` artifacts are untracked.** `dist/burpvalve_linux_amd64.tar.gz`
  and its `.sha256` are currently tracked even though `.gitignore` ignores
  `dist/` (v0.1.1 committed them). This release stops committing release
  archives to source, extending D2's no-committed-binaries rule: `git rm
  --cached dist/burpvalve_linux_amd64.tar.gz dist/burpvalve_linux_amd64.tar.gz.sha256`
  lands as a chore commit at the start of this plan. GitHub release assets are
  the sole canonical distribution artifacts. (Flagged for owner veto since it
  changes v0.1.1 practice.)
- **`internal/attestations/attestations.go` `ToolVersion = "0.1.0"` is a
  schema/tooling constant, not the binary version; it does NOT bump with
  v0.1.2.** Add a code comment saying so during work item 1 so future release
  agents stop wondering.
- **The v0.1.2 tag is annotated** (matching v0.1.0/v0.1.1).
- **Do not use `workflow_dispatch`** for this release; the tag-push path is
  the only sanctioned trigger.

## Non-Goals

- No license or visibility change in v0.1.2; preserve the then-current
  skill metadata until the launch plan (D5) performs the MIT/public flip.
- No new features between landing and tagging.
- No demo regeneration unless a demo is actively wrong (`readme-demos.yml`
  only uploads artifacts, it does not commit them; a refresh would be a
  manual generate/stage/commit step — deferred to launch).
- Issues #13, #14, #17 are NOT closed by this release.

## Work Items

1. **Changelog.** Add a v0.1.2 section to `docs/CHANGELOG.md` in the existing
   capability-organized style; update the scope-window paragraph. Source:
   landing-sequence commits and issue refs (#6, #8, #9, #10, #11, #12, #15,
   partial #13/#14/#16). Update `docs/CHANGELOG_RESEARCH.md` only with a
   short note that v0.1.2 was written from live history (no reconstruction
   needed); it remains primarily prior-release research. Add the ToolVersion
   comment (see Resolved Decisions).
2. **Docs version references.** `docs/release-install.md`: update pinned
   `--version v0.1.1` examples to v0.1.2. README: bump static release badge
   to v0.1.2; keep the Go badge minor-only style (`1.25`) and confirm it
   matches `go.mod`; add the dynamic CI badge
   (`.../actions/workflows/ci.yml/badge.svg?branch=main`) — note it renders
   only for authenticated viewers while the repo is not public, which is
   acceptable; "verify badge" means it renders for an authenticated viewer.
3. **Chore: untrack `dist/` artifacts** (see Resolved Decisions). One commit.
4. **Package and validate locally (pre-release).**
   - `make build VERSION=v0.1.2` (so a curious `./bin/burpvalve --version`
     shows v0.1.2, not `dev`; the packaged binary is the authoritative one).
   - `VERSION=v0.1.2 DIST_DIR=$(mktemp -d) ./scripts/package-skill.sh` — use a
     temp DIST_DIR so the repo stays clean.
   - `make check-size`; if it warns (>8 MiB), record the size in the
     changelog entry (that is the "release notes" location; `release.yml`
     uses generated notes with no custom body).
   - Validate the **extracted package**, not just source: unpack the archive,
     check `VERSION` file content, `jsm validate` the extracted skill, and
     run `scripts/bin/burpvalve --version` expecting `v0.1.2`.
   - Pre-release smoke: `bash ./install.sh --from-archive <temp tarball>
     --skills-dir <tmp> --bin-dir <tmp> --yes`, then `<tmp>/bin/burpvalve
     --version` = v0.1.2 and `setup --json` runs. (The README `--version
     latest` flow would falsely validate v0.1.1 at this point — do not use it
     pre-release.)
5. **Commit, push, tag, release.** Exact order: commit all release-prep
   changes; push `main`; create **annotated** tag `v0.1.2` on that commit;
   push the tag (`git push origin v0.1.2`) — `release.yml` triggers on the
   pushed `v*` tag only. Then verify `gh release view v0.1.2` lists the full
   expected asset set: four platform archives (linux/darwin x amd64/arm64),
   four per-archive `.sha256` files, and `checksums.txt` (install.sh's GitHub
   path requires `checksums.txt`).
6. **Post-release smoke (pinned).** Run the documented authenticated `gh api`
   install flow with `--version v0.1.2` (pinned, not `latest`) into temp
   dirs; assert installed `burpvalve --version` = v0.1.2 and `setup --json`
   works. Optionally repeat with `latest` to confirm propagation.
7. **Issue hygiene (verification pass, not first closure).** The landing plan
   closes #8/#9/#10/#11/#12/#15 with commit references as their units land;
   this item **verifies** each closed issue references its commit and adds
   the release reference. #6/#16 follow their verification-cell outcomes.
   Comment on #13/#14 pointing to their plans; leave them and #17 open.

## Acceptance Criteria

- `gh release list` shows v0.1.2 latest; the full nine-asset set is present.
- Pinned post-release install passes; installed binary reports v0.1.2.
- Extracted-package validation passed pre-release (VERSION file, jsm
  validate, packaged binary version).
- `docs/CHANGELOG.md` v0.1.2 section names every landed capability with issue
  refs; `docs/release-install.md` has no v0.1.1-pinned examples.
- README: release badge v0.1.2, CI badge present and rendering for
  authenticated viewers.
- No tracked files under `dist/`; working tree clean after release.
- Skill frontmatter still uses the v0.1.2 pre-launch metadata; the launch
  plan owns the MIT/public flip.
- #13/#14/#17 still open with boundary comments.

## Open Questions

None. (Demo refresh resolved: deferred to launch unless a demo is actively
wrong. Tracked-dist decision flagged above for owner veto.)
