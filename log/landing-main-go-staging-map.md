# Landing Main.go Staging Map

This file is the durable staging map for the Phase 2 `cmd/burpvalve/main.go`
landing work. It exists so later commits can split the current working tree by
feature unit instead of accidentally committing the whole 3,545-line `main.go`
diff at once.

Payload note: the plan once mentioned `log/landing-mainggo-staging-map.md`.
That is treated as a typo. The authoritative payload path is this file:
`log/landing-main-go-staging-map.md`.

This commit must stage this map only. It must not stage `cmd/burpvalve/main.go`
or any other source file.

## Source Snapshot

- Repository head used for this map: `7bc2172`.
- Mapped file: `cmd/burpvalve/main.go`.
- Current diff stat for that file: 1 file, 3,545 changed lines, 3,193
  insertions, 352 deletions.
- Current zero-context hunk count from
  `git diff --unified=0 -- cmd/burpvalve/main.go`: 142 hunks.
- Current top-level touched symbol count from the local overlap scan: 171 symbol
  or block entries. Several entries overlap the same large insertion hunk.

The zero-context hunk count is a reconciliation aid, not the staging unit. Hunk
88 is a single contiguous insertion from current lines 2189-3854 that contains
multiple Phase 2 features. Future source staging must split that area by the
function and type ranges listed below.

## Unit Legend

- unit 7: verifier provenance and policy model. Owns commit response-template
  plumbing, verifier response schema docs, and verifier response fields.
- unit 10: recovery contract and `explain` command. Owns `explain`, readable
  result-contract CLI output, and the small `lint`/`ci` main.go output plumbing
  that feeds `explain`.
- unit 11: config foundation. Owns `config`, `config init`, config merge/show
  helpers, strict config JSON loading, and reusable config-summary helpers.
- unit 12: setup/init readiness and repair. Owns setup/init/repair command
  behavior, scaffold-target defaults, git-init prompts, and scaffold robot input.
- unit 13: hook feature-context templates. No current `cmd/burpvalve/main.go`
  hunk is assigned to this unit; it should stage templates/tests only unless the
  working tree changes after this map.
- unit 14: verifier prompt generator. Owns `verifier prompts` CLI, profile
  flags, and robot docs for verifier packet generation.
- unit 15: attestation query and TUI. Owns `attestations` list/show/latest and
  any browse/TUI command registration in `main.go`.
- unit 16: beads delivery preflight. Owns `beads preflight`, `--bead`,
  `--bead-rationale`, bead metadata normalization, and bead metadata passed into
  commit artifacts.
- unit 17: completion verify and guided install. Owns completion install/verify
  commands, shell detection, completion robot output, and completion guide text.
- unit 19: arrow-chain sanitization. Owns only the paste-hostile arrow text in
  the command shim error inside `installCommandShim`.
- unit 20: E2E harness and residual test sweep. No current `cmd/burpvalve/main.go`
  hunk is assigned to this unit; it should stage tests only unless a later audit
  finds residual main.go changes not covered here.

Unit 9 is intentionally absent from this map because the reviewed Phase 2 plan
does not assign `main.go` ownership to the lint enforcement-level unit. The
current `main.go` lint hunks are assigned to unit 10 because they are JSON/result
contract plumbing used by `explain`, not the internal lint enforcement model.

## Staging Rules

1. Stage by current function or block range first, then by individual statement
   only for explicitly split blocks.
2. Do not stage all of hunk 88 at once. It spans unit 10, 11, 12, 14, 15, and
   16 code.
3. If a listed split cannot be staged cleanly with `git add -p`, stop before the
   affected source commit and ask for sign-off. Do not silently merge units.
4. If a unit is intentionally merged after sign-off, the commit must carry the
   relevant `--bead-rationale`.
5. Keep imports with the first unit that needs them to compile, unless a later
   source edit changes usage.
6. The map is a checklist, not permission to widen a later commit. Any source
   hunk not listed here is a blocker until the map is updated or clarified.

## Known Cross-Unit Overlaps

- unit 7 plus unit 16, current lines 2706-2744, 4035-4045, 4128-4139, 4271-4304,
  and 4915-4954: the commit command surface now carries both verifier
  `--responses-template` support and bead delivery metadata. Preferred split:
  unit 7 stages response-template and response-schema plumbing; unit 16 stages
  `--bead`, `--bead-rationale`, `normalizeCommitBeads`, and metadata fields. If
  this cannot be split mechanically, request user sign-off before merging the
  affected unit 7 and unit 16 source commits.
- unit 11 plus unit 12, current lines 4445-4883 and related helper ranges:
  setup/init/repair now consume config defaults while config foundation defines
  the config summary and value helpers. Preferred split: unit 11 stages config
  data/model display helpers and config-loading utility helpers; unit 12 stages
  the setup/init/repair control-flow that consumes those helpers. If compiler
  dependencies make that infeasible, the plan's expected merge pressure allows
  a proposed unit 11 plus unit 12 merge only after user sign-off and with
  `--bead-rationale`.
- unit 17 plus unit 19, current line 1679: `installCommandShim` belongs to unit
  17 except the exact error text `create command shim %s -> %s`, which belongs
  to unit 19. This is expected to be statement-level separable; no merge is
  proposed.
- shared `isInteractiveTerminal`, current lines 5124-5125: assign the variable
  seam to unit 11 because `config init` is the earliest mapped unit that needs
  it for testable wizard gating. Units 12, 15, and 17 may reuse it later.

## Hunk Ledger

The following ledger covers all 142 zero-context hunks from the current diff.
When a hunk is split, the owner is named as "split" and the exact symbol-level
ownership appears in the Symbol Ledger.

| Hunk(s) | Current line(s) | Owner | Note |
| --- | ---: | --- | --- |
| h1 | 15 | unit 16 | `time` import for beads preflight timestamp display. |
| h2 | 17 | unit 10 | `internal/attestations` import first needed by `explain`; later reused by unit 15. |
| h3 | 154 | unit 10 | root registration for `newExplainCommand`. |
| h4 | 160-162 | split | root registrations: verifier unit 14, attestations unit 15, beads unit 16. |
| h5 | 270-272 | unit 10 | robot docs use `CommandPath()` so nested command docs route correctly; unit 10 owns the shared robot-doc routing foundation. |
| h6 | 336 | unit 12 | setup robot-doc command-path alias. |
| h7 | 342-350 | unit 10 | `explain` robot input docs. |
| h8-h11 | 371-399 | unit 12 | init/repair robot docs gain `git_init` and full command-path aliases. |
| h12 | 402-437 | split | commit robot input docs: response template/schema to unit 7, bead fields to unit 16. |
| h13 | 440-450 | unit 16 | beads preflight robot input docs. |
| h14 | 456 | unit 10 | `ci` command-path robot-doc alias for result-contract surface. |
| h15 | 463-513 | split | verifier docs unit 14, attestations docs unit 15, config docs unit 11. |
| h16 | 516-524 | unit 17 | completion and completion verify robot input docs. |
| h17 | 534-600 | split | setup output unit 12, explain output unit 10, completion output unit 17. |
| h18 | 602-661 | split | config output unit 11, attestations output unit 15, beads output unit 16, verifier output unit 14. |
| h19 | 669-681 | split | setup/init/repair notes unit 12; explain notes unit 10. |
| h20 | 687-721 | split | config unit 11, completion unit 17, attestations unit 15, beads unit 16, verifier unit 14. |
| h21-h75 | 770-2020 | unit 17 with one unit 19 statement | completion command, install/verify, shell detection, and completion guide. Unit 19 owns only current line 1679. |
| h76-h80 | 2062-2084 | unit 12 | setup command flags, no-beads/no-ntm readiness options, robot dispatch. |
| h81-h87 | 2088-2187 | unit 10 | `explain` option/types/command/run wrapper. |
| h88 | 2189-3854 | split by symbol | giant insertion containing explain, init/repair constructors, commit constructor, lint/ci constructors, verifier prompts, attestations, beads, and config. Use Symbol Ledger. |
| h89-h99 | 3856-3959 | unit 11 | config JSON preview, config init write/path/load, strict JSON decode. |
| h100 | 3980-3981 | unit 12 | setup options for Beads/NTM skip controls. |
| h101 | 4004 | unit 12 | init `gitInit` option. |
| h102 | 4029 | unit 12 | repair `gitInit` option. |
| h103 | 4036-4052 | split | `commitOptions` response-template unit 7, bead fields unit 16, `configInitOptions` unit 11. |
| h104 | 4055-4067 | unit 11 | config-init robot input and injectable config prompt functions. |
| h105 | 4082-4097 | unit 17 | completion robot input/output structs. |
| h106 | 4105 | unit 12 | scaffold robot `git_init` input. |
| h107 | 4129-4137 | split | robot commit bead fields unit 16; response-template field unit 7. |
| h108-h109 | 4236, 4264 | unit 12 | apply robot `git_init` to init/repair options. |
| h110 | 4282-4287 | unit 16 | robot commit bead metadata ingestion. |
| h111 | 4293-4295 | unit 7 | robot commit `responses_template` ingestion. |
| h112 | 4313 | unit 10 | lint robot path now asks for JSON output, feeding explain/result contract. |
| h113-h119 | 4330-4410 | unit 17 | completion robot command dispatch and structured output. |
| h120 | 4437 | unit 10 | legacy lint mode forwards JSON flag into result-contract lint output. |
| h121-h122 | 4449-4464 | split | setup config consumption: unit 12 control-flow, unit 11 config summary attachment. |
| h123 | 4472-4587 | unit 11 | setup config summary and config value rendering helpers. |
| h124 | 4595 | unit 12 | scaffold mutation confirmation uses shared interactive seam. |
| h125 | 4632-4662 | unit 12 | maybe-ask git init prompt and predicate. |
| h126-h133 | 4758-4870 | split | init/repair config consumption: unit 12 control-flow, unit 11 config summary/default helper calls. See overlap notes. |
| h134 | 4884-4914 | unit 16 | normalize commit bead metadata and default CLI root. |
| h135 | 4916-4921 | unit 16 | runPreCommit normalizes bead metadata. |
| h136 | 4924-4934 | unit 7 | runPreCommit response-template mode. |
| h137 | 4938-4939 | unit 16 | runPreCommit passes bead metadata into pre-commit options. |
| h138-h139 | 4972-4981 | unit 10 | lint command emits JSON and human summary as result-contract input. |
| h140-h141 | 5044, 5051 | unit 12 | init/repair wizard gating uses testable interactive seam. |
| h142 | 5124-5125 | unit 11 | shared `isInteractiveTerminal` variable seam, first needed by config init. |

This ledger covers all zero-context hunks h1 through h142. Split hunks are
expanded below so every function or registration block has exactly one planned
owner.

## Symbol Ledger

### Imports And Root Command

| Current range | Owner | Function/block | Note |
| ---: | --- | --- | --- |
| 3-27 | split | import block | Stage `time` with unit 16; stage `internal/attestations` with unit 10. |
| 109-170 | split | `newRootCommand` | Stage registration line 154 with unit 10; line 160 with unit 14; line 161 with unit 15; line 162 with unit 16. Existing registrations not listed here remain with their prior owners. |

### Robot Help And Robot Docs

| Current range | Owner | Function/block | Note |
| ---: | --- | --- | --- |
| 257-286 | unit 10 | `robotHelpForCommand` | Shared `CommandPath()` routing for nested robot docs; later command docs depend on it. |
| 334-341 | unit 12 | `robotInputDoc`: setup cases | Setup alias and target input docs. |
| 342-349 | unit 10 | `robotInputDoc`: explain case | New explain input docs. |
| 350-398 | unit 12 | `robotInputDoc`: init and repair cases | Skip maps and `git_init` input. |
| 399-439 | split | `robotInputDoc`: commit case | Lines 402-403, 406-438 to unit 7 for response-template/schema; lines 404-405 to unit 16 for bead metadata. |
| 440-449 | unit 16 | `robotInputDoc`: beads case | Beads preflight arguments and flags. |
| 450-462 | unit 10 | `robotInputDoc`: lint and ci cases | Result-contract robot input aliases. |
| 463-472 | unit 14 | `robotInputDoc`: verifier prompts case | Verifier prompt flags/profile docs. |
| 473-489 | unit 15 | `robotInputDoc`: attestations cases | Attestation query filters and show argument. |
| 490-512 | unit 11 | `robotInputDoc`: config cases | Config show/init robot input schema. |
| 513-526 | unit 17 | `robotInputDoc`: completion cases | Completion and completion verify input schema. |
| 532-548 | unit 12 | `robotOutputDoc`: setup output | Setup readiness/report schema. |
| 549-562 | unit 10 | `robotOutputDoc`: explain output | Explain schema. |
| 563-600 | unit 17 | `robotOutputDoc`: completion output | Completion install/verify structured output. |
| 602-620 | unit 11 | `robotOutputDoc`: config output | Config show/init schemas. |
| 621-633 | unit 15 | `robotOutputDoc`: attestations output | Attestation query schema. |
| 634-647 | unit 16 | `robotOutputDoc`: beads output | Beads preflight schema. |
| 648-661 | unit 14 | `robotOutputDoc`: verifier prompts output | Verifier prompt response schema. |
| 667-674 | unit 12 | `robotNotes`: setup | Setup read-only and readiness recovery notes. |
| 675-680 | unit 10 | `robotNotes`: explain | Explain read-only/recovery notes. |
| 681-686 | unit 12 | `robotNotes`: init/repair | Robot mutation confirmation notes. |
| 687-692 | unit 11 | `robotNotes`: config init | Strict config JSON and confirmation notes. |
| 693-704 | unit 17 | `robotNotes`: completion | Completion precedence and verification notes. |
| 705-709 | unit 15 | `robotNotes`: attestations | Attestation query read-only notes. |
| 710-715 | unit 16 | `robotNotes`: beads | Beads preflight read-only and metadata notes. |
| 716-721 | unit 14 | `robotNotes`: verifier prompts | Verifier spawning and no-fabrication notes. |

### Completion And Arrow-Text Region

| Current range | Owner | Function/block | Note |
| ---: | --- | --- | --- |
| 744-797 | unit 17 | `newCompletionCommand` | Guided installer fallback, config-aware shell selection, subcommand registration. |
| 798-821 | unit 17 | `completionShellSelection`, `configuredCompletionShellSelection` | Config/detection source tracking for completion shell. |
| 822-847 | unit 17 | `configuredCompletionShell`, `printCompletion` | Completion shell default and raw script printing. |
| 848-868 | unit 17 | completion option structs | Install and verify options. |
| 869-934 | unit 17 | `newCompletionInstallCommand` | Config-aware install command, flags, confirmation, result printing. |
| 935-981 | unit 17 | `newCompletionVerifyCommand` | Read-only completion verification command and JSON output. |
| 982-1004 | unit 17 | completion plan/result structs | Install plan and mutation result state. |
| 1005-1084 | unit 17 | `completionVerifyReport`, `TextWithOptions` | Human and JSON completion verification report. |
| 1085-1163 | unit 17 | completion report row helpers | Text rendering helpers for completion reports. |
| 1164-1315 | unit 17 | completion wizard and confirmation helpers | Interactive guided install flow. |
| 1316-1373 | unit 17 | install plan rendering and construction | Deterministic install plan surface. |
| 1374-1508 | unit 17 | completion verification checks | Build verification report, shell selection, file/dir/script/rc checks. |
| 1509-1662 | unit 17 | path and command discovery/install helpers | PATH checks, next steps, apply install, ensure command path. |
| 1663-1683 | split | `installCommandShim` | Unit 17 owns symlink behavior; unit 19 owns only line 1679 arrow-chain text. |
| 1684-1765 | unit 17 | completion rc/path rc helpers | Shell startup and PATH file block management. |
| 1766-1810 | unit 17 | completion install result and inline config summary | Completion-specific human result rendering. |
| 1811-1864 | unit 17 | completion guide and shell-source label | Non-interactive guide and source labels. |
| 1865-1972 | unit 17 | default path, rc, bin, display, quoting helpers | Completion install defaults and shell quoting. |
| 1973-2022 | unit 17 | completion shell detection | Shell detection evidence and parent process inspection. |
| 2023-2038 | unit 17 | `normalizeCompletionShell` | Shell name normalization. |
| 1679 | unit 19 | `create command shim %s -> %s` text | Paste-hostile arrow-chain string. Stage this line separately from unit 17 if possible. |

### Setup, Explain, Commit, Verifier, Attestations, Beads, Config

| Current range | Owner | Function/block | Note |
| ---: | --- | --- | --- |
| 2040-2060 | existing/no new owner | `bindLegacyFlags` | Only context in the current diff; no current hunk assigned here. |
| 2061-2087 | unit 12 | `newSetupCommand` | Setup flags, JSON/robot dispatch, Beads/NTM skips. |
| 2088-2092 | unit 10 | `explainOptions` | Explain command options. |
| 2093-2123 | unit 10 | `explanation`, `explainBlocker` | Explain JSON schema. |
| 2124-2137 | unit 10 | `scaffoldMutationExplanationInput` | Explain parser shape for init/repair JSON. |
| 2138-2161 | unit 10 | `newExplainCommand` | Explain command registration, flags, examples. |
| 2162-2188 | unit 10 | `runExplain` | Explain runtime wrapper and error response. |
| 2189-2207 | unit 10 | `buildExplanation` | Explain source loading and artifact lookup. |
| 2208-2262 | unit 10 | `explainJSON` | Structured input routing for setup/init/repair/lint/commit/completion/attestations. |
| 2263-2288 | unit 10 | `explainSetup` | Setup explanation. |
| 2289-2316 | unit 10 | `explainLint` | Lint result explanation; pairs with unit 10 lint output plumbing. |
| 2317-2352 | unit 10 | `explainScaffoldMutation` | Init/repair explanation. |
| 2353-2367 | unit 10 | `explainCommit` | Commit gate explanation. |
| 2368-2411 | unit 10 | `explainCompletionVerify` | Completion verify explanation. |
| 2412-2423 | unit 10 | `validateExplainArtifact` | Attestation artifact validation for explain. |
| 2424-2465 | unit 10 | `explainArtifact` | Explain passing/blocked artifacts. |
| 2466-2499 | unit 10 | `recordFromArtifact` | Explain artifact-to-record fallback. |
| 2500-2515 | unit 10 | `appendUniqueStrings` | Explain helper for feature/bead IDs. |
| 2516-2520 | unit 10 | `looksLikeLegacyBeadID` | Explain helper for legacy bead ID fallback. |
| 2521-2532 | unit 10 | `baseExplanation` | Shared explanation baseline. |
| 2533-2571 | unit 10 | `printExplanation` | Human explain output. |
| 2572-2585 | unit 10 | `foundWord`, `firstString` | Explain/report text helpers. |
| 2586-2645 | unit 12 | `newInitCommand` | Init flags, targets, git-init, repo-bin/tool-doc skips. |
| 2646-2705 | unit 12 | `newRepairCommand` | Repair flags, targets, git-init, repo-bin/tool-doc skips. |
| 2706-2744 | split | `newCommitCommand` | Lines for `--responses-template` and response-template example to unit 7; `--bead` and `--bead-rationale` lines to unit 16. |
| 2745-2767 | unit 10 | `newLintCommand` | JSON/result-contract lint CLI surface. |
| 2768-2790 | unit 10 | `newCICommand` | JSON/result-contract CI CLI surface. |
| 2791-2798 | unit 14 | `verifierPromptOptions` | Verifier prompt options. |
| 2799-2811 | unit 14 | `newVerifierCommand` | Verifier parent command. |
| 2812-2834 | unit 14 | `newVerifierPromptsCommand` | Verifier prompt command flags/profile/filtering. |
| 2835-2853 | unit 14 | `runVerifierPrompts` | Build prompt packets and output JSON/text. |
| 2854-2871 | unit 14 | `printVerifierPrompts` | Manual packet printing. |
| 2872-2880 | unit 15 | `attestationQueryOptions` | Query filters including bead. |
| 2881-2908 | unit 15 | `newAttestationsCommand` | Attestation parent and interactive browse dispatch. |
| 2909-2917 | unit 15 | `bindAttestationQueryFlags` | Shared attestation query flags. |
| 2918-2934 | unit 15 | `newAttestationsListCommand` | Attestation list command. |
| 2935-2952 | unit 15 | `newAttestationsShowCommand` | Attestation show command. |
| 2953-2968 | unit 15 | `newAttestationsLatestCommand` | Latest attestation command. |
| 2969-3022 | unit 15 | `runAttestationsList`, `runAttestationsShow`, `runAttestationsLatest` | Attestation query execution. |
| 3023-3043 | unit 15 | `validateAttestationStatus`, `attestationQueryError` | Query validation and JSON errors. |
| 3044-3097 | unit 15 | `printAttestationList`, `printAttestationDetail` | Human attestation output. |
| 3098-3126 | unit 16 | beads preflight types | Options, report, bead summary. |
| 3127-3141 | unit 16 | `newBeadsCommand` | Beads parent command. |
| 3142-3178 | unit 16 | `newBeadsPreflightCommand` | Beads preflight flags/examples/output mode. |
| 3179-3236 | unit 16 | `buildBeadsPreflightReport` | Read-only preflight report and warnings. |
| 3237-3260 | unit 16 | `inspectBead` | `br show --json` integration. |
| 3261-3277 | unit 16 | `stagedPathNames` | Staged payload path list for preflight. |
| 3278-3305 | unit 16 | `hasFatalPreflightWarning`, `beadsPreflightNextSteps` | Warning classification and safe order text. |
| 3306-3365 | unit 16 | `printBeadsPreflight`, `shortValue`, `firstNonEmptyString`, `shortTime` | Human preflight output and formatting helpers. |
| 3366-3402 | unit 11 | `newConfigCommand` | Config show parent/default command. |
| 3403-3438 | unit 11 | `newConfigInitCommand` | Config init command, flags, examples. |
| 3439-3492 | unit 11 | `runConfigShow`, `configView`, `printConfigView`, `configFoundLabel` | Config report and display. |
| 3493-3524 | unit 11 | `runConfigInit` | Robot input and config-init entrypoint. |
| 3525-3585 | unit 11 | `shouldRunConfigInitWizard`, `runConfigInitWizard` | Interactive config-init flow. |
| 3586-3614 | unit 11 | `chooseConfigScope` | Global/project config scope prompt. |
| 3615-3633 | unit 11 | `readConfigFileIfExists` | Existing config load/validate. |
| 3634-3670 | unit 11 | `askConfigSections` | Config wizard orchestration. |
| 3671-3690 | unit 11 | `askInstallConfigSection` | Skills/bin defaults. |
| 3691-3731 | unit 11 | `askCompletionConfigSection` | Shell/completion defaults. |
| 3732-3764 | unit 11 | `askOutputConfigSection` | Color/confirmation defaults. |
| 3765-3818 | unit 11 | `askScaffoldConfigSection` | Init/repair scaffold default config. |
| 3819-3855 | unit 11 | `askOptionalText`, `askChoice`, `firstBool`, `boolPtr` | Config prompt helpers. |
| 3856-3961 | unit 11 | config JSON helpers | Preview, confirmation text, write path/load, strict decode. |

### Options, Robot Runners, Runtime Helpers

| Current range | Owner | Function/block | Note |
| ---: | --- | --- | --- |
| 3977-3983 | unit 12 | `setupOptions` | Beads/NTM setup skip options. |
| 3984-4009 | unit 12 | `initOptions` | Git init and scaffold target skips. |
| 4010-4034 | unit 12 | `repairOptions` | Git init and scaffold target skips. |
| 4035-4045 | split | `commitOptions` | `beads`/`beadRationale` unit 16; `responsesTemplate` unit 7. |
| 4046-4054 | unit 11 | `configInitOptions` | Config init state. |
| 4055-4067 | unit 11 | `robotConfigInitInput` and config prompt variables | Config robot schema and test seams. |
| 4068-4080 | unit 10 | `lintOptions`, `ciOptions`, `robotTargetInput` | Result-contract command input types. |
| 4081-4099 | unit 17 | `robotCompletionInput`, `robotCompletionOutput` | Completion robot schema. |
| 4100-4127 | unit 12 | `robotScaffoldInput`, `robotSkipInput` | Init/repair robot schema including `git_init`. |
| 4128-4139 | split | `robotCommitInput` | Bead fields unit 16; response-template field unit 7. |
| 4140-4177 | unit 10 | `robotLintInput`, `robotCIInput`, `decodeRobotInput` | Shared strict robot JSON decode for result-contract commands. |
| 4178-4215 | unit 12 | `runSetupRobots`, `runInitRobots`, `runRepairRobots` | Setup/init/repair robot mode dispatch. |
| 4216-4270 | unit 12 | `applyRobotScaffoldInputToInit`, `applyRobotScaffoldInputToRepair` | Robot skip and git-init option application. |
| 4271-4304 | split | `runCommitRobots` | Bead metadata fields unit 16; response-template field unit 7. |
| 4305-4329 | unit 10 | `runLintRobots`, `runCIRobots` | Result-contract robot dispatch. |
| 4330-4412 | unit 17 | `runCompletionRobots`, `completionRobotOutput` | Completion robot output and config-aware commands. |
| 4413-4444 | split | `runLegacyMode` | Init/repair legacy path remains unit 12; lint/ci forwarding lines belong to unit 10. |
| 4445-4471 | split | `runSetup` | Unit 12 owns setup inspection flow and Beads/NTM skips; unit 11 owns config load/summary attachment. |
| 4472-4587 | unit 11 | `setupConfigSummary`, `configSettingValue`, `configBoolValue`, `inspectOptionsFromConfig` | Config summary and defaults-to-inspect conversion. |
| 4588-4662 | unit 12 | `confirmScaffoldMutation`, `describeScaffoldTargets`, `maybeAskGitInit`, `needsGitInitPrompt` | Init/repair mutation confirmation and git-init prompt. |
| 4663-4736 | split | configured scaffold defaults helpers | Unit 11 owns config-default interpretation; unit 12 owns application to init/repair options. Split only if compiler-safe; otherwise request 11+12 merge sign-off. |
| 4737-4748 | unit 12 | `scaffoldTargetNamesWithRepoBin` | Repo-bin target expansion for init/repair. |
| 4749-4816 | split | `runInit` | Unit 12 owns init control-flow, wizard, git-init, scaffold apply; unit 11 owns config load/default summary attachment. |
| 4817-4883 | split | `runRepair` | Unit 12 owns repair control-flow, wizard, git-init, scaffold apply; unit 11 owns config load/default summary attachment. |
| 4884-4914 | unit 16 | `normalizeCommitBeads`, `defaultCLIRoot` | Bead metadata validation and default root helper. |
| 4915-4954 | split | `runPreCommit` | Unit 16 owns bead normalization/metadata; unit 7 owns response-template branch. |
| 4955-4971 | unit 10 | `runCI` | Result-contract CI output. |
| 4972-4987 | unit 10 | `runLint` | Result-contract lint JSON and human summary. |
| 4988-5038 | existing/no new owner | `encodeJSON`, color helpers | Context or previously owned helper code; no current hunk except surrounding references. |
| 5040-5053 | unit 12 | `shouldRunInitWizard`, `shouldRunRepairWizard` | Testable wizard gating for setup/init readiness. |
| 5054-5123 | unit 12 | init/repair wizard skip/result helpers | Scaffold wizard option/result application. |
| 5124-5125 | unit 11 | `isInteractiveTerminal` variable seam | Shared interactive test seam, first staged with config foundation. |

## No-Main-Go Units

- unit 13 has no mapped `cmd/burpvalve/main.go` hunk in the current working
  tree. If unit 13 later needs a main.go edit, it is outside this map and should
  be treated as new scope.
- unit 20 has no mapped `cmd/burpvalve/main.go` hunk in the current working
  tree. Its Phase 2 ownership is residual tests/E2E harness, not main.go source.

## Verification Checklist For This Map

Before committing this map bead, verify:

- `git status --short` shows this map as the only staged payload file.
- `test -s log/landing-main-go-staging-map.md` passes.
- `rg -n "unit 7|unit 10|unit 11|unit 12|unit 13|unit 14|unit 15|unit 16|unit 17|unit 19|unit 20" log/landing-main-go-staging-map.md` finds every required unit.
- `git diff --stat -- cmd/burpvalve/main.go` still reports the same 3,545-line
  working-tree source diff unless a later agent intentionally changes it.
- `git diff --unified=0 -- cmd/burpvalve/main.go | rg -n "@@" | wc -l` still
  reports 142 hunks unless a later agent intentionally changes it.
- Staged-slice verification uses `git stash push --keep-index --include-untracked
  -m land-main-map-verify`, runs `go test ./...`, and then restores the broader
  working tree with `git stash pop`.
