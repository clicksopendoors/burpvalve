package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetupWorkflowFixtures(t *testing.T) {
	repo := repoRoot(t)
	tests := []struct {
		name               string
		setup              func(t *testing.T, root string)
		expectedStatuses   map[string]CheckStatus
		expectedReportText []string
		expectedPlans      []string
		verify             func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult)
	}{
		{
			name: "empty-dir",
			expectedStatuses: map[string]CheckStatus{
				"agents": StatusMissing,
				"claude": StatusMissing,
				"docs":   StatusMissing,
				"beads":  StatusMissing,
			},
			expectedReportText: []string{
				"AGENTS.md - shared instructions",
				"CLAUDE.md - points Claude Code",
				".beads - local issue and task graph",
			},
			expectedPlans: []string{"agents", "claude", "beads"},
			verify: func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult) {
				requireGeneratedScaffold(t, root)
				requireSymlinkTarget(t, root, "CLAUDE.md", "AGENTS.md")
				requireNoChangesAfterRepair(t, root, defaultApplyOptions(t))
			},
		},
		{
			name: "git-empty",
			setup: func(t *testing.T, root string) {
				runGit(t, root, "init", "-q")
			},
			expectedStatuses: map[string]CheckStatus{
				"git-repo":       StatusPresent,
				"git-hooks-path": StatusUnavailable,
				"githook":        StatusMissing,
			},
			expectedReportText: []string{
				".git - Git metadata",
				"git core.hooksPath - Git setting",
				".githooks/pre-commit - script Git runs",
			},
			expectedPlans: []string{"git-hooks-path", "githook"},
			verify: func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult) {
				requireGeneratedScaffold(t, root)
				if !contains(initResult.Created, "git config core.hooksPath") {
					t.Fatalf("git fixture should configure hooksPath: %#v", initResult.Created)
				}
			},
		},
		{
			name: "existing-agents",
			expectedStatuses: map[string]CheckStatus{
				"agents": StatusPresent,
				"claude": StatusMissing,
			},
			expectedReportText: []string{
				"AGENTS.md - shared instructions",
				"CLAUDE.md - points Claude Code",
			},
			expectedPlans: []string{"claude", "beads"},
			verify: func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult) {
				body := readFile(t, root, "AGENTS.md")
				for _, needle := range []string{"# Existing Agent Contract", "Project-specific notes", agentsRepairStart, "## Beads", "## NTM Session Naming", "## Backpressure"} {
					if !strings.Contains(body, needle) {
						t.Fatalf("existing AGENTS fixture missing %q after repair:\n%s", needle, body)
					}
				}
			},
		},
		{
			name: "regular-claude",
			expectedStatuses: map[string]CheckStatus{
				"agents": StatusMissing,
				"claude": StatusConflict,
			},
			expectedReportText: []string{
				"conflict     CLAUDE.md",
				"manual review before repair",
			},
			expectedPlans: []string{"claude"},
			verify: func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult) {
				if target, err := os.Readlink(filepath.Join(root, "CLAUDE.md")); err != nil || target != "AGENTS.md" {
					t.Fatalf("regular CLAUDE.md should be migrated to AGENTS.md symlink, target=%q err=%v", target, err)
				}
				if got := readFile(t, root, "AGENTS.md"); !strings.Contains(got, "Human Claude Notes") {
					t.Fatalf("regular CLAUDE.md content was not migrated into AGENTS.md: %q", got)
				}
				if len(initResult.Conflicts) == 0 {
					t.Fatalf("regular CLAUDE.md should still conflict during init: init=%#v", initResult.Conflicts)
				}
				if len(repairResult.Conflicts) != 0 {
					t.Fatalf("regular CLAUDE.md should not conflict during repair: repair=%#v", repairResult.Conflicts)
				}
			},
		},
		{
			name: "existing-beads",
			expectedStatuses: map[string]CheckStatus{
				"beads":  StatusPresent,
				"agents": StatusMissing,
			},
			expectedReportText: []string{
				".beads - local issue and task graph",
				"AGENTS.md - shared instructions",
			},
			expectedPlans: []string{"agents", "claude"},
			verify: func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult) {
				if contains(initResult.Commands, "br init") {
					t.Fatalf("init reinitialized existing beads: %#v", initResult.Commands)
				}
				if contains(repairResult.Commands, "br init") {
					t.Fatalf("repair reinitialized existing beads: %#v", repairResult.Commands)
				}
			},
		},
		{
			name: "partial-backpressure",
			expectedStatuses: map[string]CheckStatus{
				"backpressure":              StatusPresent,
				"backpressure-attestations": StatusPresent,
				"agents":                    StatusMissing,
			},
			expectedReportText: []string{
				"backpressure - rules a change",
				"backpressure/attestations - tracked proof",
			},
			expectedPlans: []string{"agents", "claude", "beads"},
			verify: func(t *testing.T, root string, initResult ApplyResult, repairResult RepairResult) {
				for rel, needle := range map[string]string{
					"backpressure/README.md":                  "Keep this project-specific policy text.",
					"backpressure/dry.md":                     "Keep this project-specific DRY rule.",
					"backpressure/attestations/existing.json": `"fixture":true`,
				} {
					if got := readFile(t, root, rel); !strings.Contains(got, needle) {
						t.Fatalf("%s did not preserve fixture content %q:\n%s", rel, needle, got)
					}
				}
				if _, err := os.Stat(filepath.Join(root, "backpressure", "autonomy-boundary.md")); err != nil {
					t.Fatalf("repair/init did not fill missing standard backpressure file: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyFixture(t, repo, tt.name)
			if tt.setup != nil {
				tt.setup(t, root)
			}
			beforeCheck := snapshotFiles(t, root)
			report, err := Inspect(root, InspectOptions{
				Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
				Looker: fakeLooker{},
				Now:    fixedNow,
			})
			if err != nil {
				t.Fatalf("Inspect returned error: %v", err)
			}
			afterCheck := snapshotFiles(t, root)
			if !reflect.DeepEqual(beforeCheck, afterCheck) {
				t.Fatalf("check mode mutated fixture\nbefore=%v\nafter=%v", beforeCheck, afterCheck)
			}
			if report.Mutating {
				t.Fatal("check report claimed mutation")
			}
			for id, status := range tt.expectedStatuses {
				assertCheck(t, report, id, status)
			}
			for _, id := range tt.expectedPlans {
				if !hasAnyPlannedChange(report, id) {
					t.Fatalf("expected planned change for %s in %#v", id, report.PlannedChanges)
				}
			}
			reportText := report.Text()
			for _, needle := range []string{"burpvalve setup report", "summary", root, "edits files", "ready"} {
				if !strings.Contains(reportText, needle) {
					t.Fatalf("readiness report text missing %q:\n%s", needle, reportText)
				}
			}
			for _, needle := range tt.expectedReportText {
				if !strings.Contains(reportText, needle) {
					t.Fatalf("%s readiness report text missing %q:\n%s", tt.name, needle, reportText)
				}
			}

			initResult, initErr := ApplyInitWithOptions(root, defaultApplyOptions(t))
			if tt.name == "regular-claude" {
				if initErr == nil {
					t.Fatal("regular CLAUDE fixture should make init report a conflict")
				}
			} else if initErr != nil {
				t.Fatalf("ApplyInit returned error: %v", initErr)
			}
			beforeInitRerun := snapshotFiles(t, root)
			initRerun, rerunErr := ApplyInitWithOptions(root, defaultApplyOptions(t))
			if tt.name == "regular-claude" {
				if rerunErr == nil {
					t.Fatal("regular CLAUDE fixture should still conflict on init rerun")
				}
			} else if rerunErr != nil {
				t.Fatalf("ApplyInit rerun returned error: %v", rerunErr)
			}
			afterInitRerun := snapshotFiles(t, root)
			if !reflect.DeepEqual(beforeInitRerun, afterInitRerun) {
				t.Fatalf("init rerun changed fixture tree\nbefore=%v\nafter=%v", beforeInitRerun, afterInitRerun)
			}
			if tt.name != "regular-claude" && len(initRerun.Created) != 0 {
				t.Fatalf("init rerun should create nothing: %#v", initRerun.Created)
			}

			repairOpts := defaultApplyOptions(t)
			if tt.name == "regular-claude" {
				repairOpts.AdoptClaude = true
			}
			repairResult, repairErr := ApplyRepairWithOptions(root, repairOpts)
			if repairErr != nil {
				t.Fatalf("ApplyRepair returned error: %v", repairErr)
			}
			beforeRepairRerun := snapshotFiles(t, root)
			repairRerun, repairRerunErr := ApplyRepairWithOptions(root, repairOpts)
			if repairRerunErr != nil {
				t.Fatalf("ApplyRepair rerun returned error: %v", repairRerunErr)
			}
			afterRepairRerun := snapshotFiles(t, root)
			if !reflect.DeepEqual(beforeRepairRerun, afterRepairRerun) {
				t.Fatalf("repair rerun changed fixture tree\nbefore=%v\nafter=%v", beforeRepairRerun, afterRepairRerun)
			}
			if len(repairRerun.Repaired) != 0 {
				t.Fatalf("repair rerun should repair nothing: %#v", repairRerun.Repaired)
			}
			tt.verify(t, root, initResult, repairResult)
		})
	}
}

func TestGeneratedClaudeOrchestratorSkillPackageAcceptance(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyInitWithOptions(root, ApplyOptions{
		Targets:     []ScaffoldTarget{TargetClaude},
		ClaudeRoute: ClaudeRouteOrchestratorSkill,
	})
	if err != nil {
		t.Fatalf("ApplyInit returned error: %v", err)
	}

	base := filepath.Join(root, ".claude", "skills", "burpvalve-orchestrator")
	required := []string{
		"SKILL.md",
		"SELF-TEST.md",
		"references/burpvalve-gate-choreography.md",
		"references/verifier-fanout-and-attestations.md",
		"references/agent-mail-and-file-coordination.md",
		"references/ntm-pane-wake-discipline.md",
		"references/beads-and-gate-window-operations.md",
		"references/orchestrator-toolbox.md",
		"examples/gated-implementation-handoff.md",
		"examples/verifier-disagreement-hold.md",
		"examples/gate-window-release.md",
		"scripts/pane_wake.py",
		"scripts/poll_worker.py",
		"scripts/poll_round.py",
		"scripts/attestation_summary.py",
		"scripts/append_finding.py",
	}
	for _, rel := range required {
		if info, err := os.Stat(filepath.Join(base, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("generated skill package missing %s: %v", rel, err)
		} else if info.IsDir() {
			t.Fatalf("generated skill package path %s is a directory", rel)
		}
	}

	skill := readFile(t, root, ".claude/skills/burpvalve-orchestrator/SKILL.md")
	for _, want := range []string{
		"name: burpvalve-orchestrator",
		"category: other",
		"license: MIT",
		"distribution: public",
		"# burpvalve-orchestrator",
		"Core rule: coordinate evidence flow; do not manufacture evidence",
		"## Quick Start",
		"## Decision Tables",
		"## Reference Map",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("generated SKILL.md missing %q:\n%s", want, skill)
		}
	}
	for _, link := range []string{
		"(references/burpvalve-gate-choreography.md)",
		"(references/verifier-fanout-and-attestations.md)",
		"(references/agent-mail-and-file-coordination.md)",
		"(references/ntm-pane-wake-discipline.md)",
		"(references/beads-and-gate-window-operations.md)",
		"(references/orchestrator-toolbox.md)",
		"(examples/gated-implementation-handoff.md)",
		"(examples/verifier-disagreement-hold.md)",
		"(examples/gate-window-release.md)",
		"(scripts/pane_wake.py)",
		"(scripts/poll_worker.py)",
		"(scripts/poll_round.py)",
		"(scripts/attestation_summary.py)",
		"(scripts/append_finding.py)",
		"(SELF-TEST.md)",
	} {
		if strings.Count(skill, link) != 1 {
			t.Fatalf("generated SKILL.md link count for %s = %d, want 1", link, strings.Count(skill, link))
		}
	}

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
			return os.ErrInvalid
		}
		return nil
	}); err != nil {
		t.Fatalf("generated skill package should not contain binary files: %v", err)
	}

	fakeBin := filepath.Join(t.TempDir(), "bin")
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
	calls := filepath.Join(t.TempDir(), "ntm-calls.txt")
	env := append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "NTM_CALLS="+calls)
	for _, rel := range []string{"pane_wake.py", "poll_worker.py", "poll_round.py", "attestation_summary.py", "append_finding.py"} {
		scriptPath := filepath.Join(base, "scripts", rel)
		if _, err := os.Stat(scriptPath); err != nil {
			t.Fatalf("stat generated script %s: %v", rel, err)
		}
		cmd := exec.Command("python3", scriptPath, "--help")
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s --help failed: %v\n%s", rel, err, output)
		}
	}
	cmd := exec.Command("python3", filepath.Join(base, "scripts", "pane_wake.py"), "--pane", "8", "--message", "wake")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pane_wake dry-run failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "DRY-RUN") {
		t.Fatalf("pane_wake dry-run output missing DRY-RUN:\n%s", output)
	}
	callLog, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--robot-capabilities", "--robot-snapshot"} {
		if !strings.Contains(string(callLog), want) {
			t.Fatalf("pane_wake did not preflight %s; calls:\n%s", want, callLog)
		}
	}
	if strings.Contains(string(callLog), "--robot-send=burpvalve") {
		t.Fatalf("pane_wake sent wake without --execute:\n%s", callLog)
	}

	if jsm, err := exec.LookPath("jsm"); err == nil {
		cmd := exec.Command(jsm, "validate", base)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jsm validate failed for generated skill package: %v\n%s", err, output)
		}
	} else {
		t.Logf("jsm unavailable; direct generated package contract checks covered frontmatter, resources, scripts, and no-binary rules")
	}
}

func copyFixture(t *testing.T, repoRoot, name string) string {
	t.Helper()
	src := filepath.Join(repoRoot, "fixtures", name)
	dst := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func defaultApplyOptions(t *testing.T) ApplyOptions {
	t.Helper()
	return applyOptions(t, successfulBeadsRunner(), fakeLooker{"br": true})
}

func requireGeneratedScaffold(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		".gitignore",
		"docs/README.md",
		"plans/README.md",
		"log/README.md",
		"backpressure/README.md",
		"backpressure/attestations/README.md",
		".githooks/pre-commit",
		"tools/burpvalve/README.md",
		".beads",
	} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing generated scaffold path %s: %v", rel, err)
		}
	}
}

func requireSymlinkTarget(t *testing.T, root, rel, want string) {
	t.Helper()
	got, err := os.Readlink(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("%s should be symlink: %v", rel, err)
	}
	if got != want {
		t.Fatalf("%s symlink = %q, want %q", rel, got, want)
	}
}

func requireNoChangesAfterRepair(t *testing.T, root string, opts ApplyOptions) {
	t.Helper()
	before := snapshotFiles(t, root)
	result, err := ApplyRepairWithOptions(root, opts)
	if err != nil {
		t.Fatalf("repair idempotency check returned error: %v", err)
	}
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("repair idempotency check changed tree\nbefore=%v\nafter=%v", before, after)
	}
	if len(result.Created) != 0 || len(result.Repaired) != 0 {
		t.Fatalf("repair idempotency check should create/repair nothing: %#v", result)
	}
}

func hasAnyPlannedChange(report Report, id string) bool {
	for _, change := range report.PlannedChanges {
		if change.ID == id {
			return true
		}
	}
	return false
}
