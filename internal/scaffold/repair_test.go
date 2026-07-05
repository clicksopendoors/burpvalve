package scaffold

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplyRepairCreatesMissingGeneratedPiecesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	runner := successfulBeadsRunner()
	opts := applyOptions(t, runner, fakeLooker{"br": true})
	result, err := ApplyRepairWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	for _, rel := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		".gitignore",
		"docs/README.md",
		"docs/ntm-bridge.md",
		"plans/README.md",
		"log/README.md",
		"backpressure/README.md",
		"backpressure/attestations/README.md",
		"backpressure/dry.md",
		"backpressure/autonomy-boundary.md",
		".githooks/pre-commit",
		"tools/burpvalve/README.md",
		".beads",
	} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("repair did not create %s: %v", rel, err)
		}
	}
	if !result.Mutating || !result.Implemented {
		t.Fatalf("repair result should be mutating and implemented: %#v", result)
	}
	if len(result.Created) == 0 {
		t.Fatalf("expected created repairs: %#v", result)
	}
	if !contains(result.Created, "CLAUDE.md") {
		t.Fatalf("expected CLAUDE.md creation: %#v", result.Created)
	}
	if _, err := os.Stat(filepath.Join(root, "bin/burpvalve")); !os.IsNotExist(err) {
		t.Fatalf("default repair should not install repo-local bin/burpvalve, err=%v", err)
	}
	before := snapshotFiles(t, root)
	rerun, err := ApplyRepairWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyRepair rerun returned error: %v", err)
	}
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("repair rerun changed file tree\nbefore=%v\nafter=%v", before, after)
	}
	if len(rerun.Repaired) != 0 || len(rerun.Conflicts) != 0 {
		t.Fatalf("rerun should not repair or conflict: %#v", rerun)
	}
}

func TestApplyRepairPreservesRepoLocalBinaryWarningFacts(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	writeFile(t, root, "cmd/burpvalve/main.go", "package main\n")
	old := fixedNow().Add(-2 * time.Hour)
	newer := fixedNow().Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "bin/burpvalve"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "cmd/burpvalve/main.go"), newer, newer); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":                       {Stdout: ""},
		"git status --short --untracked-files=all": {Stdout: ""},
		"git config --get core.hooksPath":          {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve":     {ExitCode: 1},
		"git ls-files -z -- cmd internal go.mod go.sum internal/scaffold/templates templates scripts install.sh": {Stdout: "cmd/burpvalve/main.go\x00"},
	}, map[string]error{
		"git check-ignore -v -- bin/burpvalve": os.ErrNotExist,
	})

	result, err := ApplyRepairWithOptions(root, ApplyOptions{
		Runner:  runner,
		Looker:  fakeLooker{"br": true, "git": true},
		Targets: []ScaffoldTarget{TargetTool},
	})
	if err != nil {
		t.Fatalf("ApplyRepairWithOptions returned error: %v", err)
	}
	if result.RepoLocalBinary == nil || result.RepoLocalBinary.FreshnessStatus != "stale" || result.RepoLocalBinary.WarningCode != "repo_local_stale" {
		t.Fatalf("repair should preserve repo-local warning facts: %#v", result.RepoLocalBinary)
	}
	text := result.Text()
	if !strings.Contains(text, "repo-local binary provenance") || !strings.Contains(text, "repo_local_stale") {
		t.Fatalf("repair text should include repo-local facts:\n%s", text)
	}
}

func TestApplyRepairAppendsMissingAgentsSectionsWithMarkers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Custom Contract\n\nKeep my notes.\n")

	result, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	body := readFile(t, root, "AGENTS.md")
	for _, needle := range []string{
		"# Custom Contract",
		"Keep my notes.",
		agentsRepairStart,
		"## Beads",
		"## NTM Session Naming",
		"## Backpressure",
		"## Docs, Plans, And Logs",
		agentsRepairEnd,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("AGENTS.md missing %q after repair:\n%s", needle, body)
		}
	}
	if !containsPrefix(result.Repaired, "AGENTS.md sections:") {
		t.Fatalf("expected AGENTS section repair: %#v", result.Repaired)
	}

	second, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("second ApplyRepair returned error: %v", err)
	}
	if strings.Count(readFile(t, root, "AGENTS.md"), agentsRepairStart) != 1 {
		t.Fatalf("repair block duplicated:\n%s", readFile(t, root, "AGENTS.md"))
	}
	if len(second.Repaired) != 0 {
		t.Fatalf("second repair should not append sections: %#v", second.Repaired)
	}
}

func TestApplyRepairPreservesUserBackpressureAttestationsAndHooks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "backpressure/README.md", "# User Backpressure\n\nCustom policy.\n")
	writeFile(t, root, "backpressure/dry.md", "# User DRY\n\nDo not rewrite.\n")
	writeFile(t, root, "backpressure/attestations/existing.json", `{"ok":true}`+"\n")
	writeFile(t, root, ".githooks/pre-commit", "#!/usr/bin/env bash\n# user hook\n")

	result, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	for rel, want := range map[string]string{
		"backpressure/README.md":                  "# User Backpressure\n\nCustom policy.\n",
		"backpressure/dry.md":                     "# User DRY\n\nDo not rewrite.\n",
		"backpressure/attestations/existing.json": `{"ok":true}` + "\n",
		".githooks/pre-commit.user":               "#!/usr/bin/env bash\n# user hook\n",
	} {
		if got := readFile(t, root, rel); got != want {
			t.Fatalf("%s overwritten:\ngot  %q\nwant %q", rel, got, want)
		}
	}
	if got := readFile(t, root, ".githooks/pre-commit"); !strings.Contains(got, "\"${BURPVALVE[@]}\" commit") || !strings.Contains(got, ".githooks/pre-commit.user") {
		t.Fatalf("dispatcher hook not installed:\n%s", got)
	}
	for _, rel := range []string{"backpressure/README.md", "backpressure/dry.md", ".githooks/pre-commit.user"} {
		if !contains(result.Preserved, rel) {
			t.Fatalf("expected %s preserved in %#v", rel, result.Preserved)
		}
	}
}

func TestApplyRepairReportsClaudeRegularFileConflictWithoutAdoption(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "CLAUDE.md", "# Human Claude Notes\n\nKeep this context.\n")

	result, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err == nil {
		t.Fatal("expected unmarked CLAUDE.md to conflict without explicit adoption")
	}
	if len(result.Conflicts) == 0 || result.Conflicts[0].Path != "CLAUDE.md" {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
	if got := readFile(t, root, "CLAUDE.md"); !strings.Contains(got, "Human Claude Notes") {
		t.Fatalf("CLAUDE.md should be preserved on conflict: %q", got)
	}
}

func TestApplyRepairMigratesClaudeRegularFileToAgentsSymlinkWithAdoption(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "CLAUDE.md", "# Human Claude Notes\n\nKeep this context.\n")
	opts := applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true})
	opts.AdoptClaude = true

	result, err := ApplyRepairWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	target, err := os.Readlink(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be a symlink: %v", err)
	}
	if target != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink = %q, want AGENTS.md", target)
	}
	agents := readFile(t, root, "AGENTS.md")
	for _, needle := range []string{
		claudeImportStart,
		"## Imported CLAUDE.md Notes",
		"# Human Claude Notes",
		"Keep this context.",
		claudeImportEnd,
	} {
		if !strings.Contains(agents, needle) {
			t.Fatalf("AGENTS.md missing migrated CLAUDE content %q:\n%s", needle, agents)
		}
	}
	for _, want := range []string{"AGENTS.md imported CLAUDE.md content", "CLAUDE.md symlink"} {
		if !contains(result.Repaired, want) {
			t.Fatalf("repair result missing %q: %#v", want, result.Repaired)
		}
	}
}

func TestApplyRepairClaudeCanSkipAgentsDependency(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "CLAUDE.md", "human-owned\n")

	result, err := ApplyRepairWithOptions(root, ApplyOptions{
		Runner:     successfulBeadsRunner(),
		Looker:     fakeLooker{"br": true},
		Targets:    []ScaffoldTarget{TargetClaude},
		SkipAgents: true,
	})
	if err == nil {
		t.Fatal("expected conflict when CLAUDE.md repair cannot create AGENTS.md")
	}
	if len(result.Conflicts) == 0 || result.Conflicts[0].Path != "AGENTS.md" {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
	if got := readFile(t, root, "CLAUDE.md"); got != "human-owned\n" {
		t.Fatalf("CLAUDE.md should be preserved on conflict: %q", got)
	}
}

func TestApplyRepairCanRepairOnlyAgents(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Custom Contract\n")

	result, err := ApplyRepairWithOptions(root, ApplyOptions{
		Runner:  successfulBeadsRunner(),
		Looker:  fakeLooker{"br": true},
		Targets: []ScaffoldTarget{TargetAgents},
	})
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	if !containsPrefix(result.Repaired, "AGENTS.md sections: Agent Startup, Atomic Work And Commits") {
		t.Fatalf("scoped AGENTS.md repair should append core sections: %#v", result.Repaired)
	}
	body := readFile(t, root, "AGENTS.md")
	for _, want := range []string{"# Custom Contract", "## Agent Startup", "## Atomic Work And Commits", "## Definition Of Done", "## Uncertainty", "## File Coordination"} {
		if !strings.Contains(body, want) {
			t.Fatalf("scoped AGENTS.md repair missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"## Beads", "## NTM Session Naming", "## Backpressure", "## Docs, Plans, And Logs"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("scoped AGENTS.md repair should not append %q:\n%s", forbidden, body)
		}
	}
	for _, rel := range []string{"CLAUDE.md", "log/README.md", "backpressure/README.md", ".githooks/pre-commit"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("scoped AGENTS.md repair should not create %s, err=%v", rel, err)
		}
	}
}

func TestApplyRepairCreatesOrchestratorOnlyWhenTargeted(t *testing.T) {
	defaultRoot := t.TempDir()
	_, err := ApplyRepairWithOptions(defaultRoot, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("default ApplyRepair returned error: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(defaultRoot, "ORCHESTRATOR.md")); !os.IsNotExist(statErr) {
		t.Fatalf("default repair should not create ORCHESTRATOR.md, err=%v", statErr)
	}

	targetedRoot := t.TempDir()
	result, err := ApplyRepairWithOptions(targetedRoot, ApplyOptions{
		Runner:  successfulBeadsRunner(),
		Looker:  fakeLooker{"br": true},
		Targets: []ScaffoldTarget{TargetOrchestrator},
	})
	if err != nil {
		t.Fatalf("targeted ApplyRepair returned error: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(targetedRoot, "ORCHESTRATOR.md")); statErr != nil {
		t.Fatalf("targeted repair should create ORCHESTRATOR.md: %v", statErr)
	}
	if !contains(result.Created, "ORCHESTRATOR.md") {
		t.Fatalf("targeted repair result should report ORCHESTRATOR.md creation: %#v", result.Created)
	}
}

func TestApplyRepairConvertsClaudeSymlinkToOrchestratorSkillWhenExplicit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Agent Operating Contract\n")
	if err := os.Symlink("AGENTS.md", filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	result, err := ApplyRepairWithOptions(root, ApplyOptions{
		Runner:      successfulBeadsRunner(),
		Looker:      fakeLooker{"br": true},
		Targets:     []ScaffoldTarget{TargetClaude},
		ClaudeRoute: ClaudeRouteOrchestratorSkill,
	})
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	info, err := os.Lstat(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("CLAUDE.md should be converted to generated regular orchestrator bootstrap")
	}
	if !strings.Contains(readFile(t, root, "CLAUDE.md"), "Claude Orchestrator Route") {
		t.Fatalf("CLAUDE.md bootstrap not written:\n%s", readFile(t, root, "CLAUDE.md"))
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude/skills/burpvalve-orchestrator/SKILL.md")); err != nil {
		t.Fatalf("orchestrator skill package not repaired: %v", err)
	}
	if !contains(result.Repaired, "CLAUDE.md orchestrator bootstrap") {
		t.Fatalf("conversion not reported: %#v", result.Repaired)
	}
}

func TestApplyRepairConvertsGeneratedOrchestratorBootstrapToSymlinkWhenExplicit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Agent Operating Contract\n")
	body, err := embeddedTemplates.ReadFile("templates/CLAUDE.md.orchestrator.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "CLAUDE.md", string(body))

	result, err := ApplyRepairWithOptions(root, ApplyOptions{
		Runner:      successfulBeadsRunner(),
		Looker:      fakeLooker{"br": true},
		Targets:     []ScaffoldTarget{TargetClaude},
		ClaudeRoute: ClaudeRouteAgentSymlink,
	})
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	target, err := os.Readlink(filepath.Join(root, "CLAUDE.md"))
	if err != nil || target != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink = %q err=%v, want AGENTS.md", target, err)
	}
	if !contains(result.Repaired, "CLAUDE.md symlink") {
		t.Fatalf("symlink conversion not reported: %#v", result.Repaired)
	}
}

func TestApplyRepairRepairsGeneratedOrchestratorSkillDrift(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyInitWithOptions(root, ApplyOptions{
		Targets:     []ScaffoldTarget{TargetClaude},
		ClaudeRoute: ClaudeRouteOrchestratorSkill,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".claude/skills/burpvalve-orchestrator/SKILL.md", "# drift\n")

	result, err := ApplyRepairWithOptions(root, ApplyOptions{
		Runner:      successfulBeadsRunner(),
		Looker:      fakeLooker{"br": true},
		Targets:     []ScaffoldTarget{TargetClaude},
		ClaudeRoute: ClaudeRepairRoutePreserve,
	})
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	if got := readFile(t, root, ".claude/skills/burpvalve-orchestrator/SKILL.md"); !strings.Contains(got, "# burpvalve-orchestrator") {
		t.Fatalf("skill drift not repaired:\n%s", got)
	}
	if !contains(result.Repaired, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
		t.Fatalf("skill drift repair not reported: %#v", result.Repaired)
	}
}

func TestApplyRepairOmitsSkippedAgentsSections(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Custom Contract\n")
	opts := applyOptions(t, successfulBeadsRunner(), fakeLooker{})
	opts.SkipBeads = true
	opts.SkipNTM = true
	opts.SkipDocs = true
	opts.SkipPlans = true
	opts.SkipLog = true

	result, err := ApplyRepairWithOptions(root, opts)
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	body := readFile(t, root, "AGENTS.md")
	for _, want := range []string{agentsRepairStart, "## Agent Startup", "## Atomic Work And Commits", "## Backpressure", "## Definition Of Done", "## Uncertainty", "## File Coordination", agentsRepairEnd} {
		if !strings.Contains(body, want) {
			t.Fatalf("skip-aware AGENTS.md repair missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"## Beads", "## NTM Session Naming", "## Docs, Plans, And Logs", "`/docs/`", "`/plans/`", "`/log/`"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("skip-aware AGENTS.md repair should omit %q:\n%s", forbidden, body)
		}
	}
	if !containsPrefix(result.Repaired, "AGENTS.md sections: Agent Startup, Atomic Work And Commits, Backpressure") {
		t.Fatalf("expected skip-aware AGENTS section repair: %#v", result.Repaired)
	}
}

func TestApplyRepairVerifiesExistingBeadsWithoutReinitializing(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := successfulBeadsRunner()
	result, err := ApplyRepairWithOptions(root, applyOptions(t, runner, fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	want := []string{"br doctor --json", "br config list", "br dep cycles", "br sync --flush-only"}
	if !reflect.DeepEqual(result.Commands, want) {
		t.Fatalf("commands = %#v, want %#v", result.Commands, want)
	}
	if contains(result.Commands, "br init") {
		t.Fatal("repair should not reinitialize existing beads")
	}
}

func TestApplyRepairConfiguresGitHooksPath(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	result, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	if strings.TrimSpace(runGit(t, root, "config", "--get", "core.hooksPath")) != ".githooks" {
		t.Fatal("repair did not configure core.hooksPath")
	}
	if !contains(result.Created, "git config core.hooksPath") {
		t.Fatalf("repair result did not record hooksPath config: %#v", result.Created)
	}
}

func TestApplyRepairMigratesLegacyHookWhenDispatcherAlreadyExists(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	dispatcher, err := embeddedTemplates.ReadFile("templates/githooks/pre-commit")
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, root, ".githooks/pre-commit", string(dispatcher))
	writeExecutable(t, root, ".git/hooks/pre-commit", "#!/usr/bin/env bash\necho legacy\n")

	result, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	if got := readFile(t, root, ".githooks/pre-commit.user"); got != "#!/usr/bin/env bash\necho legacy\n" {
		t.Fatalf("legacy hook was not preserved: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".git/hooks/pre-commit")); !os.IsNotExist(err) {
		t.Fatalf("legacy hook should be moved out of .git/hooks, err=%v", err)
	}
	if strings.TrimSpace(runGit(t, root, "config", "--get", "core.hooksPath")) != ".githooks" {
		t.Fatal("repair did not configure core.hooksPath")
	}
	if !contains(result.Preserved, ".githooks/pre-commit.user") {
		t.Fatalf("repair did not report preserved legacy hook: %#v", result.Preserved)
	}
}

func TestApplyRepairMakesStaleGeneratedHookExecutable(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	writeFile(t, root, ".githooks/pre-commit", "#!/usr/bin/env bash\nBURPVALVE=(./bin/burpvalve)\n\"${BURPVALVE[@]}\" commit\n\"${BURPVALVE[@]}\" lint\n")

	result, err := ApplyRepairWithOptions(root, applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true}))
	if err != nil {
		t.Fatalf("ApplyRepair returned error: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, ".githooks/pre-commit"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("repair left generated hook non-executable: %v", info.Mode().Perm())
	}
	if !contains(result.Repaired, ".githooks/pre-commit dispatcher") {
		t.Fatalf("repair did not report dispatcher update: %#v", result.Repaired)
	}
}
