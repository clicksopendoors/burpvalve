package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestApplyInitCreatesGeneratedTreeAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	runner := successfulBeadsRunner()
	opts := applyOptions(t, runner, fakeLooker{"br": true})
	result, err := ApplyInitWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}

	required := []string{
		"AGENTS.md",
		"CLAUDE.md",
		".gitignore",
		"docs/README.md",
		"docs/ntm-bridge.md",
		"plans/README.md",
		"log/README.md",
		"log/backpressure/failed/README.md",
		"backpressure/manifest.yaml",
		"backpressure/README.md",
		"backpressure/attestations/README.md",
		"backpressure/lint-rules.md",
		"backpressure/dry.md",
		"backpressure/anti-reward-hacking.md",
		"backpressure/one-function-one-test.md",
		"backpressure/definition-of-done.md",
		"backpressure/evidence-log.md",
		"backpressure/scope-control.md",
		"backpressure/destructive-operations.md",
		"backpressure/data-integrity.md",
		"backpressure/security-boundaries.md",
		"backpressure/visual-regression.md",
		"backpressure/performance-budget.md",
		"backpressure/autonomy-boundary.md",
		".githooks/pre-commit",
		"tools/burpvalve/README.md",
		".beads",
	}
	for _, rel := range required {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing generated path %s: %v", rel, err)
		}
	}
	dest, err := os.Readlink(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be symlink: %v", err)
	}
	if dest != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink = %q, want AGENTS.md", dest)
	}
	hook, err := os.Stat(filepath.Join(root, ".githooks/pre-commit"))
	if err != nil {
		t.Fatal(err)
	}
	if hook.Mode()&0o111 == 0 {
		t.Fatal("pre-commit hook should be executable")
	}
	if _, err := os.Stat(filepath.Join(root, "bin/burpvalve")); !os.IsNotExist(err) {
		t.Fatalf("default scaffold should not create repo-local bin/burpvalve, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ORCHESTRATOR.md")); !os.IsNotExist(err) {
		t.Fatalf("default scaffold should not create ORCHESTRATOR.md, err=%v", err)
	}
	for _, want := range []string{"git init", "git config core.hooksPath"} {
		if !contains(result.Created, want) {
			t.Fatalf("created output missing %q: %#v", want, result.Created)
		}
	}
	if len(result.Created) != len(required)+2 {
		t.Fatalf("created count = %d, want %d: %#v", len(result.Created), len(required)+2, result.Created)
	}

	before := snapshotFiles(t, root)
	rerun, err := ApplyInitWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyInit rerun returned error: %v", err)
	}
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rerun changed file tree\nbefore=%v\nafter=%v", before, after)
	}
	if len(rerun.Created) != 0 {
		t.Fatalf("rerun should not create files: %#v", rerun.Created)
	}
	if len(rerun.Repaired) != 0 {
		t.Fatalf("rerun should not repair files: %#v", rerun.Repaired)
	}
	if len(rerun.Conflicts) != 0 {
		t.Fatalf("rerun should not report conflicts: %#v", rerun.Conflicts)
	}
}

func TestApplyInitRunsNTMQuickWithSnapshot(t *testing.T) {
	root := t.TempDir()
	runner := successfulBeadsRunner()
	result, err := ApplyInitWithOptions(root, applyOptions(t, runner, fakeLooker{"br": true, "ntm": true}))
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if result.NTM.Status != "registered" {
		t.Fatalf("ntm status = %#v", result.NTM)
	}
	wantCalls := []string{
		"br init",
		"br sync --import-only",
		"br doctor --json",
		"br config list",
		"br dep cycles",
		"br sync --flush-only",
		"ntm --robot-capabilities",
		"ntm quick " + filepath.Base(root),
		"ntm --robot-snapshot",
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
	if result.NTM.SnapshotOutput != "snapshot ok\nattention tail" {
		t.Fatalf("snapshot output truncated or missing: %q", result.NTM.SnapshotOutput)
	}
}

func TestApplyInitCanSkipOptionalAgentSubstrates(t *testing.T) {
	root := t.TempDir()
	runner := successfulBeadsRunner()
	opts := applyOptions(t, runner, fakeLooker{})
	opts.SkipAgents = true
	opts.SkipClaude = true
	opts.SkipBeads = true
	opts.SkipNTM = true

	result, err := ApplyInitWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}

	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", ".beads"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s should not be created when skipped, err=%v", rel, err)
		}
	}
	for _, rel := range []string{".githooks/pre-commit", "backpressure/manifest.yaml"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s should still be created: %v", rel, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "bin/burpvalve")); !os.IsNotExist(err) {
		t.Fatalf("default init should not create repo-local bin/burpvalve, err=%v", err)
	}
	wantSkipped := []string{
		"AGENTS.md (--no-agents)",
		".beads (--no-beads)",
		"CLAUDE.md (--no-claude)",
		"ntm quick (--no-ntm)",
	}
	if !reflect.DeepEqual(result.Skipped, wantSkipped) {
		t.Fatalf("skipped = %#v, want %#v", result.Skipped, wantSkipped)
	}
	if result.NTM.Status != "skipped" {
		t.Fatalf("ntm status = %#v, want skipped", result.NTM)
	}
	if len(result.Commands) != 0 {
		t.Fatalf("skip flags should not run br or ntm commands: %#v", result.Commands)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("skip flags should not invoke br or ntm runners: %#v", runner.calls)
	}
}

func TestApplyInitCanCreateOnlySelectedTargets(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, ApplyOptions{
		Runner:  successfulBeadsRunner(),
		Looker:  fakeLooker{"br": true},
		Targets: []ScaffoldTarget{TargetLog, TargetAttestations},
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	for _, rel := range []string{"log/README.md", "log/backpressure/failed/README.md", "backpressure/attestations/README.md"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("selected init target missing %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "backpressure/manifest.yaml", ".githooks/pre-commit", "bin/burpvalve", ".beads"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("scoped init should not create %s, err=%v", rel, err)
		}
	}
	if len(result.Commands) != 0 {
		t.Fatalf("scoped log/attestations init should not run commands: %#v", result.Commands)
	}
}

func TestApplyInitCanCreateOrchestratorTargetExplicitly(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, ApplyOptions{
		Targets: []ScaffoldTarget{TargetOrchestrator},
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "ORCHESTRATOR.md")); err != nil {
		t.Fatalf("explicit orchestrator target should create ORCHESTRATOR.md: %v", err)
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "backpressure/manifest.yaml", ".githooks/pre-commit", ".beads"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("scoped orchestrator init should not create %s, err=%v", rel, err)
		}
	}
	if !contains(result.Created, "ORCHESTRATOR.md") {
		t.Fatalf("result should report ORCHESTRATOR.md creation: %#v", result.Created)
	}
	body := readFile(t, root, "ORCHESTRATOR.md")
	if strings.Contains(body, "## Dogfood Findings") {
		t.Fatalf("orchestrator target should not include dogfood block by default:\n%s", body)
	}
}

func TestApplyInitCanCreateDogfoodOrchestratorTarget(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyInitWithOptions(root, ApplyOptions{
		Targets: []ScaffoldTarget{TargetOrchestrator},
		Dogfood: true,
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	body := readFile(t, root, "ORCHESTRATOR.md")
	for _, want := range []string{
		"## Dogfood Findings",
		"Log every issue, complication, and workflow friction",
		"docs/dogfooding-findings-YYYY-MM.md",
		"Why it matters",
		"How-to-apply or proposed follow-up",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dogfood ORCHESTRATOR.md missing %q:\n%s", want, body)
		}
	}
}

func TestApplyInitDefaultDoesNotCreateOrchestratorTarget(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true, "ntm": true}))
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "ORCHESTRATOR.md")); !os.IsNotExist(err) {
		t.Fatalf("default init should not create ORCHESTRATOR.md, err=%v", err)
	}
	if contains(result.Created, "ORCHESTRATOR.md") {
		t.Fatalf("result should not report ORCHESTRATOR.md creation by default: %#v", result.Created)
	}
}

func TestApplyInitAddsAgentsPointerOnlyWhenOrchestratorTargetActive(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyInitWithOptions(root, ApplyOptions{
		Targets: []ScaffoldTarget{TargetAgents},
	})
	if err != nil {
		t.Fatalf("ApplyInit agents returned error: %v", err)
	}
	agents := readFile(t, root, "AGENTS.md")
	if strings.Contains(agents, "ORCHESTRATOR.md") {
		t.Fatalf("AGENTS.md should not mention ORCHESTRATOR.md unless target is active:\n%s", agents)
	}

	withPointer := t.TempDir()
	_, err = ApplyInitWithOptions(withPointer, ApplyOptions{
		Targets: []ScaffoldTarget{TargetAgents, TargetOrchestrator},
	})
	if err != nil {
		t.Fatalf("ApplyInit agents+orchestrator returned error: %v", err)
	}
	agents = readFile(t, withPointer, "AGENTS.md")
	if !strings.Contains(agents, "Orchestrators should read `ORCHESTRATOR.md` first") {
		t.Fatalf("AGENTS.md missing orchestrator pointer when target active:\n%s", agents)
	}
	if _, err := os.Lstat(filepath.Join(withPointer, "ORCHESTRATOR.md")); err != nil {
		t.Fatalf("combined target should create ORCHESTRATOR.md: %v", err)
	}
}

func TestApplyInitCanCreateRepoLocalToolWhenSelected(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, ApplyOptions{
		BackpressureToolPath: fakeBackpressureTool(t),
		Targets:              []ScaffoldTarget{TargetTool},
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	tool, err := os.Stat(filepath.Join(root, "bin/burpvalve"))
	if err != nil {
		t.Fatal(err)
	}
	if tool.Mode()&0o111 == 0 {
		t.Fatal("repo-local fallback should be executable")
	}
	if got := readFile(t, root, ".gitignore"); !strings.Contains(got, "bin/") {
		t.Fatalf("repo-local bin should add bin/ to .gitignore:\n%s", got)
	}
	if !contains(result.Created, "bin/burpvalve") {
		t.Fatalf("result should report bin creation: %#v", result.Created)
	}
}

func TestApplyInitRendersAgentsForSelectedTargets(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, ApplyOptions{
		Runner:               successfulBeadsRunner(),
		Looker:               fakeLooker{"br": true},
		BackpressureToolPath: fakeBackpressureTool(t),
		Targets:              []ScaffoldTarget{TargetAgents},
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if !contains(result.Created, "AGENTS.md") {
		t.Fatalf("AGENTS.md not created: %#v", result.Created)
	}
	body := readFile(t, root, "AGENTS.md")
	for _, forbidden := range []string{
		"{{",
		"## Beads",
		"## NTM Session Naming",
		"## Backpressure",
		"## Docs, Plans, And Logs",
		"`/docs/`",
		"`/plans/`",
		"`/log/`",
		"`/backpressure/README.md`",
		"br ready --json",
		"backpressure verifier",
		"Backpressure verifier has a complete",
		"Bead closed or blocker recorded",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("scoped AGENTS.md should omit %q:\n%s", forbidden, body)
		}
	}
	for _, want := range []string{
		"# Agent Operating Contract",
		"## Agent Startup",
		"## Atomic Work And Commits",
		"- One feature, one commit.",
		"## Definition Of Done",
		"## Uncertainty",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scoped AGENTS.md missing %q:\n%s", want, body)
		}
	}
}

func TestApplyInitRendersAgentsForSkipFlags(t *testing.T) {
	root := t.TempDir()
	opts := applyOptions(t, successfulBeadsRunner(), fakeLooker{})
	opts.SkipBeads = true
	opts.SkipNTM = true
	opts.SkipDocs = true
	opts.SkipPlans = true
	opts.SkipLog = true
	opts.SkipBackpressure = true
	opts.SkipAttestations = true

	result, err := ApplyInitWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if !contains(result.Created, "AGENTS.md") {
		t.Fatalf("AGENTS.md not created: %#v", result.Created)
	}
	body := readFile(t, root, "AGENTS.md")
	for _, forbidden := range []string{
		"{{",
		"## Beads",
		"## NTM Session Naming",
		"## Backpressure",
		"## Docs, Plans, And Logs",
		"`/docs/`",
		"`/plans/`",
		"`/log/`",
		"`/backpressure/README.md`",
		"br ready --json",
		"backpressure verifier",
		"Backpressure verifier has a complete",
		"Bead closed or blocker recorded",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("skip-aware AGENTS.md should omit %q:\n%s", forbidden, body)
		}
	}
}

func TestApplyInitClaudeTargetCreatesAgentsDependency(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, ApplyOptions{
		Runner:  successfulBeadsRunner(),
		Looker:  fakeLooker{"br": true},
		Targets: []ScaffoldTarget{TargetClaude},
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("CLAUDE target should create %s: %v", rel, err)
		}
	}
	if target, err := os.Readlink(filepath.Join(root, "CLAUDE.md")); err != nil || target != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink = %q, err=%v", target, err)
	}
	if contains(result.Created, "backpressure/manifest.yaml") {
		t.Fatalf("CLAUDE target should not create backpressure files: %#v", result.Created)
	}
}

func TestApplyInitOrchestratorSkillRouteCreatesSkillWithoutSidecar(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, ApplyOptions{
		Targets:     []ScaffoldTarget{TargetClaude},
		ClaudeRoute: ClaudeRouteOrchestratorSkill,
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("orchestrator route should create AGENTS.md dependency: %v", err)
	}
	info, err := os.Lstat(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("orchestrator route should create CLAUDE.md: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("orchestrator route CLAUDE.md should be a generated regular file, not a symlink")
	}
	claude := readFile(t, root, "CLAUDE.md")
	if !strings.Contains(claude, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
		t.Fatalf("orchestrator CLAUDE.md missing skill pointer:\n%s", claude)
	}
	for _, rel := range []string{
		".claude/skills/burpvalve-orchestrator/SKILL.md",
		".claude/skills/burpvalve-orchestrator/SELF-TEST.md",
		".claude/skills/burpvalve-orchestrator/references/verifier-fanout-and-attestations.md",
		".claude/skills/burpvalve-orchestrator/examples/gate-window-release.md",
		".claude/skills/burpvalve-orchestrator/scripts/pane_wake.py",
	} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("orchestrator skill route missing %s: %v", rel, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "ORCHESTRATOR.md")); !os.IsNotExist(err) {
		t.Fatalf("orchestrator route should not create ORCHESTRATOR.md sidecar, err=%v", err)
	}
	if !contains(result.Created, "CLAUDE.md") ||
		!contains(result.Created, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
		t.Fatalf("orchestrator route creations not reported: %#v", result.Created)
	}
}

func TestApplyInitNoAgentsDoesNotCreateDanglingClaudeRoute(t *testing.T) {
	t.Run("missing AGENTS fails before mutation", func(t *testing.T) {
		root := t.TempDir()
		opts := applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true})
		opts.SkipAgents = true

		result, err := ApplyInitWithOptions(root, opts)
		if err == nil {
			t.Fatal("expected active Claude route without AGENTS.md to fail")
		}
		if _, err := os.Lstat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
			t.Fatalf("AGENTS.md should not be created, err=%v", err)
		}
		if _, err := os.Lstat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("CLAUDE.md should not be created without AGENTS.md, err=%v", err)
		}
		if len(result.Conflicts) != 1 || result.Conflicts[0].Path != "AGENTS.md" {
			t.Fatalf("AGENTS dependency conflict not reported: %#v", result.Conflicts)
		}
		if len(result.Created) != 0 || len(result.Repaired) != 0 {
			t.Fatalf("preflight conflict should happen before mutation: %#v", result)
		}
	})

	t.Run("existing AGENTS can still receive CLAUDE symlink", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "AGENTS.md", "# Existing Contract\n")
		opts := applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true})
		opts.SkipAgents = true

		result, err := ApplyInitWithOptions(root, opts)
		if err != nil {
			t.Fatalf("ApplyInit returned error: %v", err)
		}
		dest, err := os.Readlink(filepath.Join(root, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("CLAUDE.md should be symlink: %v", err)
		}
		if dest != "AGENTS.md" {
			t.Fatalf("CLAUDE.md symlink = %q, want AGENTS.md", dest)
		}
		if !contains(result.Created, "CLAUDE.md") {
			t.Fatalf("CLAUDE.md creation not reported: %#v", result.Created)
		}
	})
}

func TestApplyInitRunsBeadsCommandSequence(t *testing.T) {
	t.Run("absent beads initializes", func(t *testing.T) {
		root := t.TempDir()
		runner := successfulBeadsRunner()
		result, err := ApplyInitWithOptions(root, applyOptions(t, runner, fakeLooker{"br": true}))
		if err != nil {
			t.Fatalf("ApplyInit returned error: %v", err)
		}
		want := []string{"br init", "br sync --import-only", "br doctor --json", "br config list", "br dep cycles", "br sync --flush-only"}
		if !reflect.DeepEqual(result.Commands, want) {
			t.Fatalf("commands = %#v, want %#v", result.Commands, want)
		}
		if !contains(result.Created, ".beads") {
			t.Fatalf("expected .beads created: %#v", result.Created)
		}
	})

	t.Run("present beads verifies without reinitializing", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		runner := successfulBeadsRunner()
		result, err := ApplyInitWithOptions(root, applyOptions(t, runner, fakeLooker{"br": true}))
		if err != nil {
			t.Fatalf("ApplyInit returned error: %v", err)
		}
		want := []string{"br doctor --json", "br config list", "br dep cycles", "br sync --flush-only"}
		if !reflect.DeepEqual(result.Commands, want) {
			t.Fatalf("commands = %#v, want %#v", result.Commands, want)
		}
		if contains(result.Commands, "br init") {
			t.Fatal("existing .beads should not be reinitialized")
		}
	})
}

func TestApplyInitReportsBeadsDoctorFailureAsConflict(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := successfulBeadsRunner()
	runner.errors["br doctor --json"] = os.ErrInvalid
	runner.results["br doctor --json"] = CommandResult{Stdout: `{"workspace_health":"degraded"}`}

	result, err := ApplyInitWithOptions(root, applyOptions(t, runner, fakeLooker{"br": true}))
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if len(result.Conflicts) == 0 {
		t.Fatalf("expected degraded doctor conflict: %#v", result)
	}
	if !strings.Contains(result.Conflicts[0].Message, "workspace health degraded") {
		t.Fatalf("unexpected conflict: %#v", result.Conflicts)
	}
}

func TestApplyInitReportsMissingBRAsConflict(t *testing.T) {
	root := t.TempDir()
	result, err := ApplyInitWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{}))
	if err == nil {
		t.Fatal("expected missing br conflict error")
	}
	if len(result.Conflicts) == 0 {
		t.Fatalf("expected missing br conflict: %#v", result)
	}
	if !strings.Contains(result.Conflicts[0].Message, "br executable unavailable") {
		t.Fatalf("unexpected conflict: %#v", result.Conflicts)
	}
	if result.Status != "partial_success" || !result.Fatal || !result.PartialSuccess || len(result.NextSteps) == 0 {
		t.Fatalf("missing structured recovery fields: %#v", result)
	}
}

func TestApplyInitConfiguresGitHooksPath(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	result, err := ApplyInitWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if !contains(result.Created, "git config core.hooksPath") {
		t.Fatalf("expected git hooksPath creation in %#v", result.Created)
	}
	output := runGit(t, root, "config", "--get", "core.hooksPath")
	if strings.TrimSpace(output) != ".githooks" {
		t.Fatalf("core.hooksPath = %q, want .githooks", output)
	}
}

func TestApplyInitWrapsLegacyGitHookBeforeConfiguringHooksPath(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	writeExecutable(t, root, ".git/hooks/pre-commit", "#!/usr/bin/env bash\necho legacy\n")

	result, err := ApplyInitWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	if got := readFile(t, root, ".githooks/pre-commit.user"); got != "#!/usr/bin/env bash\necho legacy\n" {
		t.Fatalf("legacy hook was not preserved: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".git/hooks/pre-commit")); !os.IsNotExist(err) {
		t.Fatalf("legacy hook should be moved out of .git/hooks, err=%v", err)
	}
	if got := readFile(t, root, ".githooks/pre-commit"); !strings.Contains(got, "\"${BURPVALVE[@]}\" commit") || !strings.Contains(got, ".githooks/pre-commit.user") {
		t.Fatalf("dispatcher hook not installed:\n%s", got)
	}
	if !contains(result.Preserved, ".githooks/pre-commit.user") {
		t.Fatalf("preserved legacy hook not reported: %#v", result.Preserved)
	}
	if strings.TrimSpace(runGit(t, root, "config", "--get", "core.hooksPath")) != ".githooks" {
		t.Fatal("core.hooksPath not configured")
	}
}

func TestApplyInitPreservesExistingUserContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "user contract\n")
	writeFile(t, root, "backpressure/dry.md", "# User DRY\n\nKeep this.\n")
	writeFile(t, root, ".githooks/pre-commit", "#!/usr/bin/env bash\n# existing hook\n")

	result, err := ApplyInitWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}
	agents := readFile(t, root, "AGENTS.md")
	for _, needle := range []string{"user contract", agentsRepairStart, "## Beads", "## Backpressure", "## Definition Of Done"} {
		if !strings.Contains(agents, needle) {
			t.Fatalf("AGENTS.md missing %q after init repair:\n%s", needle, agents)
		}
	}
	if got := readFile(t, root, "backpressure/dry.md"); got != "# User DRY\n\nKeep this.\n" {
		t.Fatalf("backpressure/dry.md overwritten: %q", got)
	}
	if got := readFile(t, root, ".githooks/pre-commit.user"); got != "#!/usr/bin/env bash\n# existing hook\n" {
		t.Fatalf("existing hook not preserved as user hook: %q", got)
	}
	if got := readFile(t, root, ".githooks/pre-commit"); !strings.Contains(got, "\"${BURPVALVE[@]}\" commit") || !strings.Contains(got, ".githooks/pre-commit.user") {
		t.Fatalf("dispatcher hook not installed:\n%s", got)
	}
	if !containsPrefix(result.Repaired, "AGENTS.md sections: Agent Startup, Beads, Atomic Work And Commits") {
		t.Fatalf("expected AGENTS.md section repair in %#v", result.Repaired)
	}
	if text := result.Text(); !strings.Contains(text, "repaired") || !strings.Contains(text, "AGENTS.md sections:") {
		t.Fatalf("init result text should show AGENTS.md repair:\n%s", text)
	}
	for _, rel := range []string{"backpressure/dry.md", ".githooks/pre-commit.user"} {
		if !contains(result.Preserved, rel) {
			t.Fatalf("expected %s to be preserved in %#v", rel, result.Preserved)
		}
	}
}

func TestApplyInitReportsClaudeRegularFileConflict(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "CLAUDE.md", "existing\n")

	result, err := ApplyInitWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("conflicts = %#v", result.Conflicts)
	}
	if result.Conflicts[0].Path != "CLAUDE.md" {
		t.Fatalf("unexpected conflict: %#v", result.Conflicts[0])
	}
	if got := readFile(t, root, "CLAUDE.md"); got != "existing\n" {
		t.Fatalf("CLAUDE.md overwritten despite conflict: %q", got)
	}
}

func TestEmbeddedTemplatesMatchRootTemplates(t *testing.T) {
	root := repoRoot(t)
	var embedded []string
	err := filepath.WalkDir(filepath.Join(root, "internal/scaffold/templates"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.Join(root, "internal/scaffold/templates"), path)
		if err != nil {
			return err
		}
		embedded = append(embedded, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(embedded)
	for _, rel := range embedded {
		got, err := os.ReadFile(filepath.Join(root, "internal/scaffold/templates", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(root, "templates", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("root template missing for embedded %s: %v", rel, err)
		}
		if string(got) != string(want) {
			t.Fatalf("embedded template %s differs from root template", rel)
		}
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func successfulBeadsRunner() *fakeRunner {
	return fakeRunnerPtr(map[string]CommandResult{
		"br init":                         {},
		"br sync --import-only":           {},
		"br doctor --json":                {Stdout: `{"workspace_health":"healthy"}`},
		"br config list":                  {},
		"br dep cycles":                   {},
		"br sync --flush-only":            {},
		"git config --get core.hooksPath": {},
		"ntm --robot-capabilities":        {Stdout: "capabilities ok"},
		"ntm --robot-snapshot":            {Stdout: "snapshot ok\nattention tail"},
	}, map[string]error{})
}

func applyOptions(t *testing.T, runner *fakeRunner, looker fakeLooker) ApplyOptions {
	t.Helper()
	looker["burpvalve"] = true
	return ApplyOptions{
		Runner:               runner,
		Looker:               looker,
		BackpressureToolPath: fakeBackpressureTool(t),
		GitInit:              true,
	}
}

func fakeBackpressureTool(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "burpvalve")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\necho fake backpressure\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}
