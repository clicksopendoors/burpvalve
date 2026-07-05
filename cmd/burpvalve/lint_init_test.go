package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/backpressure"
	"burpvalve/internal/charmui"
	"burpvalve/internal/lintconfig"
)

func TestLintInitHelpDocumentsFlags(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("lint", "init", "-h")
	if err != nil {
		t.Fatalf("lint init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{"Detect Go and Node/Astro lint setup", "--detect", "--write", "--preset", "--force", "--root", "--json", "--jobs"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("lint init help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestRobotsLintInitHelpDocumentsSchemas(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "lint", "init", "-h")
	if err != nil {
		t.Fatalf("robots lint init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots lint init help wrote stderr: %s", stderr)
	}
	for _, needle := range []string{"confirm", "proposed_commands", "manifest_update", "lint init --detect is always read-only"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots lint init help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestLintJSONCompatAndJobsFlag(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "lint", "--root", target, "--json", "--jobs", "2")
	if err != nil {
		t.Fatalf("lint --json --jobs failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("lint --json should not write stderr: %s", stderr)
	}
	var result struct {
		Command string `json:"command"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode lint JSON: %v\n%s", err, stdout)
	}
	if result.Command != "lint" || result.Status == "" {
		t.Fatalf("unexpected lint JSON compatibility result: %#v", result)
	}

	_, _, err = runBurpvalve(t, repoRoot, "lint", "--root", target, "--json", "--jobs", "0")
	if err == nil {
		t.Fatal("lint --jobs 0 should fail")
	}
}

func TestLintInitDetectNeverMutates(t *testing.T) {
	repoRoot := findRepoRoot(t)
	root := lintInitFixture(t)
	beforeManifest := readLintInitFile(t, root, "backpressure/manifest.yaml")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "lint", "init", "--root", root, "--json", "--detect", "--write", "--force", "--preset", "go")
	if err != nil {
		t.Fatalf("lint init detect failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result lintInitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode lint init result: %v\n%s", err, stdout)
	}
	if result.Status != "detected" || result.Mutating {
		t.Fatalf("detect should be read-only detected status: %#v", result)
	}
	if got := readLintInitFile(t, root, "backpressure/manifest.yaml"); got != beforeManifest {
		t.Fatalf("--detect must not mutate manifest\nbefore:\n%s\nafter:\n%s", beforeManifest, got)
	}
	if _, err := os.Stat(filepath.Join(root, "backpressure/lint-rules.md")); !os.IsNotExist(err) {
		t.Fatalf("--detect must not create lint-rules.md, err=%v", err)
	}
}

func TestLintInitWriteRequiresConfirmation(t *testing.T) {
	repoRoot := findRepoRoot(t)
	root := lintInitFixture(t)
	beforeManifest := readLintInitFile(t, root, "backpressure/manifest.yaml")

	stdout, _, err := runBurpvalve(t, repoRoot, "lint", "init", "--root", root, "--json", "--write", "--preset", "go")
	if err == nil {
		t.Fatal("lint init --write without force or robot confirmation should fail closed")
	}
	var result lintInitResult
	if decodeErr := json.Unmarshal(stdout, &result); decodeErr != nil {
		t.Fatalf("decode confirmation result: %v\n%s", decodeErr, stdout)
	}
	if !result.ConfirmationRequired || !result.Fatal {
		t.Fatalf("expected confirmation_required fatal result: %#v", result)
	}
	if got := readLintInitFile(t, root, "backpressure/manifest.yaml"); got != beforeManifest {
		t.Fatalf("unconfirmed write mutated manifest\nbefore:\n%s\nafter:\n%s", beforeManifest, got)
	}
}

func TestRobotsLintInitRequiresConfirmForWrite(t *testing.T) {
	repoRoot := findRepoRoot(t)
	root := lintInitFixture(t)
	beforeManifest := readLintInitFile(t, root, "backpressure/manifest.yaml")
	input := `{"root":` + mustJSONString(t, root) + `,"write":true,"preset":"go"}`

	stdout, _, err := runBurpvalveWithInput(t, repoRoot, input, "--robots", "lint", "init")
	if err == nil {
		t.Fatal("robots lint init write without confirm should fail")
	}
	var result lintInitResult
	if decodeErr := json.Unmarshal([]byte(stdout), &result); decodeErr != nil {
		t.Fatalf("decode robots no-confirm result: %v\n%s", decodeErr, stdout)
	}
	if !result.ConfirmationRequired {
		t.Fatalf("robots no-confirm should require confirmation: %#v", result)
	}
	if got := readLintInitFile(t, root, "backpressure/manifest.yaml"); got != beforeManifest {
		t.Fatalf("robots no-confirm mutated manifest\nbefore:\n%s\nafter:\n%s", beforeManifest, got)
	}
}

func TestRobotsLintInitConfirmWrites(t *testing.T) {
	repoRoot := findRepoRoot(t)
	root := lintInitFixture(t)
	input := `{"root":` + mustJSONString(t, root) + `,"write":true,"preset":"go","confirm":true}`

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, input, "--robots", "lint", "init")
	if err != nil {
		t.Fatalf("robots lint init confirmed write failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result lintInitResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode robots confirmed result: %v\n%s", err, stdout)
	}
	if result.Status != "written" || !result.Mutating {
		t.Fatalf("confirmed robot write should report written mutating result: %#v", result)
	}
	manifest := readLintInitFile(t, root, "backpressure/manifest.yaml")
	for _, needle := range []string{"id: go-test", "id: go-vet", "id: go-fmt-check"} {
		if !strings.Contains(manifest, needle) {
			t.Fatalf("manifest missing %q:\n%s", needle, manifest)
		}
	}
}

func TestLintInitPresetFiltering(t *testing.T) {
	root := lintInitFixture(t)
	writeCLIFile(t, root, "package.json", `{"scripts":{"lint":"eslint .","test":"vitest","format":"prettier ."}}`)
	writeCLIFile(t, root, "package-lock.json", "{}")

	goResult, err := buildLintInitResult(lintInitOptions{root: root, preset: "go", jobs: 1})
	if err != nil {
		t.Fatalf("build go lint init result: %v", err)
	}
	if len(goResult.ProposedCommands) == 0 {
		t.Fatal("go preset should propose Go commands")
	}
	for _, command := range goResult.ProposedCommands {
		if !strings.HasPrefix(command.ID, "go-") {
			t.Fatalf("go preset included non-Go command: %#v", command)
		}
	}

	nodeResult, err := buildLintInitResult(lintInitOptions{root: root, preset: "node", jobs: 1})
	if err != nil {
		t.Fatalf("build node lint init result: %v", err)
	}
	if len(nodeResult.ProposedCommands) == 0 {
		t.Fatal("node preset should propose Node commands")
	}
	for _, command := range nodeResult.ProposedCommands {
		if strings.HasPrefix(command.ID, "go-") {
			t.Fatalf("node preset included Go command: %#v", command)
		}
	}
}

func TestLintInitWizardDeclinedScopedSetupRecordsCoverageOnConfirmedWrite(t *testing.T) {
	root := lintInitPolyglotFixture(t)
	result, err := buildLintInitResult(lintInitOptions{
		root: root,
		jobs: 1,
		wizardPlan: &lintInitWizardPlan{
			SelectedIDs: map[string]bool{
				"go-test":      true,
				"go-vet":       true,
				"go-fmt-check": true,
			},
			Commands: map[string]lintconfig.Command{},
			DeclinedRoots: []string{
				"web",
			},
			DeclinedAt: "2026-07-02T16:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("build declined scoped result: %v", err)
	}
	if result.Coverage.Status != "partial" || !result.Coverage.NeedsScopedSetup {
		t.Fatalf("declined scoped setup should report partial scoped coverage: %#v", result.Coverage)
	}
	if got := strings.Join(result.Coverage.DeclinedRoots, ","); got != "web" {
		t.Fatalf("declined roots = %q", got)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "unchecked roots: web") {
		t.Fatalf("declined scoped setup should warn about unchecked roots: %#v", result.Warnings)
	}

	err = runLintInit(lintInitOptions{
		root:  root,
		write: true,
		force: true,
		jobs:  1,
		now:   fixedLintInitNow,
		wizardPlan: &lintInitWizardPlan{
			SelectedIDs: map[string]bool{
				"go-test":      true,
				"go-vet":       true,
				"go-fmt-check": true,
			},
			Commands:      map[string]lintconfig.Command{},
			DeclinedRoots: []string{"web"},
			DeclinedAt:    "2026-07-02T16:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("confirmed declined scoped write failed: %v", err)
	}
	manifest := readLintInitFile(t, root, "backpressure/manifest.yaml")
	for _, needle := range []string{"lint_coverage:", "declined_roots:", "- web", "declined_at: \"2026-07-02T16:00:00Z\""} {
		if !strings.Contains(manifest, needle) {
			t.Fatalf("manifest missing declined coverage %q:\n%s", needle, manifest)
		}
	}
}

func TestLintInitWizardAcceptedScopedSetupCoversPolyglotRoots(t *testing.T) {
	root := lintInitPolyglotFixture(t)
	result, err := buildLintInitResult(lintInitOptions{root: root, jobs: 1})
	if err != nil {
		t.Fatalf("build accepted scoped result: %v", err)
	}
	if !result.Coverage.NeedsScopedSetup || result.Coverage.Status != "full" {
		t.Fatalf("polyglot accepted setup should need scope and cover all detected roots: %#v", result.Coverage)
	}
	covered := strings.Join(result.Coverage.CoveredRoots, ",")
	if !strings.Contains(covered, ".") || !strings.Contains(covered, "web") {
		t.Fatalf("coverage should include repo root and web root: %#v", result.Coverage)
	}
}

func TestLintInitWizardSingleRootDoesNotNeedScopedQuestion(t *testing.T) {
	root := lintInitFixture(t)
	result, err := buildLintInitResult(lintInitOptions{root: root, jobs: 1})
	if err != nil {
		t.Fatalf("build single-root result: %v", err)
	}
	if result.Coverage.NeedsScopedSetup || result.Coverage.MultiRoot {
		t.Fatalf("single-root Go setup should not ask scoped setup question: %#v", result.Coverage)
	}
}

func TestLintInitWizardConfirmDefaultNoAndCancelDoesNotMutate(t *testing.T) {
	root := lintInitFixture(t)
	beforeManifest := readLintInitFile(t, root, "backpressure/manifest.yaml")
	result := lintInitResult{
		Root: root,
		Detection: backpressure.LintDetection{
			GoRoots: []backpressure.LintGoRoot{{Path: "."}},
		},
		ProposedCommands: []lintconfig.Command{{
			ID:             "go-test",
			Command:        "go test ./...",
			Required:       true,
			Paths:          []string{"."},
			TimeoutSeconds: 120,
			RunDirectory:   ".",
		}},
		ManifestUpdate: lintInitFileUpdate{Changed: true, After: "lint_commands: []\n"},
	}
	var confirmPrompts []charmui.ConfirmPrompt
	restore := stubLintInitPrompts(t,
		func(prompt charmui.ConfirmPrompt) (bool, error) {
			confirmPrompts = append(confirmPrompts, prompt)
			if prompt.Prompt == "Write these lint setup changes?" {
				return false, nil
			}
			return prompt.Default, nil
		},
		func(prompt charmui.TextPrompt) (string, error) {
			return prompt.Default, nil
		},
	)
	defer restore()
	_, confirmed, err := collectLintInitWizardPlan(result, lintInitOptions{root: root, jobs: 1})
	if err != nil {
		t.Fatalf("collect wizard plan: %v", err)
	}
	if confirmed {
		t.Fatal("final wizard confirmation should have defaulted to No in this test")
	}
	if len(confirmPrompts) == 0 || confirmPrompts[len(confirmPrompts)-1].Prompt != "Write these lint setup changes?" || confirmPrompts[len(confirmPrompts)-1].Default {
		t.Fatalf("final confirmation should be write prompt with default No: %#v", confirmPrompts)
	}
	preview := confirmPrompts[len(confirmPrompts)-1].Description
	for _, needle := range []string{"Selected commands:", "- go-test: go test ./...", "Manifest changed: true", "Manifest after:", "Default is No."} {
		if !strings.Contains(preview, needle) {
			t.Fatalf("wizard preview missing %q:\n%s", needle, preview)
		}
	}
	if got := readLintInitFile(t, root, "backpressure/manifest.yaml"); got != beforeManifest {
		t.Fatalf("cancelled wizard mutated manifest\nbefore:\n%s\nafter:\n%s", beforeManifest, got)
	}
}

func lintInitFixture(t *testing.T) string {
	t.Helper()
	root := fixtureGitRepo(t)
	writeCLIFile(t, root, "go.mod", "module example.test/lintinit\n\ngo 1.24\n")
	return root
}

func lintInitPolyglotFixture(t *testing.T) string {
	t.Helper()
	root := lintInitFixture(t)
	writeCLIFile(t, root, "web/package.json", `{"scripts":{"lint":"eslint .","test":"vitest"}}`)
	writeCLIFile(t, root, "web/package-lock.json", "{}")
	return root
}

func readLintInitFile(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

func fixedLintInitNow() time.Time {
	return time.Date(2026, 7, 2, 16, 0, 0, 0, time.UTC)
}

func stubLintInitPrompts(t *testing.T, confirm func(charmui.ConfirmPrompt) (bool, error), text func(charmui.TextPrompt) (string, error)) func() {
	t.Helper()
	prior := lintInitPrompts
	lintInitPrompts = lintInitPromptIO{confirm: confirm, text: text}
	return func() {
		lintInitPrompts = prior
	}
}
