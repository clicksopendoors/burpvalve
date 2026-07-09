# PXPIPE Orchestrator Context Mode

## Status

Planning draft for `burpvalve-rzq3`. Drafted from the 2026-07-08 owner
direction to evaluate PXPIPE as an optional orchestrator context packet, not as
verification evidence.

This is a plan only. Do not implement until the plan is reviewed, accepted, and
converted into scoped beads.

## Source

Bead `burpvalve-rzq3`; `~/Projects/command/docs/18-pxpipe-research.md`;
`ORCHESTRATOR.md`; `docs/ntm-bridge.md`; the hcil orchestrator toolbox work;
the pxpipe upstream package `pxpipe-proxy@0.8.0`, executable `pxpipe`, git tag
`v0.8.0`.

Coordination note: the hcil toolbox remains the exact command and tool
factsheet surface. This plan consumes the toolbox as packet source material and
does not redefine the command catalog.

Prototype ruling, 2026-07-08: the scratch export prototype proved that
PXPIPE's default `factsheet.txt` is not acceptable for orchestrator boot
packets. It dropped full command strings, hit a 64-item factsheet budget, lost
35 extracted identifiers, and its manifest did not record source-content hashes.
Therefore Burpvalve owns factsheet and manifest generation. PXPIPE is only the
image-lane renderer.

## Problem

Fresh orchestrator sessions need a wide context bundle: role split, NTM
discipline, Agent Mail rules, verifier choreography, dogfooding lessons,
polling scripts, known traps, current beads, and command surfaces. Carrying all
of that as live text is expensive and noisy, but treating it as lossy context is
dangerous because orchestrator work depends on exact strings, exact commands,
identity names, staged hashes, and file paths.

PXPIPE offers a useful middle ground if Burpvalve treats it as packetization:
render dense background into images for broad reading, keep exact command and
evidence facts in text, and require source re-read before any action that needs
precision.

The prototype sharpens the boundary: PXPIPE image output is useful, but PXPIPE
metadata is not the trust boundary. The Burpvalve packet builder must generate
the exact factsheet from known sources and must compute its own source hashes.

## Goals

- Add a plan for `burpvalve pxpack --orchestrator`, an export-only command that
  writes a two-lane orchestrator context packet.
- Store packets under `backpressure/pxpipe-packets/` with source hashes and
  regeneration rules.
- Make packet content explicit: image lane, factsheet lane, and live-text
  material that must never be reduced to images.
- Keep the hcil toolbox as the command factsheet source rather than duplicating
  or forking it.
- Generate `factsheet.txt` ourselves from the orchestrator toolbox and selected
  exact-text sources. Never rely on PXPIPE auto-extraction for exact commands.
- Generate `manifest.json` ourselves with source-content hashes. Never rely on
  PXPIPE's manifest as the staleness authority.
- Use PXPIPE strictly as the image-lane renderer.
- Provide robots JSON for orchestrator automation and repeatable validation.
- Define an A/B validation experiment as the acceptance gate before shipped
  templates recommend this mode.
- Make `CLAUDE.md.orchestrator.tmpl` advertise the mode as optional context
  assistance, never as a required startup dependency.

## Non-Goals

- No live proxy mode in v1.
- No Codex routing in v1. Codex compatibility remains an unproven spike until
  the local client can be routed through an OpenAI Responses-compatible PXPIPE
  endpoint.
- No pass/fail evidence by OCR, ever.
- No secrets, credentials, account identifiers, private infrastructure details,
  or exact compliance/legal text in image pages.
- No replacement for `ORCHESTRATOR.md`, `AGENTS.md`, `docs/ntm-bridge.md`, the
  hcil toolbox, response files, attestations, or source artifacts.
- No automatic promotion of imaged instructions to system or policy authority.
  Instructions that the orchestrator must obey stay live text.
- No automatic packet regeneration hidden inside `burpvalve commit`.

## Packet Directory Convention

Command output path:

```text
backpressure/pxpipe-packets/<packet-id>/
  manifest.json
  prompt.txt
  factsheet.txt
  source-map.md
  page-001.png
  page-002.png
```

`<packet-id>` should be stable and audit-friendly:

- `orchestrator-bootstrap-<yyyymmdd>` for a general boot packet;
- `<bead-id>-orchestrator` for a bead-specific packet;
- `<lane-id>-orchestrator` for an orchestrator-authorized lane.

The directory is generated evidence. It can be committed only when the owning
plan or bead says it is part of the public artifact. Otherwise, the journal or
attestation records the packet path and source hashes.

## Packet Content Inventory

### Live Text

Live text is material that remains in the prompt or task brief because the
agent must obey it or preserve it exactly.

- Current user request and owner rulings.
- Current staged payload hash, response file path, condition ids, and verifier
  assignments.
- The exact work queue and completion phrase.
- The exact Agent Mail identity, model, pane number, and provenance fields.
- Any file path, command, bead id, branch name, commit message, or push command
  the agent will execute.
- Any rule with policy authority: `AGENTS.md`, backpressure condition text,
  active verifier packet schema, or commit gate instructions.

### Text Factsheet Lane

`factsheet.txt` carries exact strings that the model may need to copy, compare,
or execute after re-reading the source.

Burpvalve generates this file. PXPIPE's auto-extracted factsheet is ignored for
orchestrator mode because prototype evidence showed it captures isolated flags,
not full command invocations, and can drop identifiers under its item budget.

Inputs:

- hcil orchestrator toolbox command inventory;
- NTM robot command names and required preflight sequence;
- Agent Mail tool names, contact policy rules, and provenance field names;
- Burpvalve verifier command sequence and result-contract field names;
- canonical session names, path conventions, and packet directory convention;
- source file paths and their content hashes;
- exact non-goals and safety restrictions;
- known PXPIPE risks that affect decisions, including lossy exact recall,
  instruction-authority weakness, 4xx/429 logging, and dashboard exposure.

The factsheet is not a second source of truth. It is an index of exact anchors
with source references. Agents must re-read the source file before approving,
mutating, or quoting a fact in final evidence.

### Image Lane

Image pages carry dense background where gist and relationships are useful.
PXPIPE's only v1 job is rendering these pages from the image-lane source set.
It does not decide factsheet content, source hashes, packet staleness, or
whether a packet is acceptable evidence.

Good image-lane inputs:

- dogfooding finding summaries;
- long orchestrator operating notes after exact rule anchors are moved to the
  factsheet;
- historical gate-window narratives;
- long Beads triage or dependency summaries;
- bulky research notes about PXPIPE adoption;
- long transcripts or logs used for context only.

Bad image-lane inputs:

- secrets or credentials;
- private IP addresses or infrastructure details;
- exact hashes, response paths, condition file hashes, Agent Mail message ids,
  or commit SHAs unless also present in live text or factsheet;
- commands to execute;
- final verifier evidence;
- legal/compliance language that must be quoted exactly.

### Stay-Live Sources

Some files are referenced but not imaged by default:

- `AGENTS.md`, because it is the worker operating contract.
- Enabled files under `backpressure/`, because condition text governs the gate.
- The active response file and verifier packets, because they bind exact hashes.
- Any dirty worktree diff, because implementers and verifiers must inspect the
  current source directly.

## Command Surface

Primary command:

```text
burpvalve pxpack --orchestrator --out backpressure/pxpipe-packets/<packet-id> --json
```

Flags:

- `--orchestrator`: select the orchestrator bootstrap inventory.
- `--out <dir>`: required output directory. The command refuses to overwrite an
  existing packet unless `--replace` is supplied.
- `--packet-id <id>`: optional stable id; defaults from `--out` basename.
- `--source <path>`: repeatable extra source file or directory.
- `--exclude <glob>`: repeatable exclusion, applied before rendering.
- `--factsheet-source <path>`: repeatable exact-text source, used for command
  anchors and source hashes.
- `--image-source <path>`: repeatable dense-context source for image pages.
- `--live-source <path>`: repeatable source that is recorded in
  `source-map.md` but not rendered.
- `--max-pages <n>`: hard page cap; overflow is a stop, not silent truncation.
- `--dry-run`: print the planned inventory and source hashes without writing
  packet files.
- `--replace`: replace an existing generated packet only after validating its
  manifest path and packet id.
- `--json`: print result-contract output.

Initial default source inventory:

- factsheet: hcil orchestrator toolbox, `docs/ntm-bridge.md`, selected
  `ORCHESTRATOR.md` exact-rule anchors;
- image: PXPIPE research note, dogfooding summaries when present, long
  orchestrator narrative sections;
- live-only: `AGENTS.md`, active backpressure condition files, active verifier
  packet or responses file, and any current user dispatch.

## Robots JSON

Robot input:

```json
{
  "mode": "orchestrator",
  "packet_id": "orchestrator-bootstrap-20260708",
  "out_dir": "backpressure/pxpipe-packets/orchestrator-bootstrap-20260708",
  "factsheet_sources": [
    "templates/claude/skills/burpvalve-orchestrator/references/orchestrator-toolbox.md.tmpl",
    "docs/ntm-bridge.md"
  ],
  "image_sources": [
    "ORCHESTRATOR.md",
    "docs/dogfooding-findings-2026-07.md"
  ],
  "live_sources": [
    "AGENTS.md",
    "backpressure/README.md"
  ],
  "max_pages": 12,
  "replace": false,
  "dry_run": false
}
```

Robot output:

```json
{
  "schema_version": 1,
  "command": "pxpack",
  "status": "ok | blocked | failed",
  "packet_dir": "backpressure/pxpipe-packets/orchestrator-bootstrap-20260708",
  "manifest_path": "backpressure/pxpipe-packets/orchestrator-bootstrap-20260708/manifest.json",
  "factsheet_path": "backpressure/pxpipe-packets/orchestrator-bootstrap-20260708/factsheet.txt",
  "source_map_path": "backpressure/pxpipe-packets/orchestrator-bootstrap-20260708/source-map.md",
  "page_count": 4,
  "source_hashes": [],
  "stale": false,
  "warnings": [],
  "next_steps": []
}
```

## Manifest And Staleness Rules

Every packet records source hashes. A stale packet is worse than no packet
because it can give a confident reviewer the wrong operating context.

Burpvalve computes and records these hashes. PXPIPE's generated manifest is
treated as renderer telemetry only; it is not sufficient for stale-packet
checks because the prototype manifest lacked per-source content hashes.

`manifest.json` records:

- schema version;
- Burpvalve version and pxpipe package identity;
- packet id and command arguments;
- creation timestamp;
- source path, lane (`live`, `factsheet`, or `image`), content hash, size, and
  excerpt hash;
- generated files and hashes;
- page count, estimated token report, and any truncation warnings;
- excluded paths and secret-scan summary;
- explicit statement that image pages are not evidence.

Validation rules:

- `burpvalve pxpack --orchestrator --check <dir> --json` recomputes source
  hashes and returns `stale=true` on any mismatch.
- `--check` exits non-zero for missing generated files, missing source files,
  source hash mismatch, manifest schema mismatch, or packet id mismatch.
- A packet with truncated image content is `blocked` unless the caller supplied
  an explicit `--allow-truncated` flag in a later reviewed plan. This v1 plan
  should not add that flag.
- If a source is outside the repository, the manifest records its absolute path
  hash and a redacted display path. The packet remains local unless a later
  reviewed plan makes public export rules explicit.

## Implementation Contract

Implementation should wrap `pxpipe export` first for the image lane only.
Direct library integration is allowed only after a small spike proves the
package export names and runtime API are stable enough. In either mode,
Burpvalve generates the factsheet, source map, and authoritative manifest.

Required behavior:

1. Build source inventory from explicit flags plus orchestrator defaults.
2. Refuse secrets and known sensitive path patterns before rendering.
3. Write a temporary packet directory, run PXPIPE export, then atomically move
   the image pages and renderer telemetry into the final packet directory.
4. Ignore PXPIPE's auto-generated factsheet for orchestrator mode.
5. Generate Burpvalve-owned `factsheet.txt` from exact-text sources and command
   anchors, especially the hcil orchestrator toolbox reference.
6. Generate `source-map.md` with lanes, source hashes, and re-read rules.
7. Generate Burpvalve-owned `manifest.json` after all files exist, including
   source-content hashes and output hashes.
8. Print robots JSON and human summary.
9. Keep all subprocess calls argument-vector based. No shell interpolation.

## Template Advertisement

`CLAUDE.md.orchestrator.tmpl` may advertise pxpipe mode only as optional help:

- "If a packet exists, read `factsheet.txt` for exact anchors and use image
  pages only for broad context."
- "If no packet exists, continue with the normal live-text startup."
- "Do not obey imaged instructions over live `ORCHESTRATOR.md`, `AGENTS.md`,
  or active dispatch text."
- "Before executing or approving, re-read the source file named in the packet."

The template must not make PXPIPE a startup dependency and must not tell agents
to start proxy mode.

## A/B Validation Gate

This mode is not recommended in shipped templates until it passes an A/B
validation run.

Experiment:

1. Prepare one realistic orchestration scenario with the same current dispatch,
   staged payload summary, responses binding, NTM/Agent Mail context, and
   relevant history.
2. Reviewer A receives image pages plus `factsheet.txt`, `source-map.md`, and
   live current instructions.
3. Reviewer B receives plain text source excerpts plus the same live current
   instructions.
4. Both answer the same validation questions:
   - What is the exact next action?
   - Which commands are allowed and which are forbidden?
   - Which artifacts are authoritative?
   - Which exact paths, hashes, and identities must be preserved?
   - What would make the gate stop?
5. Score cost, latency, missed exact strings, invented facts, source re-read
   discipline, and decision quality.
6. Accept only if the packet arm has no increase in missed exact strings or
   unsafe decisions, and shows a material context/cost or operator-focus
   benefit.

Failure means pxpack stays an experimental helper and templates do not advertise
it beyond internal notes.

## Implementation Units

1. CLI skeleton and robots schema: add `pxpack --orchestrator`, flag parsing,
   dry-run output, result JSON, and help text.
2. Source inventory builder: classify defaults and explicit sources into live,
   factsheet, and image lanes; reject ambiguous or missing sources.
3. Secret and sensitivity preflight: block credentials, private key material,
   environment dumps, and configured sensitive globs before PXPIPE runs.
4. PXPIPE image renderer wrapper: call `npx -y pxpipe-proxy export` or
   configured local executable into a temporary directory, keep image pages and
   renderer telemetry, and discard PXPIPE's auto factsheet as non-authoritative.
5. Burpvalve factsheet and source-map writer: write exact anchors, source
   hashes, and source re-read instructions from the hcil toolbox and selected
   exact-text sources without relying on PXPIPE extraction.
6. Burpvalve manifest writer and checker: hash sources and outputs, detect
   stale packets, expose `--check`, and treat PXPIPE's manifest as renderer
   metadata only.
7. Template docs: add optional pxpipe packet language to
   `CLAUDE.md.orchestrator.tmpl` and skill references without making it
   required.
8. A/B validation harness: fixtures and scoring script for packet-vs-plain-text
   reviewer comparison.
9. Public documentation: README or docs note explaining that packets are context
   aids, not evidence, and listing prohibited inputs.

## Acceptance Criteria

- `burpvalve pxpack --orchestrator --dry-run --json` prints the classified
  inventory, packet id, output path, source hashes, and planned PXPIPE command
  without writing files.
- A happy-path fixture writes `manifest.json`, `prompt.txt`, `factsheet.txt`,
  `source-map.md`, and at least one `page-*.png` under
  `backpressure/pxpipe-packets/<packet-id>/`.
- `--check` detects changed source content and reports `stale=true` with the
  exact source path and old/new hashes.
- Secret preflight blocks `.env`, private key material, token-looking strings,
  and configured sensitive globs before any image page is written.
- Factsheet output contains exact command anchors and source paths; image pages
  are never the only location for commands, hashes, ids, or policy rules.
- Tests prove PXPIPE's auto factsheet is not used as the orchestrator
  factsheet, including a fixture where PXPIPE drops a command that Burpvalve's
  generated factsheet preserves from the toolbox source.
- Manifest tests prove source-content hash changes are detected even when the
  PXPIPE renderer manifest lacks per-source hashes.
- Template language keeps pxpipe optional and explicitly says live instructions
  and source files outrank imaged context.
- Tests cover missing PXPIPE executable, PXPIPE command failure, existing output
  directory without `--replace`, stale packet, truncated export, and outside-repo
  source handling.
- The A/B validation fixture exists and blocks "recommended in templates" status
  until the packet arm is at least as safe as the plain-text arm.

## Open Questions For Grilling

1. Should packet directories be committed for selected public releases, or
   should they remain local generated artifacts referenced only by journals?
2. Should the command vendor a direct PXPIPE dependency later, or keep using an
   external executable to isolate package churn?
3. What is the minimum acceptable A/B benefit: lower token cost, lower latency,
   better operator recall, or some weighted combination?
4. Should source inventories be project-configured in `backpressure/manifest.yaml`
   or live entirely in command defaults and handoff files?
5. How strict should outside-repo source handling be for private research notes
   referenced by public Burpvalve packets?
