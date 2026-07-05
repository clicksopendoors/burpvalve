package scaffold

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var expectedConditionIDs = []string{
	"lint-rules",
	"dry",
	"anti-reward-hacking",
	"one-function-one-test",
	"definition-of-done",
	"evidence-log",
	"scope-control",
	"destructive-operations",
	"data-integrity",
	"security-boundaries",
	"visual-regression",
	"performance-budget",
	"autonomy-boundary",
}

func TestScaffoldTemplatesExist(t *testing.T) {
	root := repoRoot(t)
	required := []string{
		"templates/AGENTS.md.tmpl",
		"templates/CLAUDE.md.orchestrator.tmpl",
		"templates/ORCHESTRATOR.md.tmpl",
		"docs/ntm-bridge.md",
		"templates/docs/README.md",
		"templates/docs/ntm-bridge.md",
		"templates/plans/README.md",
		"templates/log/README.md",
		"templates/log/backpressure/failed/README.md",
		"templates/backpressure/manifest.yaml",
		"templates/backpressure/README.md",
		"templates/backpressure/attestations/README.md",
		"templates/githooks/pre-commit",
	}
	for _, id := range expectedConditionIDs {
		required = append(required, "templates/backpressure/"+id+".md")
	}

	for _, rel := range required {
		path := filepath.Join(root, rel)
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("missing template %s: %v", rel, err)
		} else if info.IsDir() {
			t.Fatalf("template %s is a directory", rel)
		}
	}
}

func TestClaudeOrchestratorSkillPackageContract(t *testing.T) {
	root := repoRoot(t)
	base := "templates/claude/skills/burpvalve-orchestrator"
	required := []string{
		"SKILL.md.tmpl",
		"references/burpvalve-gate-choreography.md.tmpl",
		"references/verifier-fanout-and-attestations.md.tmpl",
		"references/agent-mail-and-file-coordination.md.tmpl",
		"references/ntm-pane-wake-discipline.md.tmpl",
		"references/beads-and-gate-window-operations.md.tmpl",
		"examples/gated-implementation-handoff.md.tmpl",
		"examples/verifier-disagreement-hold.md.tmpl",
		"examples/gate-window-release.md.tmpl",
		"scripts/pane_wake.py.tmpl",
		"scripts/attestation_summary.py.tmpl",
		"scripts/append_finding.py.tmpl",
		"SELF-TEST.md.tmpl",
		".MAINTAINER-CHECKS.md.tmpl",
	}
	for _, rel := range required {
		path := filepath.Join(root, base, filepath.FromSlash(rel))
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("missing skill package resource %s: %v", rel, err)
		} else if info.IsDir() {
			t.Fatalf("skill package resource %s is a directory", rel)
		}
	}

	skill := readTemplate(t, root, filepath.ToSlash(filepath.Join(base, "SKILL.md.tmpl")))
	for _, want := range []string{
		"---",
		"name: burpvalve-orchestrator",
		"category: other",
		"- ctx-cli",
		"- tool-git",
		"license: MIT",
		"distribution: public",
		"# burpvalve-orchestrator",
		"Core rule: coordinate evidence flow; do not manufacture evidence",
		"one atomic payload per gated commit",
		"Terminal wakes route attention only",
		"## Quick Start",
		"## Decision Tables",
		"## Reference Map",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("SKILL.md.tmpl missing %q", want)
		}
	}
	description := foldedYAMLValue(skill, "description")
	if description == "" || len(description) > 500 {
		t.Fatalf("description length = %d, want 1..500: %q", len(description), description)
	}
	for _, rel := range required {
		trimmed := strings.TrimSuffix(rel, ".tmpl")
		if rel == "SKILL.md.tmpl" {
			continue
		}
		if rel == ".MAINTAINER-CHECKS.md.tmpl" {
			if strings.Contains(skill, ".MAINTAINER-CHECKS.md") {
				t.Fatal("SKILL.md must not link maintainer checks")
			}
			continue
		}
		count := strings.Count(skill, "("+trimmed+")")
		if count != 1 {
			t.Fatalf("SKILL.md link count for %s = %d, want 1", trimmed, count)
		}
	}
}

func TestClaudeOrchestratorSkillPackageHasNoBinaryFiles(t *testing.T) {
	root := repoRoot(t)
	base := filepath.Join(root, "templates/claude/skills/burpvalve-orchestrator")
	if err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.ContainsRune(string(body), 0) {
			return fmt.Errorf("%s contains NUL byte", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeOrchestratorSkillScriptsAreSafeByDefault(t *testing.T) {
	root := repoRoot(t)
	base := "templates/claude/skills/burpvalve-orchestrator/scripts"
	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, fakeBin, "ntm", `#!/usr/bin/env bash
printf '%s\n' "$@" >> "$NTM_CALLS"
case "$1" in
  --robot-capabilities|--robot-snapshot) printf '{}\n' ;;
  --robot-send=*) printf '{"sent":true}\n' ;;
  *) printf '{}\n' ;;
esac
`)
	calls := filepath.Join(tmp, "ntm-calls.txt")
	env := append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "NTM_CALLS="+calls)
	for _, rel := range []string{"pane_wake.py", "attestation_summary.py", "append_finding.py"} {
		script := materializeScriptTemplate(t, root, tmp, filepath.ToSlash(filepath.Join(base, rel+".tmpl")), rel)
		cmd := exec.Command("python3", script, "--help")
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s --help failed: %v\n%s", rel, err, output)
		}
	}

	paneWake := filepath.Join(tmp, "pane_wake.py")
	cmd := exec.Command("python3", paneWake, "--pane", "8", "--message", "wake")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pane_wake dry-run failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "DRY-RUN") {
		t.Fatalf("pane_wake dry-run output missing DRY-RUN:\n%s", output)
	}
	callLog := readFile(t, tmp, "ntm-calls.txt")
	for _, want := range []string{"--robot-capabilities", "--robot-snapshot"} {
		if !strings.Contains(callLog, want) {
			t.Fatalf("pane_wake did not preflight %s; calls:\n%s", want, callLog)
		}
	}
	if strings.Contains(callLog, "--robot-send=burpvalve") {
		t.Fatalf("pane_wake sent wake without --execute:\n%s", callLog)
	}

	findings := filepath.Join(tmp, "findings.md")
	writeFile(t, tmp, "findings.md", "# Dogfood Findings\n")
	appendFinding := filepath.Join(tmp, "append_finding.py")
	cmd = exec.Command("python3", appendFinding, "--log", findings, "--title", "Packet friction")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("append_finding dry-run failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "DRY-RUN") {
		t.Fatalf("append_finding dry-run output missing DRY-RUN:\n%s", output)
	}
	if got := readFile(t, tmp, "findings.md"); got != "# Dogfood Findings\n" {
		t.Fatalf("append_finding mutated without --write:\n%s", got)
	}
	cmd = exec.Command("python3", appendFinding, "--log", findings, "--title", "Packet friction", "--write")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("append_finding --write failed: %v\n%s", err, output)
	}
	if got := readFile(t, tmp, "findings.md"); !strings.Contains(got, "Packet friction") {
		t.Fatalf("append_finding --write did not append:\n%s", got)
	}
}

func TestManifestConditionOrder(t *testing.T) {
	root := repoRoot(t)
	manifest := readTemplate(t, root, "templates/backpressure/manifest.yaml")
	conditionsBlock := strings.SplitN(manifest, "\nlint_commands:", 2)[0]
	re := regexp.MustCompile(`(?m)^  - id: ([a-z0-9-]+)$`)
	matches := re.FindAllStringSubmatch(conditionsBlock, -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, match[1])
	}

	if !reflect.DeepEqual(got, expectedConditionIDs) {
		t.Fatalf("condition order mismatch\ngot:  %#v\nwant: %#v", got, expectedConditionIDs)
	}
}

func TestRequiredTemplateHeadings(t *testing.T) {
	root := repoRoot(t)
	checks := map[string][]string{
		"templates/AGENTS.md.tmpl": {
			"# Agent Operating Contract",
			"## Project Purpose",
			"## Agent Startup",
			"## Commands",
			"## Beads",
			"Close delivery beads through the Burpvalve gate",
			"Admin-only `.beads/` housekeeping may batch",
			"## Taking Orders And Handoffs",
			"## Atomic Work And Commits",
			"## Burpvalve Gate Choreography",
			"## Verifier Work",
			"## NTM Session Naming",
			"not the default verifier fanout",
			"/docs/ntm-bridge.md",
			"## Verifier Orchestration",
			"Standing verifier authorization permits spawning",
			"It is never per-cell evidence",
			"## Backpressure",
			"## Definition Of Done",
			"## Docs, Plans, And Logs",
			"## Uncertainty",
			"## File Coordination",
		},
		"templates/ORCHESTRATOR.md.tmpl": {
			"# Orchestrator Operating Notes",
			"coder contract",
			"source of truth for project rules",
			"## Mission Pattern",
			"## Claude Route Relationship",
			"ordinary-agent route",
			"orchestrator-skill route",
			"## Conduct Rules",
			"## Tick Loop",
			"## Contact Mesh",
			"pre-approve the expected",
			"Finished work must never sit undelivered",
			"## Monitoring Discipline",
			"runtime `Working (` marker",
			"The pane tail is not the completion signal",
			"## Audit And Rollback Duties",
			"## Coordination Boundaries",
			"## Source Of Truth Links",
			"AGENTS.md#backpressure",
			"AGENTS.md#definition-of-done",
		},
		"templates/docs/README.md": {
			"# Docs",
			"Durable project knowledge lives here",
		},
		"templates/docs/ntm-bridge.md": {
			"# NTM Bridge Policy",
			"not the default fanout mechanism",
			"2-10 agents",
			"ntm --robot-snapshot",
		},
		"templates/plans/README.md": {
			"# Plans",
			"Actionable work from a plan should be converted into beads",
		},
		"templates/log/README.md": {
			"# Log",
			"Promote durable decisions to `/docs/` or `/plans/`",
		},
		"templates/log/backpressure/failed/README.md": {
			"# Failed Backpressure Attempts",
			"not passing commit attestations",
		},
		"templates/backpressure/README.md": {
			"# Backpressure",
			"prevent agents from self-certifying work",
		},
		"templates/backpressure/attestations/README.md": {
			"# Commit Backpressure Attestations",
			"Tracked passing commit attestations live here",
		},
		"templates/backpressure/lint-rules.md": {
			"# Lint Rules",
			"Suggested policy candidates",
			"Important distinction",
		},
		"templates/backpressure/dry.md": {
			"# DRY - Do Not Repeat Yourself",
			"The DRY subagent should check",
		},
		"templates/backpressure/anti-reward-hacking.md": {
			"# Anti-Reward-Hacking",
			"superficial success signals",
		},
		"templates/backpressure/one-function-one-test.md": {
			"# One Function One Test",
			"test granularity rule",
		},
		"templates/backpressure/definition-of-done.md": {
			"# Definition Of Done",
			"what \"done\" means",
		},
		"templates/backpressure/evidence-log.md": {
			"# Evidence Log",
			"what evidence agents must record",
		},
		"templates/backpressure/scope-control.md": {
			"# Scope Control",
			"prevent agents from expanding tasks",
		},
		"templates/backpressure/destructive-operations.md": {
			"# Destructive Operations",
			"risky commands and irreversible changes",
		},
		"templates/backpressure/data-integrity.md": {
			"# Data Integrity",
			"invariants for persisted data",
		},
		"templates/backpressure/security-boundaries.md": {
			"# Security Boundaries",
			"auth, secrets, tenant isolation",
		},
		"templates/backpressure/visual-regression.md": {
			"# Visual Regression",
			"screenshot, browser, responsive layout",
		},
		"templates/backpressure/performance-budget.md": {
			"# Performance Budget",
			"latency, memory, token, cost",
		},
		"templates/backpressure/autonomy-boundary.md": {
			"# Autonomy Boundary",
			"explicit human approval",
		},
	}

	for rel, needles := range checks {
		body := readTemplate(t, root, rel)
		for _, needle := range needles {
			if !strings.Contains(body, needle) {
				t.Fatalf("%s missing required text %q", rel, needle)
			}
		}
	}
}

func TestClaudeOrchestratorSkillNTMReferenceIncludesPollingStandard(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/claude/skills/burpvalve-orchestrator/references/ntm-pane-wake-discipline.md.tmpl")
	for _, want := range []string{
		"## Polling Standard",
		"runtime `Working (` marker",
		"absent on two consecutive polls",
		"The pane tail is never the completion signal",
		"Pollers must terminate loudly",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("ntm-pane-wake-discipline.md.tmpl missing %q", want)
		}
	}
}

func TestClaudeOrchestratorSkillAgentMailReferenceIncludesContactMesh(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/claude/skills/burpvalve-orchestrator/references/agent-mail-and-file-coordination.md.tmpl")
	for _, want := range []string{
		"## Contact Mesh Pre-Approval",
		"Immediately after Agent Mail registration",
		"workers to orchestrator",
		"implementer to verifiers and verifiers to implementer",
		"set_contact_policy open",
		"undelivered behind predictable pending contact approval",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("agent-mail-and-file-coordination.md.tmpl missing %q", want)
		}
	}
}

func TestOrchestratorTemplateCarriesAgentMailRegistrationMandate(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"templates/ORCHESTRATOR.md.tmpl",
		"internal/scaffold/templates/ORCHESTRATOR.md.tmpl",
	} {
		body := readTemplate(t, root, rel)
		rendered, err := renderOrchestratorTemplateBody(body, orchestratorTemplateData{})
		if err != nil {
			t.Fatalf("render %s: %v", rel, err)
		}
		assertContainsAll(t, rel, string(rendered),
			"## Agent Mail Registration",
			"Every sub-agent the orchestrator spawns or wakes",
			"must register with Agent Mail before starting",
			"`ensure_project` plus `register_agent`",
			"`macro_start_session`",
			"repo's absolute path as the project key",
			"Include this registration instruction in every dispatch brief and terminal",
			"wake. A pane or worker that has not registered",
			"not considered dispatched",
			"Workers must report their",
			"Agent Mail identity in completion messages",
		)
	}
}

func TestOrchestratorRouteTemplatesCarryRoleSplit(t *testing.T) {
	root := repoRoot(t)
	orchestratorNeedles := []string{
		"## Role Split",
		"orchestrator coordinates only",
		"does not implement, edit docs, run verifier",
		"execute commit-gate payload work itself",
		"Orchestrator scope is triage",
		"owner-decision capture",
		"Workers are the implementers and checkers",
		"preserve their actual evidence and provenance",
	}
	for _, rel := range []string{
		"templates/ORCHESTRATOR.md.tmpl",
		"internal/scaffold/templates/ORCHESTRATOR.md.tmpl",
	} {
		body := readTemplate(t, root, rel)
		rendered, err := renderOrchestratorTemplateBody(body, orchestratorTemplateData{})
		if err != nil {
			t.Fatalf("render %s: %v", rel, err)
		}
		assertContainsAll(t, rel, string(rendered), orchestratorNeedles...)
	}

	routeNeedles := []string{
		"Hard role split",
		"this Claude route coordinates",
		"Do not implement, edit docs, run verifier cells",
		"execute payload work",
		"Route implementation and checking to worker agents",
		"triage, decision capture, routing, review",
	}
	for _, rel := range []string{
		"templates/CLAUDE.md.orchestrator.tmpl",
		"internal/scaffold/templates/CLAUDE.md.orchestrator.tmpl",
	} {
		assertContainsAll(t, rel, readTemplate(t, root, rel), routeNeedles...)
	}

	skillNeedles := []string{
		"## Role Split",
		"When worker agents are available",
		"orchestrator does not implement, edit",
		"run verifier cells",
		"execute commit-gate payload work itself",
		"Workers",
		"are the implementers and checkers",
		"preserve their actual evidence and",
	}
	for _, rel := range []string{
		"templates/claude/skills/burpvalve-orchestrator/SKILL.md.tmpl",
		"internal/scaffold/templates/claude/skills/burpvalve-orchestrator/SKILL.md.tmpl",
	} {
		assertContainsAll(t, rel, readTemplate(t, root, rel), skillNeedles...)
	}
}

func TestOrchestratorRouteTemplatesCarryModelEffortTiering(t *testing.T) {
	root := repoRoot(t)
	orchestratorNeedles := []string{
		"## Model And Effort Tiering",
		"Reasoning effort follows cell or task type",
		"not the agent's role title",
		"chosen deliberately at spawn time",
		"`xhigh` for an implementer on the serial critical path",
		"judgment-cell verifiers",
		"`dry`, `anti-reward-hacking`, `scope-control`",
		"`security-boundaries`, and `definition-of-done`",
		"`medium` for",
		"mechanical-cell verifiers",
		"dogfooding C13",
		"Record each pane's model and effort in dispatch notes",
		"only between rounds, never mid-verification",
		"respawn at the right tier",
	}
	for _, rel := range []string{
		"templates/ORCHESTRATOR.md.tmpl",
		"internal/scaffold/templates/ORCHESTRATOR.md.tmpl",
	} {
		body := readTemplate(t, root, rel)
		rendered, err := renderOrchestratorTemplateBody(body, orchestratorTemplateData{})
		if err != nil {
			t.Fatalf("render %s: %v", rel, err)
		}
		assertContainsAll(t, rel, string(rendered), orchestratorNeedles...)
	}

	skillNeedles := []string{
		"## Model And Effort Tiering",
		"Set model and reasoning effort deliberately at spawn time",
		"Effort follows the",
		"cell or task type, not the role title",
		"critical-path implementer work uses",
		"`xhigh`",
		"judgment-cell verification uses `high`",
		"`anti-reward-hacking`, `scope-control`",
		"`definition-of-done`",
		"mechanical-cell verification plus lookup or inventory",
		"uses `medium`",
		"Record each pane's model and effort in dispatch notes",
		"Adjust running panes only between rounds",
	}
	for _, rel := range []string{
		"templates/claude/skills/burpvalve-orchestrator/SKILL.md.tmpl",
		"internal/scaffold/templates/claude/skills/burpvalve-orchestrator/SKILL.md.tmpl",
	} {
		assertContainsAll(t, rel, readTemplate(t, root, rel), skillNeedles...)
	}

	referenceNeedles := []string{
		"## Effort Tiers",
		"Set verifier model and reasoning effort explicitly",
		"Do not rely on runtime defaults",
		"Judgment-cell verifiers use `high`",
		"`dry`, `anti-reward-hacking`, `scope-control`",
		"`security-boundaries`, and",
		"`definition-of-done`",
		"Mechanical-cell verifiers use `medium`",
		"lookup work, and inventory work",
		"escalation verdict",
		"Record each pane's model and effort in dispatch notes",
		"never mid-verification",
		"respawn at the right tier",
	}
	for _, rel := range []string{
		"templates/claude/skills/burpvalve-orchestrator/references/verifier-fanout-and-attestations.md.tmpl",
		"internal/scaffold/templates/claude/skills/burpvalve-orchestrator/references/verifier-fanout-and-attestations.md.tmpl",
	} {
		assertContainsAll(t, rel, readTemplate(t, root, rel), referenceNeedles...)
	}
}

func TestOrchestratorTemplateReferencesAgentsWithoutDuplicatingGateRules(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/ORCHESTRATOR.md.tmpl")
	for _, want := range []string{
		"AGENTS.md#backpressure",
		"AGENTS.md#definition-of-done",
		"AGENTS.md#atomic-work-and-commits",
		"AGENTS.md#file-coordination",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("ORCHESTRATOR.md missing AGENTS link %q:\n%s", want, body)
		}
	}
	for _, forbiddenHeading := range []string{
		"## Backpressure",
		"## Definition Of Done",
		"## Atomic Work And Commits",
		"## File Coordination",
		"## Verifier Orchestration",
	} {
		if strings.Contains(body, forbiddenHeading) {
			t.Fatalf("ORCHESTRATOR.md duplicates forbidden heading %q:\n%s", forbiddenHeading, body)
		}
	}
	for _, forbiddenRule := range []string{
		"Backpressure verifier has a complete feature x condition matrix",
		"Relevant checks run and results recorded",
		"pass, not_applicable, fail, or unknown",
		"One feature, one commit",
		"Do not fabricate subagent confirmation",
	} {
		if strings.Contains(body, forbiddenRule) {
			t.Fatalf("ORCHESTRATOR.md duplicates forbidden rule vocabulary %q:\n%s", forbiddenRule, body)
		}
	}
}

func TestOrchestratorTemplateCarriesThroughputDoctrine(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/ORCHESTRATOR.md.tmpl")
	rendered, err := renderOrchestratorTemplateBody(body, orchestratorTemplateData{})
	if err != nil {
		t.Fatalf("render ORCHESTRATOR.md: %v", err)
	}
	assertContainsAll(t, "rendered ORCHESTRATOR.md", string(rendered),
		"## Throughput",
		"Monitor HEAD advancement, queue transitions, and dependency closures.",
		"Include the current queue order in every release wake and handoff message",
		"Treat a release wake as authorization",
		"self-promote",
		"Use verify-early, gate-fast choreography",
		"send verifier",
		"packets at park time",
		"Hold `GATE-WINDOW` only for mechanics",
		"send a superseding packet",
	)
}

func TestAgentsTemplateCarriesCoordinationAndGateGuidance(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/AGENTS.md.tmpl")
	rendered, err := renderAgentsTemplateBody(body, agentsTemplateData{
		Beads:        true,
		Backpressure: true,
		Docs:         true,
	})
	if err != nil {
		t.Fatalf("render AGENTS.md: %v", err)
	}
	got := string(rendered)
	for _, want := range []string{
		"## Taking Orders And Handoffs",
		"Treat owner, maintainer, and active orchestrator assignments as scoped work orders.",
		"Claim the bead before implementation",
		"Announce shared-file scope before editing files that other active agents may touch.",
		"## Burpvalve Gate Choreography",
		"Stage only the files owned by the current atomic payload.",
		"never use bypass flags",
		"## Verifier Work",
		"Verifiers are read-only reviewers",
		"Authorization to request verification is never evidence",
		"If two trusted verifiers disagree, hold the commit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered AGENTS.md missing %q:\n%s", want, got)
		}
	}
}

func TestScaffoldTemplatesCarryConductRules(t *testing.T) {
	root := repoRoot(t)
	authorRule := "When git committing, never add co-authorship for Codex or Claude (no Co-Authored-By trailers for AI tools). Author is clicksopendoors only."
	timelineRule := "Never estimate or reference development time or timelines. Describe scope, sequencing, and blockers instead."

	agentsBody := readTemplate(t, root, "templates/AGENTS.md.tmpl")
	renderedAgents, err := renderAgentsTemplateBody(agentsBody, agentsTemplateData{
		Beads:        true,
		Backpressure: true,
		Docs:         true,
	})
	if err != nil {
		t.Fatalf("render AGENTS.md: %v", err)
	}
	assertContainsAll(t, "rendered AGENTS.md", string(renderedAgents), authorRule, timelineRule)

	orchestratorBody := readTemplate(t, root, "templates/ORCHESTRATOR.md.tmpl")
	renderedOrchestrator, err := renderOrchestratorTemplateBody(orchestratorBody, orchestratorTemplateData{})
	if err != nil {
		t.Fatalf("render ORCHESTRATOR.md: %v", err)
	}
	assertContainsAll(t, "rendered ORCHESTRATOR.md", string(renderedOrchestrator),
		authorRule,
		timelineRule,
	)

	assertContainsAll(t, "CLAUDE.md.orchestrator.tmpl", readTemplate(t, root, "templates/CLAUDE.md.orchestrator.tmpl"),
		authorRule,
		timelineRule,
	)

	assertContainsAll(t, "orchestrator SKILL.md.tmpl", readTemplate(t, root, "templates/claude/skills/burpvalve-orchestrator/SKILL.md.tmpl"),
		authorRule,
		timelineRule,
	)
}

func TestClaudeOrchestratorSkillReferencesThroughputDoctrine(t *testing.T) {
	root := repoRoot(t)
	gate := readTemplate(t, root, "templates/claude/skills/burpvalve-orchestrator/references/burpvalve-gate-choreography.md.tmpl")
	assertContainsAll(t, "burpvalve-gate-choreography reference", gate,
		"## Verify Early, Gate Fast",
		"compute the Burpvalve staged-payload hash",
		"Keep `GATE-WINDOW` free while verdicts are pending.",
		"send a superseding packet",
		"Hold the window only for mechanics",
	)

	window := readTemplate(t, root, "templates/claude/skills/burpvalve-orchestrator/references/beads-and-gate-window-operations.md.tmpl")
	assertContainsAll(t, "beads-and-gate-window reference", window,
		"Include the current queue order in every release wake",
		"A release wake authorizes the next queued owner",
		"self-promote according to the",
		"recorded queue",
		"Verifier packets should usually go out at park time.",
	)
}

func TestAgentsTemplateClaudeOrchestratorRoutePointer(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/AGENTS.md.tmpl")
	withoutRoute, err := renderAgentsTemplateBody(body, agentsTemplateData{})
	if err != nil {
		t.Fatalf("render without route: %v", err)
	}
	if strings.Contains(string(withoutRoute), ".claude/skills/burpvalve-orchestrator/SKILL.md") {
		t.Fatalf("orchestrator skill pointer rendered without route:\n%s", withoutRoute)
	}

	withRoute, err := renderAgentsTemplateBody(body, agentsTemplateData{ClaudeOrchestratorRoute: true})
	if err != nil {
		t.Fatalf("render with route: %v", err)
	}
	if !strings.Contains(string(withRoute), ".claude/skills/burpvalve-orchestrator/SKILL.md") {
		t.Fatalf("orchestrator skill pointer missing:\n%s", withRoute)
	}
	if strings.Contains(string(withRoute), "ORCHESTRATOR.md` is supplemental") {
		t.Fatalf("sidecar text rendered without sidecar target:\n%s", withRoute)
	}

	withRouteAndSidecar, err := renderAgentsTemplateBody(body, agentsTemplateData{
		ClaudeOrchestratorRoute: true,
		Orchestrator:            true,
	})
	if err != nil {
		t.Fatalf("render with route and sidecar: %v", err)
	}
	if !strings.Contains(string(withRouteAndSidecar), "`ORCHESTRATOR.md` is supplemental project-local coordinator guidance") {
		t.Fatalf("sidecar relationship text missing:\n%s", withRouteAndSidecar)
	}
}

func TestOrchestratorTemplateDogfoodBlockIsConditional(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/ORCHESTRATOR.md.tmpl")
	withoutDogfood, err := renderOrchestratorTemplateBody(body, orchestratorTemplateData{})
	if err != nil {
		t.Fatalf("render without dogfood: %v", err)
	}
	if strings.Contains(string(withoutDogfood), "## Dogfood Findings") {
		t.Fatalf("dogfood block rendered by default:\n%s", withoutDogfood)
	}

	withDogfood, err := renderOrchestratorTemplateBody(body, orchestratorTemplateData{Dogfood: true})
	if err != nil {
		t.Fatalf("render with dogfood: %v", err)
	}
	for _, want := range []string{
		"## Dogfood Findings",
		"Log every issue, complication, and workflow friction",
		"Why it matters",
		"How-to-apply or proposed follow-up",
	} {
		if !strings.Contains(string(withDogfood), want) {
			t.Fatalf("dogfood block missing %q:\n%s", want, withDogfood)
		}
	}
}

func TestAgentsTemplateVerifierSectionIsConditional(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/AGENTS.md.tmpl")
	withoutVerifier, err := renderAgentsTemplateBody(body, agentsTemplateData{Backpressure: true})
	if err != nil {
		t.Fatalf("render without verifier: %v", err)
	}
	if strings.Contains(string(withoutVerifier), "## Verifier Orchestration") || strings.Contains(string(withoutVerifier), "Standing verifier authorization permits spawning") {
		t.Fatalf("verifier section rendered without configured verifier defaults:\n%s", withoutVerifier)
	}

	withVerifier, err := renderAgentsTemplateBody(body, agentsTemplateData{Verifier: true, Backpressure: true})
	if err != nil {
		t.Fatalf("render with verifier: %v", err)
	}
	for _, want := range []string{
		"## Verifier Orchestration",
		"Standing verifier authorization permits spawning read-only verifier subagents for backpressure checks.",
		"It is never per-cell evidence.",
		"Every feature x condition cell still needs real verifier output",
		"Do not fabricate subagent confirmation.",
	} {
		if !strings.Contains(string(withVerifier), want) {
			t.Fatalf("verifier section missing %q:\n%s", want, withVerifier)
		}
	}
}

func TestNTMBridgePolicy(t *testing.T) {
	root := repoRoot(t)
	doc := readTemplate(t, root, "docs/ntm-bridge.md")
	required := []string{
		"optional top-level coordination bridge",
		"not the default fanout mechanism",
		"User authorized Burpvalve requesting subagents",
		"Proof demonstrated by installation of Burpvalve",
		"expected to spawn read-only verifier subagents",
		"Do not fabricate subagent confirmation",
		"2-10 agents",
		"Do not use NTM as automatic per-cell pane fanout",
		"ntm --robot-snapshot",
		"attention state and relevant tail output",
		"every feature/condition cell still needs an explicit verdict",
	}
	for _, needle := range required {
		if !strings.Contains(doc, needle) {
			t.Fatalf("docs/ntm-bridge.md missing %q", needle)
		}
	}
	for _, rel := range []string{"docs/ntm-bridge.md", "templates/docs/ntm-bridge.md", "templates/AGENTS.md.tmpl"} {
		body := strings.ToLower(readTemplate(t, root, rel))
		for _, forbidden := range []string{
			"ntm is the default verifier fanout",
			"ntm is the default fanout",
			"default per-cell pane fanout",
			"automatic per-cell pane fanout is allowed",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains forbidden NTM default-fanout claim %q", rel, forbidden)
			}
		}
	}
}

func TestHookTemplateCallsGeneratedToolPath(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/githooks/pre-commit")
	required := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"if [[ -d ./cmd/burpvalve ]]",
		"BURPVALVE=(go run ./cmd/burpvalve)",
		"command -v burpvalve",
		"BURPVALVE=(burpvalve)",
		"elif [[ -x ./bin/burpvalve ]]",
		"BURPVALVE=(./bin/burpvalve)",
		"burpvalve source, PATH command, and repo-local shim are not available",
		"COMMIT_ARGS=()",
		"BURPVALVE_FEATURE",
		"BURPVALVE_RESPONSES",
		"BURPVALVE_AGENT",
		"BURPVALVE_MODEL",
		"\"${BURPVALVE[@]}\" commit",
		"\"${COMMIT_ARGS[@]}\"",
		"\"${BURPVALVE[@]}\" lint",
		".githooks/pre-commit.user",
	}
	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Fatalf("hook template missing %q", needle)
		}
	}
	if strings.Index(body, "\"${BURPVALVE[@]}\" commit") > strings.Index(body, "\"${BURPVALVE[@]}\" lint") {
		t.Fatal("hook must run commit before lint")
	}
}

func TestHookTemplatePassesBurpvalveEnvironmentAsCommitFlags(t *testing.T) {
	root := t.TempDir()
	hook := readTemplate(t, repoRoot(t), "templates/githooks/pre-commit")
	writeFile(t, root, ".githooks/pre-commit", hook)
	if err := os.Chmod(filepath.Join(root, ".githooks/pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd/burpvalve"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, root, "go", `#!/usr/bin/env bash
printf '%s\n' "$@" >> calls.txt
`)
	cmd := exec.Command(".githooks/pre-commit")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BURPVALVE_FEATURE=feature-123",
		"BURPVALVE_RESPONSES=responses.json",
		"BURPVALVE_AGENT=codex",
		"BURPVALVE_MODEL=gpt-5",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook failed: %v\n%s", err, output)
	}
	got := readFile(t, root, "calls.txt")
	want := strings.Join([]string{
		"run",
		"./cmd/burpvalve",
		"commit",
		"--feature",
		"feature-123",
		"--responses",
		"responses.json",
		"--agent",
		"codex",
		"--model",
		"gpt-5",
		"run",
		"./cmd/burpvalve",
		"lint",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("hook calls = %q, want %q", got, want)
	}
}

func TestHookTemplateUsesPathBurpvalveWhenNoSourceTreeExists(t *testing.T) {
	root := t.TempDir()
	hook := readTemplate(t, repoRoot(t), "templates/githooks/pre-commit")
	writeFile(t, root, ".githooks/pre-commit", hook)
	if err := os.Chmod(filepath.Join(root, ".githooks/pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "tools/burpvalve/README.md", "tool docs only\n")
	writeExecutable(t, root, "burpvalve", `#!/usr/bin/env bash
printf '%s\n' "$@" >> calls.txt
`)
	cmd := exec.Command(".githooks/pre-commit")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BURPVALVE_FEATURE=feature-123",
		"BURPVALVE_RESPONSES=responses.json",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook failed: %v\n%s", err, output)
	}
	got := readFile(t, root, "calls.txt")
	want := strings.Join([]string{
		"commit",
		"--feature",
		"feature-123",
		"--responses",
		"responses.json",
		"lint",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("hook calls = %q, want %q", got, want)
	}
}

func TestLintRulesSeparateWishlistFromEnforcement(t *testing.T) {
	root := repoRoot(t)
	body := readTemplate(t, root, "templates/backpressure/lint-rules.md")
	required := []string{
		"policy wishlist",
		"A rule is enforced only after this file names the command or analyzer that checks it.",
		"`lint-rules` as a backpressure condition asks a subagent",
		"`burpvalve lint` runs executable lint/format/static-analysis commands",
	}
	for _, needle := range required {
		if !strings.Contains(body, needle) {
			t.Fatalf("lint-rules template missing %q", needle)
		}
	}
}

func TestTemplatesDoNotClaimFakeEnforcement(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"templates/backpressure/lint-rules.md",
		"templates/backpressure/README.md",
		"templates/AGENTS.md.tmpl",
	} {
		body := strings.ToLower(readTemplate(t, root, rel))
		for _, forbidden := range []string{
			"already enforced",
			"automatically enforced",
			"guaranteed enforced",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains fake enforcement claim %q", rel, forbidden)
			}
		}
	}
}

func readTemplate(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

func foldedYAMLValue(body, key string) string {
	lines := strings.Split(body, "\n")
	prefix := key + ": >-"
	for i, line := range lines {
		if strings.TrimSpace(line) != prefix {
			continue
		}
		var parts []string
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				break
			}
			if !strings.HasPrefix(next, "  ") {
				break
			}
			parts = append(parts, strings.TrimSpace(next))
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func assertContainsAll(t *testing.T, label, body string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			t.Fatalf("%s missing %q:\n%s", label, needle, body)
		}
	}
}

func materializeScriptTemplate(t *testing.T, root, targetDir, templateRel, outputName string) string {
	t.Helper()
	body := readTemplate(t, root, templateRel)
	path := filepath.Join(targetDir, outputName)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
