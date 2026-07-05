# Open-Source Public Launch

## Status

Planning, revision 2. Revised 2026-07-02 after two independent Codex reviews
(Agent Mail messages 2274/2275; both "needs revision" — the anonymous install
command was not executable against the real installer, the Go module installer
conflicts with the module path, old release assets were unhandled, and license text was
missing from packaged artifacts). Explicitly the **last** plan in the round
(decision D5): executes only after land-working-set, release v0.1.2, the
issue-17 lint wizard, verifier orchestration, and bead-closure mode are all
delivered and the owner declares the feature set done. Beads for this plan
must carry dependencies on those plans' completion so it cannot start early.

## Source

Decision D5 in `docs/decisions-2026-07-02-review-round.md`.

## Ordering Decisions (resolve review ambiguities up front)

1. **Repo home is decided first.** Staying under `clicksopendoors` vs moving
   to another owner/org changes installer URLs, `BURPVALVE_REPO` defaults,
   badge URLs, release links, and (if Go's module installer is ever supported) the Go
   module path. Nothing else in this plan starts until this is decided.
2. **License is decided second** (owner decision; Apache-2.0 recommended, MIT
   the simpler alternative). Recorded in a decision doc.
3. **All launch docs/metadata/audit work lands before the visibility flip.**
   Exact flip order: prepare everything → cut the launch release
   before visibility changes → flip visibility → immediately run the anonymous verification
   suite → fix-forward anything it finds. A brief window where the just-cut
   release predates public visibility is accepted; the reverse order (flip
   first) is not, because pre-launch metadata must never be publicly
   visible.

## Non-Goals

- No feature work.
- The contribution policy stays (it is honest and OSS-compatible)
  — but its final sentence about pre-launch repository state is removed, and
  the tone pass ensures it reads as
  maintainer policy, not hostility.
- **Go's module installer is explicitly NOT a documented install path.** `go.mod`
  declares `module burpvalve` with matching internal imports; a module-path
  migration is out of scope for launch. If ever wanted, it is its own plan.
- No unsupported-platform claims: `release.yml` builds linux/darwin x amd64/arm64
  only; public docs must say exactly that.

## Work Items

### 1. License and packaging

- Add root `LICENSE` (chosen license). If Apache-2.0: decide NOTICE (default:
  no NOTICE file unless attribution requires one; record the decision).
- **`scripts/package-skill.sh` must include `LICENSE` (and `NOTICE` if any)
  in every release archive** — today it packages only the skill directory,
  binary, and VERSION, so release artifacts would claim a license they don't
  carry. Same for the skill package.
- Dependency-license review: enumerate go.mod dependencies (Bubble Tea/Lip
  Gloss stack is MIT-family, verify the full graph, e.g. via
  `go-licenses`), confirm compatibility, decide whether release artifacts
  need third-party notices.
- Update every metadata surface to the chosen license: README badge,
  `skill/burpvalve/SKILL.md` frontmatter (`license: <chosen>`,
  `distribution:` set to the JSM-valid public value — verify the allowed
  vocabulary against the JSM schema first, then `jsm validate` the result).

### 2. Pre-publication audit (full-surface, with explicit thresholds)

Owner-defined thresholds, recorded before auditing: secrets/tokens/creds =
always blockers; personal data beyond the owner's own name/username =
blocker; **local machine paths (`<local-machine-path>` examples), legacy
working-title naming, and agent names in history = accepted as
harmless history unless the owner vetoes** (they are not secrets; squashing
history to hide them costs provenance). Pending that veto, publish history
as-is.

Audit checklist (each item pass/fail against the thresholds):

- Full git history secret scan (gitleaks or equivalent) — includes old blobs
  of `bin/burpvalve` and `dist/` archives.
- Final `git ls-files` sweep for committed binaries/archives (D2 and the
  v0.1.2 plan should have removed `bin/burpvalve` and `dist/*.tar.gz`;
  verify none remain or regressed).
- All tracked docs and plans: `plans/GOAL.md`, `docs/CHANGELOG_RESEARCH.md`
  (contains local paths), `docs/release-install.md` (contains a
  machine-specific `--skills-dir` example and source-repo wording — rewrite
  for public), skill references, `SELF-TEST.md`.
- `.beads/` content: `issues.jsonl` (local `source_repo_path` values, legacy
  IDs), `config.yaml`, `metadata.json`.
- Demo sources AND generated assets (`docs/demos/*.sh|.tape|.txt`,
  `docs/demos/generated/*.svg|.gif`) scanned for usernames/hostnames/paths.
- GitHub surfaces that flip with the repo: ALL issues (open and closed,
  including comments and attachments — decide per finding: edit, delete
  comment, or accept), labels/milestones, old release notes and assets (see
  item 3), workflow run logs and artifacts (delete old runs if they leak
  context), repo About metadata (description/homepage/topics currently blank
  — write them as part of launch).
- Pre-flip cleanliness gate: `git status --short --ignored` reviewed; no
  runtime state (`.ntm/`, `.doctor/`, local logs) tracked or about to be.
- README D4 pass: remove remaining bare `->` chains from public prose.
- Issue-state reconciliation: every open issue either reflects a real public
  roadmap item or gets closed/updated before the flip (stale pre-launch
  roadmap issues must not become misleading public status).

### 3. Old releases (was unhandled — now explicit)

v0.1.0/v0.1.1 releases (and their tag source archives) become public with
the flip, and their packaged skills carry pre-launch metadata. Decision:
**delete the v0.1.0 and v0.1.1 GitHub releases and their assets before the
flip** (tags may remain — tag source trees just show the license-less
pre-launch source, consistent with published history; add one line to the
launch release notes noting earlier versions were pre-launch releases).
Owner may veto in favor of keeping them; keeping requires accepting
pre-launch-labeled artifacts remaining downloadable forever.

### 4. Public install path (rewritten to match the real installer)

`install.sh` has no default repo and, when stdin is a pipe, exits unless
`--yes`/`--skills-dir` are supplied. Two changes plus documentation:

- Add a compiled-in public default: `repo="${BURPVALVE_REPO:-<owner>/burpvalve}"`
  (set after the repo-home decision), so the minimal command works.
- Update the installer's failure text (then "check access to restricted
  release assets") for public-repo reality; keep the `gh`-if-present,
  direct-URL-fallback behavior (verified: `gh` is optional).
- README documents two paths, in this order: (a) download-inspect-run
  (fetch `install.sh` from the release tag — pinned, not `main` — read it,
  run it); (b) the one-liner `curl -fsSL <raw tag URL>/install.sh | bash -s
  -- --yes` for people who accept piped installs. Authenticated install
  instructions move out of README into `docs/release-install.md`.

### 5. Badges, README audience pass, community surface

- Status badge: retire the pre-launch status label; license badge: chosen
  license; CI badge (added in v0.1.2) now renders publicly — verify
  anonymously.
- README pass for a public audience; refresh VHS demos (manual
  generate/stage/commit — `readme-demos.yml` only uploads artifacts);
  de-slop/tone pass; remove the pre-launch tail sentence from About
  contributions.
- `SECURITY.md` with a concrete security channel: **enable GitHub Private
  vulnerability reporting and name it as the channel** (fallback: a
  dedicated email the owner confirms). "Report securely" without a channel
  is not acceptance-passable.
- Decide issues-only vs Discussions (recommendation: issues only, matching
  the contribution policy). No CONTRIBUTING.md.

### 6. Release, flip, verify

- Cut the launch release (version decided after repo-home/license decisions;
  includes LICENSE-bearing archives; audit generated release notes text
  before publishing).
- Verify the release package contents: license file present, updated skill
  frontmatter, `jsm validate` on the extracted skill, all nine assets.
- Flip visibility. Immediately verify anonymously (clean machine or
  container, no gh auth): clone; both documented install paths on linux
  amd64 (arm64/darwin as available); `burpvalve --version`; `burpvalve init`
  end-to-end in a fresh repo; badge rendering; release asset downloads;
  `gh`-absent AND `gh`-unauthenticated installer paths.
- Set repo About metadata (description, topics, homepage) at flip time.

## Acceptance Criteria

- Anonymous user on a clean machine installs and runs `burpvalve init` using
  only public instructions, via both documented paths, without `gh`.
- Root LICENSE exists AND every release archive/skill package contains it;
  all metadata surfaces (badge, frontmatter, About) agree; `jsm validate`
  passes on the public skill package.
- Audit checklist completed with recorded pass/fail per item and owner
  sign-off on the thresholds; no secrets in published history; old releases
  handled per item 3's decision.
- Go's module installer appears nowhere as a supported public install path.
- No public surface claims unsupported-platform support, community contributions beyond
  the stated policy, or enforcement that does not execute.
- SECURITY.md names a working security reporting channel.
- Open-issue list reconciled before the flip.

## Open Questions For The Owner

1. License: Apache-2.0 (recommended) vs MIT.
2. Repo home: stays under `clicksopendoors` or moves — decide FIRST.
3. Old releases: delete before flip (recommended) vs keep with pre-launch
   artifacts public.
4. History threshold veto: accept local paths/legacy naming in published
   history (recommended) vs squash-relaunch.
5. Launch announcement scope (out of tree; affects timing only).
