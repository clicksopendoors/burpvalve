package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/charmui"
)

func TestRootHelpIsDeveloperGrade(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("-h")
	if err != nil {
		t.Fatalf("root help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Burpvalve sets up a repo so each work unit (the atomic change being checked) is checked before it is committed.",
		"Terminal runs use a question-and-answer flow for setup and attestation evidence (the seal written for checked work).",
		"Scripts can use --json and --responses",
		"Automation can use --robots",
		"Set NO_TUI=1",
		"Quick Start:",
		"Shell Setup:",
		"Usage:",
		"Available Commands:",
		"completion",
		"Print shell completion setup for your shell",
		"  init",
		"Add Burpvalve files and the local commit check",
		"  commit",
		"Check the files you are about to commit",
		`Use "burpvalve [command] -h" for more information about a command.`,
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("root help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("help should not write stderr, got: %s", stderr)
	}
	for _, forbidden := range []string{
		"developer inspecting it for the first time",
		"meant to be readable",
		"built for agents that loop often",
		"agent-loop substrate",
		"staged-change backpressure gate",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("root help leaked meta positioning %q:\n%s", forbidden, stdout)
		}
	}
}

func TestBareInvocationShowsRootHelp(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand()
	if err != nil {
		t.Fatalf("bare invocation should show help: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Quick Start:") || !strings.Contains(stdout, "Available Commands:") {
		t.Fatalf("bare invocation did not show root help:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("bare invocation should not write stderr, got: %s", stderr)
	}
}

func TestInitHelpExplainsWhatItAddsAndFlags(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("init", "-h")
	if err != nil {
		t.Fatalf("init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Add the standard Burpvalve files to a repo",
		"Bubble Tea question-and-answer setup flow",
		"flags hide those pieces from",
		"Use --force to skip questions",
		"use --robots with JSON input",
		"appends the missing",
		"Common targets: AGENTS.md, CLAUDE.md",
		"Quick Start:",
		"Usage:",
		"burpvalve init [target...] [flags]",
		"--force",
		"--no-beads",
		"--no-ntm",
		"--no-claude-symlink",
		"--no-agents-md",
		"--no-log",
		"--no-attestations",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("init help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("help should not write stderr, got: %s", stderr)
	}
}

func TestCommitHelpExplainsInteractiveAndNonInteractiveUse(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("commit", "-h")
	if err != nil {
		t.Fatalf("commit help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Check the work unit (the atomic staged change)",
		"seal, also called an attestation (the evidence artifact)",
		"valve (the fail-closed",
		"burped back",
		"refused by the valve",
		"asks for it",
		"asks Bubble Tea questions",
		"Use --responses FILE for",
		"Use --responses-template",
		"NO_TUI=1",
		"The git hook calls this command",
		"burpvalve commit --responses-template --feature br-123 > responses.json",
		"--responses",
		"--responses-template",
		"--feature",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("commit help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("help should not write stderr, got: %s", stderr)
	}
}

func TestAttestationsHelpDefinesSealAndAttestation(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("attestations", "-h")
	if err != nil {
		t.Fatalf("attestations help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Read seals, also called attestations (the evidence artifacts)",
		"blocked reports without changing files",
		"Use JSON output for agents and scripts",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("attestations help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("attestations help should not write stderr, got: %s", stderr)
	}
}

func TestCommitRobotsHelpIncludesResponsesSchema(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "commit", "-h")
	if err != nil {
		t.Fatalf("robots commit help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots help should not write stderr, got: %s", stderr)
	}
	var doc struct {
		Command    string         `json:"command"`
		Flags      []robotFlag    `json:"flags"`
		RobotInput map[string]any `json:"robot_input"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode robots commit help: %v\n%s", err, stdout)
	}
	if doc.Command != "burpvalve commit" {
		t.Fatalf("unexpected command: %#v", doc)
	}
	if !robotHelpHasFlag(doc.Flags, "--responses-template") {
		t.Fatalf("robots commit help missing --responses-template: %#v", doc.Flags)
	}
	if _, ok := doc.RobotInput["responses_file_schema"]; !ok {
		t.Fatalf("robots commit help missing responses_file_schema: %#v", doc.RobotInput)
	}
	if containsStandaloneBurpLanguage(stdout) {
		for _, needle := range []string{
			"valve (the fail-closed",
			"burped back",
			"meaning refused by the valve",
		} {
			if !strings.Contains(stdout, needle) {
				t.Fatalf("robots commit help uses burp language without local definition %q:\n%s", needle, stdout)
			}
		}
	}
}

func TestRepairHelpExplainsTargetsAndSkipFlags(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("repair", "-h")
	if err != nil {
		t.Fatalf("repair help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Bubble Tea question-and-answer repair flow",
		"flags hide those pieces from",
		"Use --force to skip questions",
		"Common targets: AGENTS.md, CLAUDE.md",
		"imports any",
		"burpvalve repair AGENTS.md",
		"burpvalve repair hooks",
		"burpvalve repair [target...] [flags]",
		"--force",
		"--no-agents",
		"--no-hooks-path",
		"--no-attestations",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("repair help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("help should not write stderr, got: %s", stderr)
	}
}

func TestCommandHelpDoesNotRenderModesOrOutputSections(t *testing.T) {
	for _, args := range [][]string{
		{"setup", "-h"},
		{"init", "-h"},
		{"repair", "-h"},
		{"commit", "-h"},
		{"lint", "-h"},
		{"ci", "-h"},
	} {
		stdout, stderr, err := executeBurpvalveCommand(args...)
		if err != nil {
			t.Fatalf("%s help failed: %v\nstdout=%s\nstderr=%s", strings.Join(args, " "), err, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("%s help should not write stderr, got: %s", strings.Join(args, " "), stderr)
		}
		for _, forbidden := range []string{"Modes:", "Output:"} {
			if strings.Contains(stdout, forbidden) {
				t.Fatalf("%s help should not render %q:\n%s", strings.Join(args, " "), forbidden, stdout)
			}
		}
	}
}

func TestCompletionHelpExplainsAutoDetect(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("completion", "-h")
	if err != nil {
		t.Fatalf("completion help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"With no shell argument, Burpvalve opens an interactive installer",
		"Pass a shell name when you want the raw completion script",
		"burpvalve completion",
		"burpvalve completion install",
		"verify",
		"burpvalve completion zsh >",
		"Install shell completions without the setup wizard",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("completion help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("completion help should not write stderr, got: %s", stderr)
	}
}

func TestCompletionAutoDetectsShellFromEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	stdout, stderr, err := executeBurpvalveCommand("completion")
	if err != nil {
		t.Fatalf("completion auto-detect failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Shell completions",
		"Detected shell:",
		"fish",
		"burpvalve completion install --shell fish",
		"burpvalve completion fish >",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("completion guide missing %q:\n%s", needle, stdout)
		}
	}
	if strings.Contains(stdout, "complete -c burpvalve") {
		t.Fatalf("bare completion should show a guide, not raw fish script:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("completion should not write stderr, got: %s", stderr)
	}
}

func TestCompletionGuideLabelsConfiguredShell(t *testing.T) {
	project := t.TempDir()
	t.Setenv("SHELL", "/usr/bin/zsh")
	completionPath := filepath.Join(project, "fish", "burpvalve.fish")
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {
    "shell": "fish",
    "completion": {
      "path": "`+strings.ReplaceAll(completionPath, `\`, `\\`)+`"
    }
  }
}`)
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeBurpvalveCommand("completion")
	if err != nil {
		t.Fatalf("completion guide with config failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Configured shell:",
		"fish",
		"(project config)",
		completionPath,
		"burpvalve completion install --shell fish",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("completion configured guide missing %q:\n%s", needle, stdout)
		}
	}
	if strings.Contains(stdout, "Detected shell:") {
		t.Fatalf("configured shell should not be mislabeled as detected:\n%s", stdout)
	}
}

func TestCompletionExplicitShellEmitsScript(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("completion", "fish")
	if err != nil {
		t.Fatalf("completion fish failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "complete -c burpvalve") {
		t.Fatalf("explicit shell should emit fish script:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("completion script should not write stderr, got: %s", stderr)
	}
}

func TestCompletionInstallWritesScriptAndRC(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := home + "/completion/_burpvalve"
	rcFile := home + "/.zshrc"
	stdout, stderr, err := executeBurpvalveCommand("completion", "install", "--shell", "zsh", "--path", path, "--rc-file", rcFile, "--update-rc", "--force")
	if err != nil {
		t.Fatalf("completion install failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	script := readTestFile(t, path)
	if !strings.Contains(script, "#compdef burpvalve") {
		t.Fatalf("installed zsh completion script looks wrong:\n%s", script)
	}
	rc := readTestFile(t, rcFile)
	for _, needle := range []string{"# >>> burpvalve completion zsh >>>", "fpath=(", "compinit"} {
		if !strings.Contains(rc, needle) {
			t.Fatalf("zshrc missing %q:\n%s", needle, rc)
		}
	}
	for _, needle := range []string{"Completion install plan", "completion file", "_burpvalve", "shell startup", ".zshrc", "Completion setup", "Wrote:", "Updated:"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("install output missing %q:\n%s", needle, stdout)
		}
	}
	if strings.Index(stdout, "Completion install plan") > strings.Index(stdout, "Completion setup") {
		t.Fatalf("install plan should print before write summary:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Completion setup") || !strings.Contains(stdout, "Wrote:") || !strings.Contains(stdout, "Updated:") {
		t.Fatalf("install summary missing expected lines:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("completion install should not write stderr, got: %s", stderr)
	}
}

func TestCompletionInstallRequiresConfirmationWithoutForce(t *testing.T) {
	_, _, err := executeBurpvalveCommand("completion", "install", "--shell", "zsh")
	if err == nil || !strings.Contains(err.Error(), "completion install requires confirmation") {
		t.Fatalf("completion install without force should require confirmation, got: %v", err)
	}
}

func TestCompletionVerifyJSONReadyZsh(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	completionPath := filepath.Join(home, "completion", "_burpvalve")
	rcFile := filepath.Join(home, ".zshrc")
	writeExecutable(t, filepath.Join(binDir, "burpvalve"), "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo test-version; exit 0; fi\nexit 0\n")
	writeCmdTestFile(t, completionPath, "#compdef burpvalve\n")
	writeCmdTestFile(t, rcFile, completionRCMarker("zsh")+"\nfpath=("+filepath.Dir(completionPath)+" $fpath)\n")
	t.Setenv("PATH", binDir)

	stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--shell", "zsh", "--path", completionPath, "--rc-file", rcFile, "--update-rc", "--bin-dir", binDir, "--json")
	if err != nil {
		t.Fatalf("completion verify failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("completion verify wrote stderr: %s", stderr)
	}
	var got completionVerifyReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode completion verify: %v\n%s", err, stdout)
	}
	if got.Status != "ready" || !got.Verified || got.Mutating {
		t.Fatalf("expected ready non-mutating verify report: %#v", got)
	}
	if got.Shell != "zsh" || got.ShellSource != "flag" || got.CommandOrigin != "path" || !got.CommandOnPath {
		t.Fatalf("unexpected shell/path facts: %#v", got)
	}
	if got.Version != "test-version" || !got.ReloadNeeded || !strings.Contains(got.ReloadCommand, rcFile) || got.RCPath != rcFile || !strings.Contains(stdout, `"next_steps": []`) {
		t.Fatalf("expected version and reload fields in JSON contract: %#v", got)
	}
	if !got.CompletionExists || !got.CompletionLooksOK || !got.RCRequired || !got.RCUpdatePresent || !got.BinDirExists || !got.PathContainsBinDir {
		t.Fatalf("expected all zsh checks true: %#v", got)
	}
	if len(got.NextSteps) != 0 {
		t.Fatalf("ready verification should not have recovery steps: %#v", got.NextSteps)
	}
}

func TestCompletionVerifyReportsRepoLocalOnlyAndMissingCompletion(t *testing.T) {
	target := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "missing-bin")
	completionPath := filepath.Join(t.TempDir(), "fish", "burpvalve.fish")
	writeExecutable(t, filepath.Join(target, "bin", "burpvalve"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--target", target, "--shell", "fish", "--path", completionPath, "--bin-dir", binDir, "--json")
	if err != nil {
		t.Fatalf("completion verify repo-local failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("completion verify wrote stderr: %s", stderr)
	}
	var got completionVerifyReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode completion verify repo-local: %v\n%s", err, stdout)
	}
	if got.Status != "action_needed" || got.Verified {
		t.Fatalf("missing completion should require action: %#v", got)
	}
	if got.CommandOrigin != "repo-local" || got.CommandOnPath || !got.RepoLocalExists {
		t.Fatalf("expected repo-local-only command facts: %#v", got)
	}
	if got.CompletionExists || got.CompletionLooksOK || got.BinDirExists || got.PathContainsBinDir {
		t.Fatalf("expected missing completion/bin facts: %#v", got)
	}
	if !strings.Contains(strings.Join(got.NextSteps, "\n"), "global command shim") || !strings.Contains(strings.Join(got.NextSteps, "\n"), "completion install --shell fish") {
		t.Fatalf("next steps should explain global command and completion install: %#v", got.NextSteps)
	}
}

func TestCompletionVerifyReportsPathAndRepoLocal(t *testing.T) {
	target := t.TempDir()
	globalBin := filepath.Join(t.TempDir(), "global-bin")
	completionPath := filepath.Join(t.TempDir(), "fish", "burpvalve.fish")
	writeExecutable(t, filepath.Join(globalBin, "burpvalve"), "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo global-version; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(target, "bin", "burpvalve"), "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo repo-version; exit 0; fi\nexit 0\n")
	writeCmdTestFile(t, completionPath, "complete -c burpvalve\n")
	t.Setenv("PATH", globalBin)

	stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--target", target, "--shell", "fish", "--path", completionPath, "--bin-dir", globalBin, "--json")
	if err != nil {
		t.Fatalf("completion verify path+repo-local failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("completion verify wrote stderr: %s", stderr)
	}
	var got completionVerifyReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode completion verify path+repo-local: %v\n%s", err, stdout)
	}
	if got.CommandOrigin != "path-and-repo-local" || !got.CommandOnPath || !got.RepoLocalExists {
		t.Fatalf("expected global PATH command plus repo-local fallback facts: %#v", got)
	}
	if got.CommandPath != filepath.Join(globalBin, "burpvalve") || got.Version != "global-version" {
		t.Fatalf("expected global command to win over repo-local fallback: %#v", got)
	}
	if strings.Contains(strings.Join(got.NextSteps, "\n"), "global command shim") {
		t.Fatalf("next steps should not ask for a global shim when command is on PATH: %#v", got.NextSteps)
	}
}

func TestCompletionVerifyNoPathModeDoesNotRequireCommand(t *testing.T) {
	completionPath := filepath.Join(t.TempDir(), "fish", "burpvalve.fish")
	writeCmdTestFile(t, completionPath, "complete -c burpvalve\n")
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--shell", "fish", "--path", completionPath, "--bin-dir", filepath.Join(t.TempDir(), "missing-bin"), "--no-path", "--json")
	if err != nil {
		t.Fatalf("completion verify --no-path failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("completion verify wrote stderr: %s", stderr)
	}
	var got completionVerifyReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode completion verify no-path: %v\n%s", err, stdout)
	}
	if got.Status != "ready" || !got.Verified {
		t.Fatalf("--no-path should allow completion-only setup to verify: %#v", got)
	}
	if got.CommandOrigin != "missing" || got.CommandOnPath {
		t.Fatalf("command should still be reported missing without blocking: %#v", got)
	}
}

func TestCompletionVerifyUsesConfiguredDefaultsAndHumanOutput(t *testing.T) {
	project := t.TempDir()
	completionPath := filepath.Join(project, "fish", "burpvalve.fish")
	writeCmdTestFile(t, completionPath, "complete -c burpvalve\n")
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {
    "shell": "fish",
    "completion": {
      "path": "`+strings.ReplaceAll(completionPath, `\`, `\\`)+`"
    }
  }
}`)
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--target", project, "--no-path", "--color", "never")
	if err != nil {
		t.Fatalf("completion verify human output failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("completion verify wrote stderr: %s", stderr)
	}
	for _, needle := range []string{"Completion verification for fish", "shell", "fish (project config)", "completion", completionPath, "config defaults", "defaults.shell = fish"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("completion verify human output missing %q:\n%s", needle, stdout)
		}
	}
}

func TestCompletionVerifyFlagsOverrideConfiguredDefaults(t *testing.T) {
	project := t.TempDir()
	configuredPath := filepath.Join(project, "fish", "burpvalve.fish")
	flagPath := filepath.Join(project, "bash", "burpvalve")
	writeCmdTestFile(t, configuredPath, "complete -c burpvalve\n")
	writeCmdTestFile(t, flagPath, "__start_burpvalve\n")
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {
    "shell": "fish",
    "completion": {
      "path": "`+strings.ReplaceAll(configuredPath, `\`, `\\`)+`"
    }
  }
}`)
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--target", project, "--shell", "bash", "--path", flagPath, "--no-path", "--json")
	if err != nil {
		t.Fatalf("completion verify flag override failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("completion verify wrote stderr: %s", stderr)
	}
	var got completionVerifyReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode completion verify flag override: %v\n%s", err, stdout)
	}
	if got.Shell != "bash" || got.ShellSource != "flag" || got.CompletionPath != flagPath || !got.CompletionLooksOK {
		t.Fatalf("flags should override configured shell/path: %#v", got)
	}
}

func TestCompletionVerifyBashAndPowerShellReports(t *testing.T) {
	for _, tc := range []struct {
		shell  string
		body   string
		rcBody string
	}{
		{shell: "bash", body: "__start_burpvalve\n", rcBody: completionRCMarker("bash") + "\n. /example/burpvalve\n"},
		{shell: "powershell", body: "Register-ArgumentCompleter -CommandName burpvalve\n", rcBody: completionRCMarker("powershell") + "\n. '/example/burpvalve.ps1'\n"},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			home := t.TempDir()
			completionPath := filepath.Join(home, "completion")
			rcFile := filepath.Join(home, "profile")
			writeCmdTestFile(t, completionPath, tc.body)
			writeCmdTestFile(t, rcFile, tc.rcBody)
			t.Setenv("PATH", t.TempDir())
			stdout, stderr, err := executeBurpvalveCommand("completion", "verify", "--shell", tc.shell, "--path", completionPath, "--rc-file", rcFile, "--update-rc", "--no-path", "--json")
			if err != nil {
				t.Fatalf("completion verify %s failed: %v\nstdout=%s\nstderr=%s", tc.shell, err, stdout, stderr)
			}
			if stderr != "" {
				t.Fatalf("completion verify wrote stderr: %s", stderr)
			}
			var got completionVerifyReport
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("decode completion verify %s: %v\n%s", tc.shell, err, stdout)
			}
			if got.Shell != tc.shell || !got.CompletionLooksOK || !got.RCUpdatePresent || !got.ReloadNeeded || got.ReloadCommand == "" {
				t.Fatalf("verify report for %s missing expected facts: %#v", tc.shell, got)
			}
		})
	}
}

func TestCompletionVerifyReportsAmbiguousShellDetection(t *testing.T) {
	previousPID := completionParentPID
	previousDetector := completionParentShellDetector
	completionParentPID = func() int { return 12345 }
	completionParentShellDetector = func(int) (string, string, bool) {
		return "", "parent process=unknown (unsupported)", false
	}
	t.Cleanup(func() {
		completionParentPID = previousPID
		completionParentShellDetector = previousDetector
	})
	t.Setenv("SHELL", "/bin/tcsh")

	_, err := buildCompletionVerifyReport(completionVerifyOptions{target: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "could not detect shell") {
		t.Fatalf("ambiguous shell detection should ask for explicit shell, got %v", err)
	}
	_, evidence, _ := detectCompletionShellWithEvidence()
	joined := strings.Join(evidence, "\n")
	if !strings.Contains(joined, "SHELL=/bin/tcsh (unsupported)") || !strings.Contains(joined, "parent process=unknown (unsupported)") {
		t.Fatalf("ambiguous detection should retain evidence, got %#v", evidence)
	}
}

func TestCompletionInstallCancelLeavesNoPartialWrites(t *testing.T) {
	previousInteractive := isInteractiveTerminal
	isInteractiveTerminal = func(*os.File, *os.File) bool { return true }
	t.Cleanup(func() { isInteractiveTerminal = previousInteractive })
	withConfigPromptStubs(t,
		func(io.Reader, io.Writer, charmui.ConfirmPrompt) (bool, error) {
			return false, nil
		},
		nil,
		nil,
	)
	path := filepath.Join(t.TempDir(), "completion", "_burpvalve")
	rcFile := filepath.Join(t.TempDir(), ".zshrc")
	stdout, stderr, err := executeBurpvalveCommand("completion", "install", "--shell", "zsh", "--path", path, "--rc-file", rcFile, "--update-rc")
	if err == nil || !strings.Contains(err.Error(), "completion install cancelled; no files changed") {
		t.Fatalf("completion install cancellation should stop before writes\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("completion script should not be written after cancel, stat=%v", statErr)
	}
	if _, statErr := os.Stat(rcFile); !os.IsNotExist(statErr) {
		t.Fatalf("rc file should not be written after cancel, stat=%v", statErr)
	}
}

func TestCompletionInstallPlanSupportsAllShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		plan, err := completionInstallPlanFor(shell, "", "", false, true, filepath.Join(t.TempDir(), "bin"))
		if err != nil {
			t.Fatalf("completion plan for %s failed: %v", shell, err)
		}
		if plan.Shell != shell || plan.Path == "" || plan.ShimPath == "" || plan.PathRCFile == "" {
			t.Fatalf("completion plan for %s incomplete: %#v", shell, plan)
		}
	}
}

func TestCompletionShellNormalization(t *testing.T) {
	for input, want := range map[string]string{
		"/bin/bash":      "bash",
		"-zsh":           "zsh",
		"/opt/bin/fish":  "fish",
		"pwsh":           "powershell",
		"powershell.exe": "powershell",
	} {
		got, ok := normalizeCompletionShell(input)
		if !ok || got != want {
			t.Fatalf("normalizeCompletionShell(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
	if got, ok := normalizeCompletionShell("tcsh"); ok || got != "" {
		t.Fatalf("normalizeCompletionShell(tcsh) = %q, %v; want empty false", got, ok)
	}
}

func writeExecutable(t *testing.T, path string, body string) {
	t.Helper()
	writeCmdTestFile(t, path, body)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCompletionWizardPromptExplainsDetectedShell(t *testing.T) {
	defaultShell, description := completionWizardShellPrompt(completionShellSelection{Shell: "zsh", Source: "detected"})
	if defaultShell != "zsh" {
		t.Fatalf("default shell = %q; want zsh", defaultShell)
	}
	for _, needle := range []string{"Detected shell: zsh", "Choose the shell Burpvalve should set up."} {
		if !strings.Contains(description, needle) {
			t.Fatalf("completion wizard prompt missing %q:\n%s", needle, description)
		}
	}
}

func TestCompletionWizardPromptExplainsConfiguredShell(t *testing.T) {
	defaultShell, description := completionWizardShellPrompt(completionShellSelection{Shell: "fish", Source: "project"})
	if defaultShell != "fish" {
		t.Fatalf("default shell = %q; want fish", defaultShell)
	}
	for _, needle := range []string{"Configured shell: fish (project config)", "Choose the shell Burpvalve should set up."} {
		if !strings.Contains(description, needle) {
			t.Fatalf("completion wizard configured prompt missing %q:\n%s", needle, description)
		}
	}
}

func TestCompletionWizardPromptExplainsFallback(t *testing.T) {
	defaultShell, description := completionWizardShellPrompt(completionShellSelection{})
	if defaultShell != "zsh" {
		t.Fatalf("fallback shell = %q; want zsh", defaultShell)
	}
	if !strings.Contains(description, "could not detect your shell") {
		t.Fatalf("completion wizard fallback prompt did not explain fallback:\n%s", description)
	}
}

func TestCompletionRejectsUnknownExplicitShell(t *testing.T) {
	_, _, err := executeBurpvalveCommand("completion", "tcsh")
	if err == nil || !strings.Contains(err.Error(), `unknown shell "tcsh"; expected bash, zsh, fish, or powershell`) {
		t.Fatalf("completion unknown shell error = %v", err)
	}
}

func TestColorFlagValidation(t *testing.T) {
	_, _, err := executeBurpvalveCommand("setup", "--color", "loud")
	if err == nil || !strings.Contains(err.Error(), `invalid --color "loud"; expected auto, always, or never`) {
		t.Fatalf("color validation error = %v", err)
	}
}

func TestColorFlagStylesHelp(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("setup", "--color", "always", "-h")
	if err != nil {
		t.Fatalf("colored help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("colored help should not write stderr, got: %s", stderr)
	}
	if !strings.Contains(stdout, "\x1b[") {
		t.Fatalf("help with --color always did not include ANSI:\n%s", stdout)
	}

	stdout, stderr, err = executeBurpvalveCommand("setup", "--color", "never", "-h")
	if err != nil {
		t.Fatalf("plain help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("plain help should not write stderr, got: %s", stderr)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("help with --color never included ANSI:\n%s", stdout)
	}
}

func TestRobotsHelpIsStructuredJSON(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "init", "-h")
	if err != nil {
		t.Fatalf("robots init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots help should not write stderr, got: %s", stderr)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("robots help should not include ANSI:\n%s", stdout)
	}

	var doc struct {
		Schema      string         `json:"schema"`
		Command     string         `json:"command"`
		Use         string         `json:"use"`
		Flags       []robotFlag    `json:"flags"`
		GlobalFlags []robotFlag    `json:"global_flags"`
		RobotInput  map[string]any `json:"robot_input"`
		Notes       []string       `json:"notes"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode robots help: %v\n%s", err, stdout)
	}
	if doc.Schema != "burpvalve.robot_help.v1" || doc.Command != "burpvalve init" {
		t.Fatalf("unexpected robots help identity: %#v", doc)
	}
	if !robotHelpHasFlag(doc.Flags, "--force") || !robotHelpHasFlag(doc.Flags, "--no-beads") || !robotHelpHasFlag(doc.Flags, "--dogfood") || !robotHelpHasFlag(doc.Flags, "--no-dogfood") {
		t.Fatalf("robots init help missing command flags: %#v", doc.Flags)
	}
	if !robotHelpHasFlag(doc.GlobalFlags, "--robots") || !robotHelpHasFlag(doc.GlobalFlags, "--color") {
		t.Fatalf("robots init help missing global flags: %#v", doc.GlobalFlags)
	}
	if _, ok := doc.RobotInput["stdin_json"]; !ok {
		t.Fatalf("robots init help missing stdin_json schema: %#v", doc.RobotInput)
	}
	if !containsString(doc.Notes, "mutating commands require --force or stdin JSON with confirm=true") {
		t.Fatalf("robots init help missing confirmation note: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "dogfood=true adds findings-log instructions to ORCHESTRATOR.md when that target is created") {
		t.Fatalf("robots init help missing dogfood note: %#v", doc.Notes)
	}
	stdin, ok := doc.RobotInput["stdin_json"].(map[string]any)
	if !ok {
		t.Fatalf("robots init help missing stdin_json schema: %#v", doc.RobotInput)
	}
	if _, ok := stdin["dogfood"]; !ok {
		t.Fatalf("robots init help missing dogfood stdin field: %#v", stdin)
	}
	if _, ok := stdin["claude_route"]; !ok {
		t.Fatalf("robots init help missing claude_route stdin field: %#v", stdin)
	}
	body := string(stdout)
	for _, needle := range []string{
		`"claude_route"`,
		`"claude_route_source"`,
		`"created"`,
		`"repaired"`,
		"controls CLAUDE.md/.claude/skills route choice",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("robots init help missing %q:\n%s", needle, body)
		}
	}
}

func TestRobotsRepairHelpDocumentsClaudeRouteFields(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "repair", "-h")
	if err != nil {
		t.Fatalf("robots repair help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots repair help should not write stderr, got: %s", stderr)
	}
	var doc struct {
		Schema      string         `json:"schema"`
		Command     string         `json:"command"`
		Flags       []robotFlag    `json:"flags"`
		RobotInput  map[string]any `json:"robot_input"`
		RobotOutput map[string]any `json:"robot_output"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode robots repair help: %v\n%s", err, stdout)
	}
	if doc.Schema != "burpvalve.robot_help.v1" || doc.Command != "burpvalve repair" {
		t.Fatalf("unexpected robots repair help identity: %#v", doc)
	}
	if !robotHelpHasFlag(doc.Flags, "--claude-route") || !robotHelpHasFlag(doc.Flags, "--adopt-claude-md") {
		t.Fatalf("robots repair help missing route/adoption flags: %#v", doc.Flags)
	}
	stdin, ok := doc.RobotInput["stdin_json"].(map[string]any)
	if !ok {
		t.Fatalf("robots repair help missing stdin_json schema: %#v", doc.RobotInput)
	}
	for _, field := range []string{"claude_route", "adopt_claude_md"} {
		if _, ok := stdin[field]; !ok {
			t.Fatalf("robots repair help missing %s stdin field: %#v", field, stdin)
		}
	}
	body := string(stdout)
	for _, needle := range []string{
		`"claude_route"`,
		`"claude_route_source"`,
		`"created"`,
		`"repaired"`,
		"import an unmarked regular CLAUDE.md into AGENTS.md",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("robots repair help missing %q:\n%s", needle, body)
		}
	}
}

func TestRobotsConfigInitHelpDocumentsVerifierDefaults(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "config", "init", "-h")
	if err != nil {
		t.Fatalf("robots config init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots config init help should not write stderr, got: %s", stderr)
	}
	for _, needle := range []string{
		`"init"`,
		`"repair"`,
		`"orchestrator"`,
		`"claude_route"`,
		`"dogfood"`,
		"controls ORCHESTRATOR.md only, not the Claude route",
		"controls CLAUDE.md/.claude/skills route choice",
		"controls Claude route repair defaults",
		`"verifier"`,
		`"authorized"`,
		`"authorization_scope"`,
		`"spawn_method"`,
		`"transcripts"`,
		"policy metadata, never verifier evidence",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots config init help missing %q:\n%s", needle, stdout)
		}
	}
	if !strings.Contains(stdout, "findings-log instructions") {
		t.Fatalf("robots config init help missing dogfood explanation:\n%s", stdout)
	}
}

func TestRobotsVerifierDoctorHelpDocumentsReportOnlyOutput(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "verifier", "doctor", "-h")
	if err != nil {
		t.Fatalf("robots verifier doctor help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots verifier doctor help should not write stderr, got: %s", stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve verifier doctor"`,
		`"report_only"`,
		`"checks"`,
		`"supported"`,
		`"limits"`,
		"never writes runtime config",
		"supported=false",
		"never per-cell verifier evidence",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots verifier doctor help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestRobotsSetupHelpDocumentsReadinessOutput(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "setup", "-h")
	if err != nil {
		t.Fatalf("robots setup help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots setup help should not write stderr, got: %s", stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve setup"`,
		`"readiness_severity"`,
		`"command_path"`,
		`"repo_bin_path"`,
		`"hook_command_source"`,
		`"repo_local_binary"`,
		"freshness_status",
		"warning_code",
		`"config"`,
		"setup is inspection-only",
		"never initializes Git",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots setup help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestRobotsBeadsPreflightHelpDocumentsReadOnlyFlow(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "beads", "preflight", "-h")
	if err != nil {
		t.Fatalf("robots beads preflight help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots beads preflight help should not write stderr, got: %s", stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve beads preflight"`,
		`"admin-only"`,
		`"bead-rationale"`,
		`"classification"`,
		`"non_beads_payload_paths"`,
		`"mutating": "always false"`,
		"never closes beads, runs br sync, stages files, commits files, or writes attestations",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots beads preflight help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestRobotsBeadsCloseHelpDocumentsAdminAndMultiBeadFlow(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "beads", "close", "-h")
	if err != nil {
		t.Fatalf("robots beads close help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots beads close help should not write stderr, got: %s", stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve beads close"`,
		`"admin_only"`,
		"skips verifier/code attestation",
		`"bead_rationale"`,
		"multi-bead delivery rationale recorded into the final attestation",
		"delivery multi-bead closures require --bead-rationale",
		"admin-only closures require exclusively .beads staged paths",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots beads close help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestRobotsHelpCommandIsStructuredJSON(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "help", "init")
	if err != nil {
		t.Fatalf("robots help command failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots help command should not write stderr, got: %s", stderr)
	}
	var doc struct {
		Schema  string `json:"schema"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode robots help command: %v\n%s", err, stdout)
	}
	if doc.Schema != "burpvalve.robot_help.v1" || doc.Command != "burpvalve init" {
		t.Fatalf("unexpected robots help command output: %#v", doc)
	}
}

func TestRobotsVersionOutputsJSON(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "version")
	if err != nil {
		t.Fatalf("robots version failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots version should not write stderr, got: %s", stderr)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode robots version: %v\n%s", err, stdout)
	}
	if got["version"] == "" {
		t.Fatalf("robots version missing version: %#v", got)
	}
}

func robotHelpHasFlag(flags []robotFlag, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func containsStandaloneBurpLanguage(text string) bool {
	lowered := strings.ToLower(text)
	lowered = strings.ReplaceAll(lowered, "burpvalve", "")
	return strings.Contains(lowered, "burp") || strings.Contains(lowered, "burped")
}

func executeBurpvalveCommand(args ...string) (string, string, error) {
	previousColorMode := colorMode
	previousRobotsMode := robotsMode
	colorMode = "auto"
	robotsMode = false
	defer func() {
		colorMode = previousColorMode
		robotsMode = previousRobotsMode
	}()
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
