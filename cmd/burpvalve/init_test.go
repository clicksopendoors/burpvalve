package main

import (
	"burpvalve/internal/charmui"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSupportsPartialSetupOptOutFlags(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm", "--no-agents-md", "--no-claude-symlink")
	if err != nil {
		t.Fatalf("partial init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	var result struct {
		ClaudeRoute string   `json:"claude_route"`
		Skipped     []string `json:"skipped"`
		Commands    []string `json:"commands"`
		NTM         struct {
			Status string `json:"status"`
		} `json:"ntm"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode init result: %v\n%s", err, stdout)
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", ".beads"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s should not be created, err=%v", rel, err)
		}
	}
	for _, rel := range []string{".githooks/pre-commit", "backpressure/manifest.yaml"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s should still be created: %v", rel, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(target, "bin/burpvalve")); !os.IsNotExist(err) {
		t.Fatalf("default init should not create repo-local bin/burpvalve, err=%v", err)
	}
	for _, want := range []string{"AGENTS.md (--no-agents)", "CLAUDE.md (--no-claude)", ".beads (--no-beads)", "ntm quick (--no-ntm)"} {
		if !containsString(result.Skipped, want) {
			t.Fatalf("skipped output missing %q: %#v", want, result.Skipped)
		}
	}
	if result.NTM.Status != "skipped" {
		t.Fatalf("ntm status = %q, want skipped", result.NTM.Status)
	}
	if result.ClaudeRoute != "none" {
		t.Fatalf("--no-claude should map route to none, got %q", result.ClaudeRoute)
	}
	for _, command := range result.Commands {
		if strings.HasPrefix(command, "br ") || strings.HasPrefix(command, "ntm ") {
			t.Fatalf("skip flags should not run br or ntm commands: %#v", result.Commands)
		}
	}
}

func TestInitJSONSkipsBeadsWhenBRMissing(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	run(t, target, "git", "init", "-q")
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "burpvalve"), "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo test-burpvalve; fi\nexit 0\n")

	cmd := exec.Command(goPath, "run", "./cmd/burpvalve", "init", "--target", target, "--force", "--json", "--no-ntm")
	cmd.Dir = repoRoot
	cmd.Env = testEnvWithPath(binDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("init without br should skip beads, not fail: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("json init should not write stderr on br skip:\n%s", stderr.String())
	}
	var result struct {
		Status    string   `json:"status"`
		Fatal     bool     `json:"fatal"`
		Skipped   []string `json:"skipped"`
		Conflicts []struct {
			Path string `json:"path"`
		} `json:"conflicts"`
		Commands []string `json:"commands"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode init result: %v\n%s", err, stdout.String())
	}
	if result.Status != "applied" || result.Fatal || len(result.Conflicts) != 0 {
		t.Fatalf("missing br should be a nonfatal skip: %#v", result)
	}
	if _, err := os.Lstat(filepath.Join(target, ".beads")); !os.IsNotExist(err) {
		t.Fatalf("missing br should not create .beads, err=%v", err)
	}
	if !containsString(result.Skipped, ".beads (br executable not found on PATH)") {
		t.Fatalf("json skipped output should report br absence: %#v", result.Skipped)
	}
	for _, command := range result.Commands {
		if strings.HasPrefix(command, "br ") {
			t.Fatalf("missing br path should not run br commands: %#v", result.Commands)
		}
	}
}

func testEnvWithPath(path string) []string {
	env := os.Environ()
	for i, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			env[i] = "PATH=" + path
			return env
		}
	}
	return append(env, "PATH="+path)
}

func TestInitSupportsSelectedTargets(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "log", "attestations")
	if err != nil {
		t.Fatalf("scoped init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, rel := range []string{"log/README.md", "log/backpressure/failed/README.md", "backpressure/attestations/README.md"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("selected init target missing %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "backpressure/manifest.yaml", ".githooks/pre-commit", "bin/burpvalve", ".beads"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("scoped init should not create %s, err=%v", rel, err)
		}
	}
}

func TestInitSupportsExplicitOrchestratorTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "orchestrator")
	if err != nil {
		t.Fatalf("orchestrator init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(target, "ORCHESTRATOR.md")); err != nil {
		t.Fatalf("explicit orchestrator target should create ORCHESTRATOR.md: %v", err)
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "backpressure/manifest.yaml", ".githooks/pre-commit", "bin/burpvalve", ".beads"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("scoped orchestrator init should not create %s, err=%v", rel, err)
		}
	}
}

func TestInitDogfoodFlagAddsOrchestratorFindingsBlock(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "orchestrator", "--dogfood")
	if err != nil {
		t.Fatalf("dogfood orchestrator init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	body := string(mustReadCLIFile(t, target, "ORCHESTRATOR.md"))
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

func TestInitConfigOrchestratorOptInCreatesTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"init":{"orchestrator":"orchestrator-md","dogfood":true}}}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("config orchestrator init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(target, "ORCHESTRATOR.md")); err != nil {
		t.Fatalf("config defaults.init.orchestrator should create ORCHESTRATOR.md: %v", err)
	}
	agents := string(mustReadCLIFile(t, target, "AGENTS.md"))
	if !strings.Contains(agents, "Orchestrators should read `ORCHESTRATOR.md` first") {
		t.Fatalf("AGENTS.md should include orchestrator pointer when config target is active:\n%s", agents)
	}
	orchestrator := string(mustReadCLIFile(t, target, "ORCHESTRATOR.md"))
	if !strings.Contains(orchestrator, "## Dogfood Findings") {
		t.Fatalf("defaults.init.dogfood should add dogfood block:\n%s", orchestrator)
	}
}

func TestInitNoDogfoodOverridesConfigDefault(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"init":{"orchestrator":"orchestrator-md","dogfood":true}}}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm", "--no-dogfood")
	if err != nil {
		t.Fatalf("config no-dogfood init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	orchestrator := string(mustReadCLIFile(t, target, "ORCHESTRATOR.md"))
	if strings.Contains(orchestrator, "## Dogfood Findings") {
		t.Fatalf("--no-dogfood should override config default:\n%s", orchestrator)
	}
}

func TestInitConfigOrchestratorOffDoesNotCreateTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"init":{"orchestrator":"off"}}}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("config orchestrator off init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(target, "ORCHESTRATOR.md")); !os.IsNotExist(err) {
		t.Fatalf("defaults.init.orchestrator=off should not create ORCHESTRATOR.md, err=%v", err)
	}
}

func TestRepairConfigOrchestratorOptInCreatesTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"repair":{"orchestrator":true}}}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "--git-init", "--no-beads")
	if err != nil {
		t.Fatalf("config orchestrator repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(target, "ORCHESTRATOR.md")); err != nil {
		t.Fatalf("defaults.repair.orchestrator=true should create ORCHESTRATOR.md: %v", err)
	}
}

func TestSetupConfigOrchestratorRequiresTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"init":{"orchestrator":"orchestrator-md"}}}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "setup", "--target", target, "--json", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("setup with orchestrator config failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var report struct {
		Checks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Path   string `json:"path"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("decode setup report: %v\n%s", err, stdout)
	}
	found := false
	for _, check := range report.Checks {
		if check.ID == "orchestrator" {
			found = true
			if check.Status != "missing" || check.Path != "ORCHESTRATOR.md" {
				t.Fatalf("orchestrator check = %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("setup should report required orchestrator check: %#v", report.Checks)
	}
}

func TestInitRepoBinIsOptIn(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--repo-bin", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("repo-bin init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(target, "bin/burpvalve")); err != nil {
		t.Fatalf("--repo-bin should install bin/burpvalve: %v", err)
	}
}

func TestRepairSupportsSelectedAgentsTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, "AGENTS.md", "# Custom Contract\n")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "AGENTS.md")
	if err != nil {
		t.Fatalf("scoped repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	agents := string(mustReadCLIFile(t, target, "AGENTS.md"))
	for _, want := range []string{"# Custom Contract", "burpvalve:repair-start", "## Agent Startup", "## Atomic Work And Commits", "## Definition Of Done", "## Uncertainty", "## File Coordination"} {
		if !strings.Contains(agents, want) {
			t.Fatalf("scoped AGENTS.md repair should append %q while preserving custom file:\n%s", want, agents)
		}
	}
	for _, forbidden := range []string{"## Beads", "## Backpressure", "## Docs, Plans, And Logs"} {
		if strings.Contains(agents, forbidden) {
			t.Fatalf("scoped AGENTS.md repair should not append %q:\n%s", forbidden, agents)
		}
	}
	for _, rel := range []string{"CLAUDE.md", "log/README.md", "backpressure/README.md", ".githooks/pre-commit"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("scoped repair should not create %s, err=%v", rel, err)
		}
	}
}

func TestRepairClaudeTargetMigratesContentAndSymlinks(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, "CLAUDE.md", "# Human Claude Notes\n\nKeep this context.\n")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "--adopt-claude-md", "CLAUDE.md")
	if err != nil {
		t.Fatalf("CLAUDE repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	link, err := os.Readlink(filepath.Join(target, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be a symlink: %v", err)
	}
	if link != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink = %q, want AGENTS.md", link)
	}
	agents := string(mustReadCLIFile(t, target, "AGENTS.md"))
	for _, needle := range []string{"burpvalve:claude-import-start", "# Human Claude Notes", "Keep this context."} {
		if !strings.Contains(agents, needle) {
			t.Fatalf("AGENTS.md missing migrated CLAUDE content %q:\n%s", needle, agents)
		}
	}
	if _, err := os.Lstat(filepath.Join(target, "backpressure/README.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE repair should not create unrelated backpressure files, err=%v", err)
	}
}

func TestRepairClaudeTargetWithoutAdoptionReportsConflict(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, "CLAUDE.md", "# Human Claude Notes\n\nKeep this context.\n")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "CLAUDE.md")
	if err == nil {
		t.Fatalf("CLAUDE repair without adoption should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stdout), "regular file exists") && !strings.Contains(string(stderr), "regular file exists") {
		t.Fatalf("CLAUDE repair conflict should explain adoption need\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if got := string(mustReadCLIFile(t, target, "CLAUDE.md")); !strings.Contains(got, "Human Claude Notes") {
		t.Fatalf("CLAUDE.md should be preserved on conflict:\n%s", got)
	}
}

func TestInitClaudeRouteFlagCreatesOrchestratorSkillRoute(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--claude-route", "orchestrator-skill", "CLAUDE.md")
	if err != nil {
		t.Fatalf("orchestrator skill route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if info, err := os.Lstat(filepath.Join(target, "CLAUDE.md")); err != nil {
		t.Fatalf("CLAUDE.md missing: %v", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("orchestrator skill route should create regular CLAUDE.md")
	}
	if got := string(mustReadCLIFile(t, target, "CLAUDE.md")); !strings.Contains(got, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
		t.Fatalf("generated CLAUDE.md missing skill pointer:\n%s", got)
	}
	if _, err := os.Lstat(filepath.Join(target, ".claude/skills/burpvalve-orchestrator/SKILL.md")); err != nil {
		t.Fatalf("orchestrator skill package missing: %v", err)
	}
}

func TestClaudeRouteFreshInitAndSetupAcceptance(t *testing.T) {
	repoRoot := findRepoRoot(t)

	t.Run("default agent symlink route", func(t *testing.T) {
		target := t.TempDir()
		stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm", "--claude-route", "agent-symlink")
		if err != nil {
			t.Fatalf("default route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if targetPath, err := os.Readlink(filepath.Join(target, "CLAUDE.md")); err != nil || targetPath != "AGENTS.md" {
			t.Fatalf("CLAUDE.md symlink = %q err=%v, want AGENTS.md", targetPath, err)
		}
		if _, err := os.Lstat(filepath.Join(target, ".claude/skills/burpvalve-orchestrator")); !os.IsNotExist(err) {
			t.Fatalf("default route should not create orchestrator skill package, err=%v", err)
		}
		report := readSetupRouteReport(t, repoRoot, target)
		if report.ClaudeRoute.Expected != "agent-symlink" ||
			report.ClaudeRoute.Detected != "agent-symlink" ||
			report.ClaudeRoute.Source != "default" {
			t.Fatalf("default setup route report = %#v", report.ClaudeRoute)
		}
	})

	t.Run("orchestrator skill route", func(t *testing.T) {
		target := t.TempDir()
		writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"init":{"claude_route":"orchestrator-skill"}}}`)
		stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm")
		if err != nil {
			t.Fatalf("orchestrator route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if info, err := os.Lstat(filepath.Join(target, "CLAUDE.md")); err != nil {
			t.Fatalf("CLAUDE.md missing: %v", err)
		} else if info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("orchestrator route should create a regular CLAUDE.md")
		}
		if got := string(mustReadCLIFile(t, target, "CLAUDE.md")); !strings.Contains(got, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
			t.Fatalf("orchestrator CLAUDE.md missing skill pointer:\n%s", got)
		}
		for _, rel := range []string{
			"AGENTS.md",
			".claude/skills/burpvalve-orchestrator/SKILL.md",
			".claude/skills/burpvalve-orchestrator/SELF-TEST.md",
			".claude/skills/burpvalve-orchestrator/references/burpvalve-gate-choreography.md",
			".claude/skills/burpvalve-orchestrator/examples/gated-implementation-handoff.md",
			".claude/skills/burpvalve-orchestrator/scripts/pane_wake.py",
		} {
			if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); err != nil {
				t.Fatalf("orchestrator route missing %s: %v", rel, err)
			}
		}
		if _, err := os.Lstat(filepath.Join(target, "ORCHESTRATOR.md")); !os.IsNotExist(err) {
			t.Fatalf("orchestrator skill route should not create ORCHESTRATOR.md sidecar, err=%v", err)
		}
		report := readSetupRouteReport(t, repoRoot, target)
		if report.ClaudeRoute.Expected != "orchestrator-skill" ||
			report.ClaudeRoute.Detected != "orchestrator-skill" ||
			report.ClaudeRoute.Source != "project" {
			t.Fatalf("orchestrator setup route report = %#v", report.ClaudeRoute)
		}
	})
}

func TestClaudeRouteRepairAcceptance(t *testing.T) {
	repoRoot := findRepoRoot(t)

	t.Run("restores symlink route drift", func(t *testing.T) {
		target := t.TempDir()
		stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm", "--claude-route", "agent-symlink")
		if err != nil {
			t.Fatalf("symlink route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if err := os.Remove(filepath.Join(target, "CLAUDE.md")); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err = runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "--no-beads", "--claude-route", "agent-symlink", "CLAUDE.md")
		if err != nil {
			t.Fatalf("symlink route repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if targetPath, err := os.Readlink(filepath.Join(target, "CLAUDE.md")); err != nil || targetPath != "AGENTS.md" {
			t.Fatalf("CLAUDE.md symlink = %q err=%v, want AGENTS.md", targetPath, err)
		}
	})

	t.Run("restores orchestrator route drift", func(t *testing.T) {
		target := t.TempDir()
		stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--no-beads", "--no-ntm", "--claude-route", "orchestrator-skill", "CLAUDE.md")
		if err != nil {
			t.Fatalf("orchestrator route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		writeCLIFile(t, target, ".claude/skills/burpvalve-orchestrator/SKILL.md", "# drift\n")
		stdout, stderr, err = runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "--no-beads", "--claude-route", "orchestrator-skill", "CLAUDE.md")
		if err != nil {
			t.Fatalf("orchestrator route repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if got := string(mustReadCLIFile(t, target, "CLAUDE.md")); !strings.Contains(got, "Burpvalve Claude orchestrator route") {
			t.Fatalf("CLAUDE.md bootstrap not repaired:\n%s", got)
		}
		if got := string(mustReadCLIFile(t, target, ".claude/skills/burpvalve-orchestrator/SKILL.md")); !strings.Contains(got, "# burpvalve-orchestrator") {
			t.Fatalf("orchestrator skill package not repaired:\n%s", got)
		}
	})
}

func TestClaudeRouteConflictAdoptionAndNoAgentsAcceptance(t *testing.T) {
	repoRoot := findRepoRoot(t)

	t.Run("unmarked regular CLAUDE conflicts until adoption", func(t *testing.T) {
		target := t.TempDir()
		writeCLIFile(t, target, "CLAUDE.md", "# Human Claude Notes\n\nKeep this context.\n")
		stdout, stderr, err := runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "--no-beads", "CLAUDE.md")
		if err == nil {
			t.Fatalf("repair without adoption should fail\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		if !strings.Contains(string(stdout), "regular file exists") && !strings.Contains(string(stderr), "regular file exists") {
			t.Fatalf("repair conflict should mention regular file adoption\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		stdout, stderr, err = runBurpvalve(t, repoRoot, "repair", "--target", target, "--force", "--json", "--no-beads", "--adopt-claude-md", "CLAUDE.md")
		if err != nil {
			t.Fatalf("repair with adoption failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if targetPath, err := os.Readlink(filepath.Join(target, "CLAUDE.md")); err != nil || targetPath != "AGENTS.md" {
			t.Fatalf("adopted CLAUDE.md symlink = %q err=%v, want AGENTS.md", targetPath, err)
		}
		if got := string(mustReadCLIFile(t, target, "AGENTS.md")); !strings.Contains(got, "Human Claude Notes") {
			t.Fatalf("adoption did not import CLAUDE.md content:\n%s", got)
		}
	})

	t.Run("no agents with active route fails closed when dependency absent", func(t *testing.T) {
		target := t.TempDir()
		stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--no-beads", "--no-ntm", "--no-hooks", "--no-agents", "--claude-route", "agent-symlink")
		if err == nil {
			t.Fatalf("init --no-agents with active route should fail\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		if !strings.Contains(string(stdout), "active Claude route requires AGENTS.md") &&
			!strings.Contains(string(stderr), "active Claude route requires AGENTS.md") {
			t.Fatalf("missing AGENTS failure should explain dependency\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		if _, err := os.Lstat(filepath.Join(target, "AGENTS.md")); !os.IsNotExist(err) {
			t.Fatalf("failed --no-agents init should not create AGENTS.md, err=%v", err)
		}
		if _, err := os.Lstat(filepath.Join(target, "CLAUDE.md")); !os.IsNotExist(err) {
			t.Fatalf("failed --no-agents init should not create CLAUDE.md, err=%v", err)
		}
	})

	t.Run("no agents with existing dependency reports preservation", func(t *testing.T) {
		target := t.TempDir()
		writeCLIFile(t, target, "AGENTS.md", "# Existing Contract\n")
		stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--no-beads", "--no-ntm", "--no-hooks", "--no-agents", "--claude-route", "agent-symlink")
		if err != nil {
			t.Fatalf("init --no-agents with existing AGENTS failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		var result struct {
			Preserved []string `json:"preserved"`
		}
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("decode init result: %v\n%s", err, stdout)
		}
		if !containsString(result.Preserved, "AGENTS.md used (--no-agents)") {
			t.Fatalf("--no-agents result should report preserved AGENTS dependency: %#v", result.Preserved)
		}
		if targetPath, err := os.Readlink(filepath.Join(target, "CLAUDE.md")); err != nil || targetPath != "AGENTS.md" {
			t.Fatalf("CLAUDE.md symlink = %q err=%v, want AGENTS.md", targetPath, err)
		}
	})
}

func TestInitJSONReportsClaudeRouteAndSource(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--git-init", "--claude-route", "orchestrator-skill", "CLAUDE.md")
	if err != nil {
		t.Fatalf("route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result struct {
		ClaudeRoute       string `json:"claude_route"`
		ClaudeRouteSource string `json:"claude_route_source"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode init result: %v\n%s", err, stdout)
	}
	if result.ClaudeRoute != "orchestrator-skill" || result.ClaudeRouteSource != "input" {
		t.Fatalf("route metadata = %#v", result)
	}
}

func TestInitRejectsContradictoryClaudeRouteAndSkip(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--no-claude", "--claude-route", "orchestrator-skill")
	if err == nil {
		t.Fatalf("contradictory route flags should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), "--no-claude conflicts with --claude-route=orchestrator-skill") {
		t.Fatalf("conflict should explain route contradiction\nstdout=%s\nstderr=%s", stdout, stderr)
	}
}

func TestSetupJSONReportsConfiguredClaudeRouteSource(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"init":{"claude_route":"orchestrator-skill"}}}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "setup", "--target", target, "--json", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("setup with route config failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var report struct {
		ClaudeRoute struct {
			Expected string `json:"expected"`
			Source   string `json:"source"`
			Detected string `json:"detected"`
		} `json:"claude_route"`
		Config struct {
			Settings []struct {
				Key    string `json:"key"`
				Source string `json:"source"`
				Value  string `json:"value"`
			} `json:"settings"`
		} `json:"config"`
	}
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("decode setup report: %v\n%s", err, stdout)
	}
	if report.ClaudeRoute.Expected != "orchestrator-skill" || report.ClaudeRoute.Source != "project" || report.ClaudeRoute.Detected != "none" {
		t.Fatalf("route facts = %#v", report.ClaudeRoute)
	}
	foundSetting := false
	for _, setting := range report.Config.Settings {
		if setting.Key == "defaults.init.claude_route" {
			foundSetting = true
			if setting.Source != "project" || setting.Value != "orchestrator-skill" {
				t.Fatalf("route setting = %#v", setting)
			}
		}
	}
	if !foundSetting {
		t.Fatalf("config settings missing defaults.init.claude_route: %#v", report.Config.Settings)
	}
}

func TestRobotsInitClaudeRouteCreatesOrchestratorSkillRoute(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	input, err := json.Marshal(map[string]any{
		"target":       target,
		"targets":      []string{"CLAUDE.md"},
		"claude_route": "orchestrator-skill",
		"confirm":      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, string(input), "init", "--robots")
	if err != nil {
		t.Fatalf("robots orchestrator route init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result struct {
		ClaudeRoute       string `json:"claude_route"`
		ClaudeRouteSource string `json:"claude_route_source"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode robots init result: %v\n%s", err, stdout)
	}
	if result.ClaudeRoute != "orchestrator-skill" || result.ClaudeRouteSource != "input" {
		t.Fatalf("robot route metadata = %#v", result)
	}
	if info, err := os.Lstat(filepath.Join(target, "CLAUDE.md")); err != nil {
		t.Fatalf("CLAUDE.md missing: %v", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("robot orchestrator route should create regular CLAUDE.md")
	}
}

func TestRobotsRepairAdoptClaudeMDMigratesContentAndSymlinks(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	writeCLIFile(t, target, "CLAUDE.md", "# Human Claude Notes\n\nKeep this context.\n")
	input, err := json.Marshal(map[string]any{
		"target":          target,
		"targets":         []string{"CLAUDE.md"},
		"adopt_claude_md": true,
		"confirm":         true,
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, string(input), "repair", "--robots")
	if err != nil {
		t.Fatalf("robots CLAUDE adoption repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	link, err := os.Readlink(filepath.Join(target, "CLAUDE.md"))
	if err != nil || link != "AGENTS.md" {
		t.Fatalf("CLAUDE.md symlink = %q err=%v, want AGENTS.md", link, err)
	}
	agents := string(mustReadCLIFile(t, target, "AGENTS.md"))
	if !strings.Contains(agents, "Human Claude Notes") {
		t.Fatalf("robots adoption did not import CLAUDE.md content:\n%s", agents)
	}
}

func TestSkipFlagsTrimWizardQuestions(t *testing.T) {
	initSkips := initWizardSkips(initOptions{noBeads: true, noNTM: true, noHooks: true})
	if !initSkips.Beads || !initSkips.NTM || !initSkips.Hooks || !initSkips.HooksPath || initSkips.Bin {
		t.Fatalf("init skip flags should map to wizard skips: %#v", initSkips)
	}
	if initSkips.Agents || initSkips.Claude {
		t.Fatalf("unrelated init questions should not be skipped: %#v", initSkips)
	}

	repairSkips := repairWizardSkips(repairOptions{noBeads: true, noHooks: true})
	if !repairSkips.Beads || !repairSkips.Hooks || !repairSkips.HooksPath || repairSkips.Bin {
		t.Fatalf("repair skip flags should map to wizard skips: %#v", repairSkips)
	}
	if repairSkips.Agents || repairSkips.Claude {
		t.Fatalf("unrelated repair questions should not be skipped: %#v", repairSkips)
	}
}

func TestInitWizardResultSuppressesHiddenOrchestratorSelection(t *testing.T) {
	var opts initOptions
	applyInitWizardResult(&opts, charmui.InitWizardResult{
		Target:       ".",
		Agents:       true,
		Claude:       true,
		Docs:         true,
		Plans:        true,
		Log:          true,
		Backpressure: true,
		Attestations: true,
		Hooks:        true,
		HooksPath:    true,
		ToolDocs:     true,
		Beads:        false,
		NTM:          false,
		Orchestrator: true,
	})
	if opts.orchestrator {
		t.Fatalf("hidden TUI orchestrator selection should not reach init options: %#v", opts)
	}
}

func TestForceSkipsQuestionAnswerFlow(t *testing.T) {
	if shouldRunInitWizard(initOptions{force: true}) {
		t.Fatal("init --force should skip the question-and-answer flow")
	}
	if shouldRunRepairWizard(repairOptions{force: true}) {
		t.Fatal("repair --force should skip the question-and-answer flow")
	}
}

func TestInteractiveGitInitPromptConditions(t *testing.T) {
	target := t.TempDir()
	if !needsGitInitPrompt(target, false, false, false, false, false) {
		t.Fatal("interactive hook setup in non-git repo should ask about git init")
	}
	for _, tt := range []struct {
		name        string
		force       bool
		gitInit     bool
		noHooks     bool
		noPreCommit bool
		noHooksPath bool
	}{
		{name: "force", force: true},
		{name: "already requested", gitInit: true},
		{name: "no hooks", noHooks: true},
		{name: "both hook pieces skipped", noPreCommit: true, noHooksPath: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if needsGitInitPrompt(target, tt.force, tt.gitInit, tt.noHooks, tt.noPreCommit, tt.noHooksPath) {
				t.Fatal("prompt should be skipped")
			}
		})
	}
	gitTarget := t.TempDir()
	if err := os.Mkdir(filepath.Join(gitTarget, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if needsGitInitPrompt(gitTarget, false, false, false, false, false) {
		t.Fatal("existing git repo should not ask about git init")
	}
}

func TestJSONInitAndRepairFailClosedWithoutForce(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	_, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--json", "log")
	if err == nil || !strings.Contains(string(stderr), "init --json will not change files without confirmation") {
		t.Fatalf("init --json without --force should fail closed, err=%v stderr=%s", err, stderr)
	}
	if _, statErr := os.Lstat(filepath.Join(target, "log/README.md")); !os.IsNotExist(statErr) {
		t.Fatalf("init --json without --force should not create log/README.md, err=%v", statErr)
	}

	_, stderr, err = runBurpvalve(t, repoRoot, "repair", "--target", target, "--json", "AGENTS.md")
	if err == nil || !strings.Contains(string(stderr), "repair --json will not change files without confirmation") {
		t.Fatalf("repair --json without --force should fail closed, err=%v stderr=%s", err, stderr)
	}
	if _, statErr := os.Lstat(filepath.Join(target, "AGENTS.md")); !os.IsNotExist(statErr) {
		t.Fatalf("repair --json without --force should not create AGENTS.md, err=%v", statErr)
	}
}

func TestRobotsInitReadsJSONInput(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	input, err := json.Marshal(map[string]any{
		"target":   target,
		"confirm":  true,
		"git_init": true,
		"skip": map[string]bool{
			"beads": true,
			"ntm":   true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, string(input), "init", "--robots")
	if err != nil {
		t.Fatalf("robots init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result struct {
		Skipped []string `json:"skipped"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode robots init result: %v\n%s", err, stdout)
	}
	if _, err := os.Lstat(filepath.Join(target, ".beads")); !os.IsNotExist(err) {
		t.Fatalf("robots skip.beads should skip .beads, err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "bin/burpvalve")); !os.IsNotExist(err) {
		t.Fatalf("robots init should not install repo-local bin/burpvalve by default, err=%v", err)
	}
	if !containsString(result.Skipped, ".beads (--no-beads)") {
		t.Fatalf("robots init skipped output missing beads skip: %#v", result.Skipped)
	}
}

func TestRobotsInitSupportsExplicitOrchestratorTarget(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	input, err := json.Marshal(map[string]any{
		"target":  target,
		"targets": []string{"orchestrator"},
		"dogfood": true,
		"confirm": true,
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, string(input), "init", "--robots")
	if err != nil {
		t.Fatalf("robots orchestrator init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(target, "ORCHESTRATOR.md")); err != nil {
		t.Fatalf("robots explicit orchestrator target should create ORCHESTRATOR.md: %v", err)
	}
	body := string(mustReadCLIFile(t, target, "ORCHESTRATOR.md"))
	if !strings.Contains(body, "## Dogfood Findings") {
		t.Fatalf("robots dogfood=true should add dogfood block:\n%s", body)
	}
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "backpressure/manifest.yaml", ".githooks/pre-commit", ".beads"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("robots scoped orchestrator init should not create %s, err=%v", rel, err)
		}
	}
}

func TestInitHookWiringRequiresGitInitOrNoHooks(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()

	stdout, stderr, err := runBurpvalve(t, repoRoot, "init", "--target", target, "--force", "--json", "--no-beads", "--no-ntm", "hooks")
	if err == nil {
		t.Fatalf("init hooks in non-git repo should require --git-init or --no-hooks\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	var blocked struct {
		Status    string `json:"status"`
		Conflicts []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(stdout, &blocked); err != nil {
		t.Fatalf("decode blocked init result: %v\n%s", err, stdout)
	}
	if blocked.Status != "partial_success" || len(blocked.Conflicts) == 0 || blocked.Conflicts[0].Path != ".git" {
		t.Fatalf("unexpected blocked init result: %#v", blocked)
	}

	targetNoHooks := t.TempDir()
	stdout, stderr, err = runBurpvalve(t, repoRoot, "init", "--target", targetNoHooks, "--force", "--json", "--no-hooks", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("init --no-hooks should not require git: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(targetNoHooks, ".git")); !os.IsNotExist(err) {
		t.Fatalf("--no-hooks should not initialize git, err=%v", err)
	}

	targetNoHooksPath := t.TempDir()
	stdout, stderr, err = runBurpvalve(t, repoRoot, "init", "--target", targetNoHooksPath, "--force", "--json", "--no-hooks-path", "--no-beads", "--no-ntm", "hooks")
	if err != nil {
		t.Fatalf("init --no-hooks-path should create hook file without requiring git: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(targetNoHooksPath, ".githooks/pre-commit")); err != nil {
		t.Fatalf("--no-hooks-path should still create hook file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(targetNoHooksPath, ".git")); !os.IsNotExist(err) {
		t.Fatalf("--no-hooks-path should not initialize git, err=%v", err)
	}
}

func TestRobotsInitRequiresConfirmation(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := t.TempDir()
	input, err := json.Marshal(map[string]any{
		"target":  target,
		"targets": []string{"log"},
	})
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, string(input), "init", "--robots")
	if err == nil {
		t.Fatalf("robots init without confirm should fail closed\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), "init --json will not change files without confirmation") {
		t.Fatalf("robots init without confirm stderr = %s", stderr)
	}
	if _, statErr := os.Lstat(filepath.Join(target, "log/README.md")); !os.IsNotExist(statErr) {
		t.Fatalf("robots init without confirm should not create log/README.md, err=%v", statErr)
	}
}

func mustReadCLIFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func readSetupRouteReport(t *testing.T, repoRoot, target string) struct {
	ClaudeRoute struct {
		Expected string `json:"expected"`
		Detected string `json:"detected"`
		Source   string `json:"source"`
	} `json:"claude_route"`
} {
	t.Helper()
	stdout, stderr, err := runBurpvalve(t, repoRoot, "setup", "--target", target, "--json", "--no-beads", "--no-ntm")
	if err != nil {
		t.Fatalf("setup route report failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var report struct {
		ClaudeRoute struct {
			Expected string `json:"expected"`
			Detected string `json:"detected"`
			Source   string `json:"source"`
		} `json:"claude_route"`
	}
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("decode setup report: %v\n%s", err, stdout)
	}
	return report
}
