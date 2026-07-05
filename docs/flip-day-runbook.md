# Flip-Day Runbook

This runbook is the owner-operated checklist for the public flip. It does not
authorize the launch, rename repositories, create repositories, delete releases,
change visibility, or publish announcements. It is used only after the owner
grants the launch permission required by D-OSL-5 and the final pre-flip gate
confirms that the scrubbed tree is ready.

## Sources Of Truth

- Decision record: `docs/decisions-2026-07-02-review-round.md`
  (`D-OSL-1` through `D-OSL-10`).
- Launch plan: `plans/open-source-launch.md`.
- Security-channel deferral: `SECURITY.md`.
- Launch umbrella: `burpvalve-osl-launch-umbrella-dla`.
- Final pre-flip gate: `burpvalve-osl-preflip-gate-557`.
- Anonymous verification: `burpvalve-osl-flip-anonymous-verify-3ab`.

## Non-Execution Rules

- No agent performs these actions without explicit owner flip authorization
  under D-OSL-5.
- No public repository creation, source repository rename, release deletion,
  visibility change, About metadata update, or announcement happens from this
  runbook alone.
- The public relaunch uses a fresh squash-relaunch initial commit from the
  scrubbed tree. It does not rewrite source history.
- Public commits are authored only as
  `clicksopendoors <michael-bltzr@users.noreply.github.com>`, with no AI
  co-author trailer.
- The launch announcement is quiet and README-only unless the owner gives a
  later contradictory ruling.

## Pending Owner Inputs

| Input | Owner | Owning beads | Decisions |
| --- | --- | --- | --- |
| Explicit launch permission for the visibility flip. | Owner | `burpvalve-osl-launch-umbrella-dla`, `burpvalve-osl-preflip-gate-557` | `D-OSL-5` |
| Replacement bead-ID prefix for the last pre-flip rename. | Owner | `burpvalve-fysf`, `burpvalve-osl-preflip-gate-557` | `D-OSL-3`, `D-OSL-7`, `D-OSL-13` |
| Confirmation that GitHub Private Vulnerability Reporting can be enabled and reached. | Owner | `burpvalve-osl-public-readme-community-utj`, `burpvalve-osl-preflip-gate-557` | `D-OSL-5`, `D-OSL-6` |

## Resolved Owner Inputs

| Input | Approved value | Decisions |
| --- | --- | --- |
| Source repository rename target. | `burpvalve-private` | `D-OSL-8` |
| Public About description. | `Repo-local backpressure for agentic development workflows.` | `D-OSL-11` |
| Public About topics. | `go`, `cli`, `developer-tools`, `git-hooks`, `ai-agents`, `verification`, `backpressure` | `D-OSL-11` |
| Public About homepage. | Blank unless a later real public URL is supplied. | `D-OSL-11` |
| Public Beads posture. | Keep `.beads/` public after scrub; ordinary lingo/agent workflow context is acceptable, but security-sensitive infrastructure detail is not. | `D-OSL-13` |

## Ordered Checklist

| Step | Action | Owner | Owning beads | Decisions |
| --- | --- | --- | --- | --- |
| 1 | Confirm the owner has granted launch permission and the final pre-flip gate is the active launch blocker. | Owner, launch coordinator | `burpvalve-osl-launch-umbrella-dla`, `burpvalve-osl-preflip-gate-557` | `D-OSL-5` |
| 2 | Confirm the current-tree scrub is green: no launch-blocking legacy pre-Burpvalve naming outside the pending bead-ID prefix rename, no local machine paths, no stale launch-policy wording, no stale demo wording, no tracked runtime metadata, and no public Beads data that exposes secrets, credentials, IP addresses, hostnames, non-public network details, or local infrastructure beyond normal Linux/OS/kernel context. | Launch coordinator, scrub owner | `burpvalve-fysf`, `burpvalve-osl-audit-tracked-files-9gq`, `burpvalve-osl-preflip-gate-557` | `D-OSL-3`, `D-OSL-7`, `D-OSL-10`, `D-OSL-13` |
| 3 | Confirm MIT licensing and package metadata are complete, including release archives that carry the license and public-facing metadata that names the public repo correctly. | Release owner | `burpvalve-osl-license-metadata-flt`, `burpvalve-osl-private-launch-release-juq` | `D-OSL-1`, `D-OSL-2`, `D-OSL-5` |
| 4 | Confirm public install documentation names the public repo and rejects Go's module installer as a supported install path. | Docs owner | `burpvalve-osl-public-install-docs-ezb`, `burpvalve-osl-public-readme-community-utj` | `D-OSL-1`, `D-OSL-5`, `D-OSL-8` |
| 5 | Confirm README and skill-facing docs are in quiet public-launch form: no broad announcement posture, no public issue-bundle filing, and no community surface beyond the approved issue-only path. | Docs owner, owner | `burpvalve-osl-readme-docs-rewrite-skills-0u59`, `burpvalve-osl-public-readme-community-utj` | `D-OSL-5`, `D-OSL-6` |
| 6 | Confirm `SECURITY.md` still points to GitHub Private Vulnerability Reporting and that the working-channel check remains deferred to the flip gate, not silently accepted earlier. | Security owner, owner | `burpvalve-osl-public-readme-community-utj`, `burpvalve-osl-preflip-gate-557` | `D-OSL-5`, `D-OSL-6` |
| 7 | Confirm old release handling is ready: v0.1.0 and v0.1.1 GitHub releases and assets are approved for deletion before the public flip, but have not been deleted before this execution step. | Release owner, owner | `burpvalve-osl-old-release-handling-8mt` | `D-OSL-4`, `D-OSL-5` |
| 8 | Confirm public fixture identity is consistent with the approved noreply address and no legacy placeholder-email fixture remains in the public tree where it would contradict the launch record. | Launch coordinator | `burpvalve-osl-audit-tracked-files-9gq`, `burpvalve-osl-preflip-gate-557` | `D-OSL-9`, `D-OSL-10` |
| 9 | Rename the current source repository to `burpvalve-private` to free `clicksopendoors/burpvalve`. | Owner | `burpvalve-osl-launch-umbrella-dla`, `burpvalve-osl-preflip-gate-557` | `D-OSL-1`, `D-OSL-5`, `D-OSL-8` |
| 10 | Delete the v0.1.0 and v0.1.1 GitHub releases and their assets before the public repository is created or made visible. | Owner or release owner under owner approval | `burpvalve-osl-old-release-handling-8mt` | `D-OSL-4`, `D-OSL-5` |
| 11 | Perform the bead-ID prefix rename as the last pre-flip scrub step, after the owner chooses the replacement prefix. Keep public `.beads/` data, including ordinary lingo/agent workflow context, but scrub security-sensitive infrastructure detail and stale evidence. Explicit Beads records remain authoritative; display-only metadata must not create ownership claims. | Tracker owner, owner | `burpvalve-fysf`, `burpvalve-osl-preflip-gate-557` | `D-OSL-3`, `D-OSL-7`, `D-OSL-13` |
| 12 | Create the new public `clicksopendoors/burpvalve` repository from the scrubbed tree as a fresh squash-relaunch initial commit authored by `clicksopendoors <michael-bltzr@users.noreply.github.com>`. Do not preserve source commit history in the public repository and do not add AI co-author trailers. | Owner | `burpvalve-osl-launch-umbrella-dla`, `burpvalve-osl-preflip-gate-557` | `D-OSL-1`, `D-OSL-5`, `D-OSL-7`, `D-OSL-8`, `D-OSL-9` |
| 13 | Enable GitHub Private Vulnerability Reporting and verify the `SECURITY.md` security reporting channel is active and reachable before treating the public launch as complete. | Owner, security owner | `burpvalve-osl-public-readme-community-utj`, `burpvalve-osl-audit-github-surfaces-n5x`, `burpvalve-osl-preflip-gate-557` | `D-OSL-5`, `D-OSL-6` |
| 14 | Set GitHub About metadata using the owner-approved D-OSL-11 description, topics, and blank homepage. | Owner | `burpvalve-osl-audit-github-surfaces-n5x` | `D-OSL-1`, `D-OSL-5`, `D-OSL-8`, `D-OSL-11` |
| 15 | Keep the announcement quiet and README-centered: no broad social launch, no upstream issue filing from local-only bundles, and no expansion beyond the approved public surfaces. | Owner, docs owner | `burpvalve-osl-public-readme-community-utj`, `burpvalve-osl-readme-docs-rewrite-skills-0u59` | `D-OSL-5`, `D-OSL-6` |
| 16 | Run the post-flip anonymous verification suite from a clean unauthenticated environment: clone, documented install paths, `burpvalve --version`, `burpvalve init`, release asset download, README badge rendering, missing-`gh` path, and unauthenticated-`gh` path. | Verification owner | `burpvalve-osl-flip-anonymous-verify-3ab`, `burpvalve-osl-public-install-docs-ezb`, `burpvalve-osl-private-launch-release-juq` | `D-OSL-1`, `D-OSL-5`, `D-OSL-7`, `D-OSL-8` |
| 17 | Fix or roll back any anonymous verification failure before considering the flip complete or expanding visibility beyond the quiet README surface. | Owner, verification owner | `burpvalve-osl-flip-anonymous-verify-3ab`, `burpvalve-osl-launch-umbrella-dla` | `D-OSL-5`, `D-OSL-7` |

## Completion Evidence

The flip is complete only when the launch coordinator records all of the
following evidence in durable project state:

- Owner launch authorization and any owner-supplied pending inputs.
- Final pre-flip gate result.
- Source repository rename confirmation.
- Old release deletion confirmation for v0.1.0 and v0.1.1.
- Bead-ID prefix rename confirmation.
- Public repository initial commit hash and author identity.
- PVR enablement and `SECURITY.md` channel verification result.
- About metadata values applied or an owner-approved reason they remain unset.
- Anonymous verification transcript and any fix-forward commits.
