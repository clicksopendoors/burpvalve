package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/backpressure"
)

func TestAccountPayloadJSONClassifiesStagedOwnership(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "src/app.go", "package app\n\nconst AccountPayload = true\n")
	writeCLIFile(t, target, "backpressure/attestations/generated.json", "{}\n")
	run(t, target, "git", "add", "src/app.go", "backpressure/attestations/generated.json")

	input := `{"records":[
		{"unit_id":"ifr2-c2","path":"src/app.go","ownership_kind":"whole_path","source":"stdin"},
		{"unit_id":"ifr2-c2","path":"docs/split.go","ownership_kind":"function","symbol":"Build","source":"stdin","rationale":"declared split-file ownership"},
		{"unit_id":"cos-route","path":"docs/split.go","ownership_kind":"test","test_pattern":"Build","source":"stdin","rationale":"declared split-file ownership"},
		{"unit_id":"ifr2-c2","path":"docs/conflict.go","ownership_kind":"whole_path","source":"stdin","rationale":"shared cli surface"},
		{"unit_id":"cos-route","path":"docs/conflict.go","ownership_kind":"whole_path","source":"stdin","rationale":"shared cli surface"}
	]}`
	writeCLIFile(t, target, "docs/split.go", "package docs\n")
	writeCLIFile(t, target, "docs/conflict.go", "package docs\n")
	run(t, target, "git", "add", "docs/split.go", "docs/conflict.go")

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, input, "account", "payload", "--root", target, "--json")
	if err != nil {
		t.Fatalf("account payload failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("account payload should not write stderr, got: %s", stderr)
	}
	var result backpressure.OwnershipAccountingResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode account result: %v\n%s", err, stdout)
	}
	if result.Mutating || result.Command != "account payload" || result.Status != "completed" {
		t.Fatalf("unexpected account result identity: %#v", result)
	}
	assertOwnershipStatus(t, result.Staged, "src/app.go", backpressure.OwnershipStatusOwned)
	assertOwnershipStatus(t, result.Staged, "backpressure/manifest.yaml", backpressure.OwnershipStatusUnowned)
	assertOwnershipStatus(t, result.Staged, "docs/conflict.go", backpressure.OwnershipStatusConflict)
	assertOwnershipStatus(t, result.Staged, "docs/split.go", backpressure.OwnershipStatusSharedDeclared)
	assertOwnershipStatus(t, result.Staged, "backpressure/attestations/generated.json", backpressure.OwnershipStatusGeneratedException)
}

func TestAccountPayloadMergesFileAndStdinWithStdinPrecedence(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "src/app.go", "package app\n\nconst AccountPayload = true\n")
	run(t, target, "git", "add", "src/app.go")
	ownershipFile := filepath.Join(t.TempDir(), "ownership.json")
	if err := os.WriteFile(ownershipFile, []byte(`{"records":[{"unit_id":"ifr2-c2","path":"src/app.go","ownership_kind":"whole_path","source":"file"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdin := `{"records":[{"unit_id":"ifr2-c2","path":"src/app.go","ownership_kind":"whole_path","source":"stdin"}]}`

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, stdin, "account", "payload", "--root", target, "--ownership-file", ownershipFile, "--json")
	if err != nil {
		t.Fatalf("account payload failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.OwnershipAccountingResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode account result: %v\n%s", err, stdout)
	}
	got := findOwnershipResult(t, result.Staged, "src/app.go")
	if got.Source != "stdin" || len(got.Owners) != 1 || got.Owners[0].Source != "stdin" {
		t.Fatalf("stdin should override same file claim: %#v", got)
	}
}

func TestAccountPayloadReportsUntrackedAndIsReadOnly(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, ".gitignore", "scratch/*.tmp\n")
	run(t, target, "git", "add", ".gitignore")
	writeCLIFile(t, target, "scratch/ignored.tmp", "ignored\n")
	writeCLIFile(t, target, "src/new.go", "package app\n")
	writeCLIFile(t, target, "log/backpressure/failed/generated.json", "{}\n")
	ownership := `{"records":[{"unit_id":"ifr2-c2","path":"src/new.go","ownership_kind":"exception","source":"stdin","rationale":"operator scratch path reviewed"}]}`
	before := runOutput(t, target, "git", "status", "--short")

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, ownership, "account", "payload", "--root", target, "--include-untracked", "--json")
	if err != nil {
		t.Fatalf("account payload failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	after := runOutput(t, target, "git", "status", "--short")
	if after != before {
		t.Fatalf("account payload mutated git status:\nbefore=%s\nafter=%s", before, after)
	}
	var result backpressure.OwnershipAccountingResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode account result: %v\n%s", err, stdout)
	}
	assertOwnershipStatus(t, result.Untracked, "scratch/ignored.tmp", backpressure.OwnershipStatusIgnoredUntracked)
	assertOwnershipStatus(t, result.Untracked, "src/new.go", backpressure.OwnershipStatusCoveredException)
	assertOwnershipStatus(t, result.Untracked, "log/backpressure/failed/generated.json", backpressure.OwnershipStatusGeneratedException)
	if result.Summary.UntrackedTotal == 0 || result.Summary.Ignored == 0 || result.Summary.Generated == 0 || result.Summary.Covered == 0 {
		t.Fatalf("untracked summary missing counts: %#v", result.Summary)
	}
}

func TestAccountPayloadIncludesDisplayOnlyBeadsContext(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, ".beads/issues.jsonl", strings.Join([]string{
		`{"id":"burpvalve-ifr2-c3-beads-enrichment-docs-pe7s","title":"ifr2-C3","status":"in_progress","priority":2,"issue_type":"task","labels":["ifr2","ownership"]}`,
		`{"id":"closed-bead","title":"Closed","status":"closed","priority":1,"issue_type":"task"}`,
	}, "\n")+"\n")
	writeCLIFile(t, target, "docs/result-contract.md", "# Contract\n")
	writeCLIFile(t, target, "README.md", "# Readme\n")
	run(t, target, "git", "add", "docs/result-contract.md", "README.md")
	input := `{"records":[{"unit_id":"burpvalve-ifr2-c3-beads-enrichment-docs-pe7s","bead_id":"burpvalve-ifr2-c3-beads-enrichment-docs-pe7s","path":"docs/result-contract.md","ownership_kind":"whole_path","source":"stdin"}]}`

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, input, "account", "payload", "--root", target, "--include-beads", "--json")
	if err != nil {
		t.Fatalf("account payload failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.OwnershipAccountingResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode account result: %v\n%s", err, stdout)
	}
	if result.Beads == nil || !result.Beads.Available || !result.Beads.DisplayOnly || len(result.Beads.Active) != 1 {
		t.Fatalf("unexpected beads context: %#v", result.Beads)
	}
	if result.Beads.Active[0].ID != "burpvalve-ifr2-c3-beads-enrichment-docs-pe7s" || result.Beads.Active[0].Type != "task" {
		t.Fatalf("active bead display metadata missing: %#v", result.Beads.Active[0])
	}
	owned := findOwnershipResult(t, result.Staged, "docs/result-contract.md")
	if owned.Status != backpressure.OwnershipStatusOwned || len(owned.BeadsContext) != 1 {
		t.Fatalf("explicit owner should receive display-only bead context: %#v", owned)
	}
	unowned := findOwnershipResult(t, result.Staged, "README.md")
	if unowned.Status != backpressure.OwnershipStatusUnowned || len(unowned.Owners) != 0 || len(unowned.BeadsContext) != 0 {
		t.Fatalf("active Beads metadata must not create ownership: %#v", unowned)
	}
}

func TestAccountPayloadBeadsContextUnavailableOutsideBeadsRepo(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "src/app.go", "package app\n")
	run(t, target, "git", "add", "src/app.go")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "account", "payload", "--root", target, "--include-beads", "--json")
	if err != nil {
		t.Fatalf("account payload failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.OwnershipAccountingResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode account result: %v\n%s", err, stdout)
	}
	if result.Beads == nil || result.Beads.Available || !result.Beads.DisplayOnly || len(result.Beads.Warnings) == 0 {
		t.Fatalf("non-Beads repo should report unavailable display context: %#v", result.Beads)
	}
	assertOwnershipStatus(t, result.Staged, "src/app.go", backpressure.OwnershipStatusUnowned)
}

func TestAccountPayloadHumanAndRobotsHelp(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	stdout, stderr, err := runBurpvalve(t, repoRoot, "account", "payload", "--root", target, "--color", "never")
	if err != nil {
		t.Fatalf("human account payload failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Burpvalve payload ownership accounting",
		"Read-only: true",
		"Staged paths:",
		"unowned backpressure/manifest.yaml",
		"Summary:",
	} {
		if !strings.Contains(string(stdout), needle) {
			t.Fatalf("human output missing %q:\n%s", needle, stdout)
		}
	}

	help, helpStderr, err := executeBurpvalveCommand("--robots", "account", "payload", "-h")
	if err != nil {
		t.Fatalf("robots account help failed: %v\nstdout=%s\nstderr=%s", err, help, helpStderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve account payload"`,
		`"ownership-file"`,
		`"include-untracked"`,
		`"include-beads"`,
		`"mutating"`,
		`"staged"`,
		`"untracked"`,
		`"beads"`,
	} {
		if !strings.Contains(help, needle) {
			t.Fatalf("robots help missing %q:\n%s", needle, help)
		}
	}
}

func assertOwnershipStatus(t *testing.T, results []backpressure.OwnershipPathResult, path string, status backpressure.OwnershipStatus) {
	t.Helper()
	got := findOwnershipResult(t, results, path)
	if got.Status != status {
		t.Fatalf("%s status = %s, want %s; result=%#v", path, got.Status, status, got)
	}
}

func findOwnershipResult(t *testing.T, results []backpressure.OwnershipPathResult, path string) backpressure.OwnershipPathResult {
	t.Helper()
	for _, result := range results {
		if result.Path == path {
			return result
		}
	}
	t.Fatalf("path %q not found in %#v", path, results)
	return backpressure.OwnershipPathResult{}
}
