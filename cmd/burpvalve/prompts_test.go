package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/backpressure"
)

func TestPromptsListJSONAndHumanOutput(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("prompts", "list", "--json")
	if err != nil {
		t.Fatalf("prompts list json failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("prompts list json should not write stderr, got: %s", stderr)
	}
	var doc struct {
		Prompts []backpressure.PromptListItem `json:"prompts"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode prompts list: %v\n%s", err, stdout)
	}
	wantNames := []string{
		"bead-conversion",
		"bead-conversion-assignment",
		"commit-choreography",
		"cross-review-polish",
		"gate-operator-brief",
		"marching-orders",
		"orchestrator-tick",
		"packet-not-received-status",
		"plan-review-packet",
		"verifier-bootstrap",
		"verifier-brief",
		"verifier-packet-relay",
		"verifier-standby-brief",
	}
	if len(doc.Prompts) != len(wantNames) {
		t.Fatalf("unexpected prompt count: got %d want %d\n%#v", len(doc.Prompts), len(wantNames), doc.Prompts)
	}
	for i, want := range wantNames {
		if doc.Prompts[i].Name != want {
			t.Fatalf("unexpected prompt at index %d: got %q want %q\n%#v", i, doc.Prompts[i].Name, want, doc.Prompts)
		}
	}

	human, stderr, err := executeBurpvalveCommand("prompts", "list", "--color", "never")
	if err != nil {
		t.Fatalf("prompts list human failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	for _, needle := range []string{
		"Burpvalve prompts",
		"Canonical orchestrator templates",
		"burpvalve verifier prompts",
		"marching-orders",
		"verifier-bootstrap",
		"variables:",
	} {
		if !strings.Contains(human, needle) {
			t.Fatalf("prompts list missing %q:\n%s", needle, human)
		}
	}
}

func TestPromptsShowJSONContractAndVars(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "marching-orders",
		"--var", "agent=LilacGlacier",
		"--var", "bead=burpvalve-oxp-prompts-command-api-6lv",
		"--var", "track=OXP prompts",
		"--json")
	if err != nil {
		t.Fatalf("prompts show json failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("prompts show json should not write stderr, got: %s", stderr)
	}
	var doc backpressure.PromptShowOutput
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode prompts show: %v\n%s", err, stdout)
	}
	if doc.Name != "marching-orders" || doc.Version == "" || len(doc.Variables) != 3 {
		t.Fatalf("show contract missing metadata: %#v", doc)
	}
	if !strings.Contains(doc.Body, "LilacGlacier") || !strings.Contains(doc.Body, "burpvalve-oxp-prompts-command-api-6lv") {
		t.Fatalf("rendered body missing variables:\n%s", doc.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"name", "version", "variables", "body"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("show JSON missing %q: %s", key, stdout)
		}
	}
	if len(raw) != 4 {
		t.Fatalf("show JSON must contain exactly the promised fields, got %v", raw)
	}
}

func TestPromptsShowUsageErrorsAreComplete(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "marching-orders", "--var", "agent=LilacGlacier")
	if err == nil {
		t.Fatalf("prompts show missing vars should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "missing required variables") || !strings.Contains(err.Error(), "bead") {
		t.Fatalf("missing-variable error not actionable: %v\nstderr=%s", err, stderr)
	}

	_, stderr, err = executeBurpvalveCommand("prompts", "show", "missing")
	if err == nil {
		t.Fatal("unknown prompt should fail")
	}
	if !strings.Contains(err.Error(), "valid prompts:") || !strings.Contains(err.Error(), "marching-orders") || !strings.Contains(err.Error(), "verifier-bootstrap") {
		t.Fatalf("unknown prompt error should list valid names: %v\nstderr=%s", err, stderr)
	}
}

func TestPromptsVerifierBootstrapContent(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "verifier-bootstrap",
		"--var", "agent=WhiteGorge",
		"--var", "project_key=/path/to/burpvalve",
		"--var", "orchestrator=RusticDog",
		"--color", "never")
	if err != nil {
		t.Fatalf("prompts show verifier-bootstrap failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Read AGENTS.md and backpressure/README.md",
		"macro_start_session",
		"Request or confirm contact",
		"Poll your inbox",
		"priority work",
		"burpvalve verifier prompts --feature <feature-id> --json",
		"pass, not_applicable, fail, or unknown",
		"Do not fabricate confirmations",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("verifier-bootstrap output missing %q:\n%s", needle, stdout)
		}
	}
}

func TestPromptsShowHostileValuesRenderLiterally(t *testing.T) {
	hostile := "br 123 'quoted' `tick` $status && rm -rf /"
	stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "marching-orders",
		"--var", "agent=LilacGlacier",
		"--var", "bead="+hostile,
		"--color", "never")
	if err != nil {
		t.Fatalf("prompts show hostile values failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, hostile) {
		t.Fatalf("hostile value should render literally:\n%s", stdout)
	}
}

func TestPromptsRobotsHelpDistinguishesVerifierPrompts(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "prompts", "show", "-h")
	if err != nil {
		t.Fatalf("robots prompts help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots prompts help should not write stderr: %s", stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve prompts show"`,
		"stable prompt name",
		"burpvalve verifier prompts is different",
		"staged-payload verifier packets",
		"--write",
		"--force",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots prompts help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestPromptsWriteCreatesExportWithContentHash(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "marching-orders",
		"--root", root,
		"--var", "agent=LilacGlacier",
		"--var", "bead=burpvalve-oxp-prompt-export-divergence-9le",
		"--write",
		"--json")
	if err != nil {
		t.Fatalf("prompts show --write failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("prompts show --write should not warn on first export: %s", stderr)
	}
	var result backpressure.PromptExportResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode prompt export: %v\n%s", err, stdout)
	}
	if !result.Written || result.PromptName != "marching-orders" || result.ContentHash == "" {
		t.Fatalf("unexpected export result: %#v", result)
	}
	exportPath := filepath.Join(root, "docs", "prompts", "marching-orders.md")
	body := readTestFile(t, exportPath)
	for _, needle := range []string{
		`burpvalve_version: "dev"`,
		`prompt_name: "marching-orders"`,
		`content_hash: "sha256:`,
		"Role: LilacGlacier",
		"Work unit: burpvalve-oxp-prompt-export-divergence-9le",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("export missing %q:\n%s", needle, body)
		}
	}
}

func TestPromptsWriteIdempotentWhenUnmodified(t *testing.T) {
	root := t.TempDir()
	args := []string{
		"prompts", "show", "verifier-brief",
		"--root", root,
		"--var", "feature=br-123",
		"--write",
		"--json",
	}
	if stdout, stderr, err := executeBurpvalveCommand(args...); err != nil {
		t.Fatalf("first export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	stdout, stderr, err := executeBurpvalveCommand(args...)
	if err != nil {
		t.Fatalf("second export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.PromptExportResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode second export: %v\n%s", err, stdout)
	}
	if result.Written || result.LocalModified || result.Divergent {
		t.Fatalf("second export should be unchanged: %#v", result)
	}
}

func TestPromptsWriteRefusesModifiedExportWithoutForce(t *testing.T) {
	root := t.TempDir()
	args := []string{
		"prompts", "show", "marching-orders",
		"--root", root,
		"--var", "agent=LilacGlacier",
		"--var", "bead=burpvalve-oxp-prompt-export-divergence-9le",
		"--write",
	}
	if stdout, stderr, err := executeBurpvalveCommand(args...); err != nil {
		t.Fatalf("initial export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	exportPath := filepath.Join(root, "docs", "prompts", "marching-orders.md")
	if err := os.WriteFile(exportPath, []byte(readTestFile(t, exportPath)+"local modification\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := executeBurpvalveCommand(args...)
	if err == nil {
		t.Fatalf("modified export should fail without --force\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "locally modified prompt export") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("refusal should explain force recovery: %v\nstderr=%s", err, stderr)
	}
}

func TestPromptsWriteForceOverwritesModifiedExport(t *testing.T) {
	root := t.TempDir()
	base := []string{
		"prompts", "show", "marching-orders",
		"--root", root,
		"--var", "agent=LilacGlacier",
		"--var", "bead=burpvalve-oxp-prompt-export-divergence-9le",
		"--write",
	}
	if stdout, stderr, err := executeBurpvalveCommand(base...); err != nil {
		t.Fatalf("initial export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	exportPath := filepath.Join(root, "docs", "prompts", "marching-orders.md")
	if err := os.WriteFile(exportPath, []byte(readTestFile(t, exportPath)+"local modification\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := executeBurpvalveCommand(append(base, "--force", "--json")...)
	if err != nil {
		t.Fatalf("force export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.PromptExportResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode force export: %v\n%s", err, stdout)
	}
	if !result.Written || !result.LocalModified {
		t.Fatalf("force result should record overwrite of modified export: %#v", result)
	}
	if body := readTestFile(t, exportPath); strings.Contains(body, "local modification") {
		t.Fatalf("force export did not overwrite local modification:\n%s", body)
	}
}

func TestPromptsShowWarnsDivergentExportAndRendersCanonical(t *testing.T) {
	root := t.TempDir()
	if stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "marching-orders",
		"--root", root,
		"--var", "agent=LilacGlacier",
		"--var", "bead=canonical",
		"--write"); err != nil {
		t.Fatalf("initial export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	stdout, stderr, err := executeBurpvalveCommand("prompts", "show", "marching-orders",
		"--root", root,
		"--var", "agent=LilacGlacier",
		"--var", "bead=updated",
		"--color", "never")
	if err != nil {
		t.Fatalf("show with divergent export failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning: local prompt export") || !strings.Contains(stderr, "embedded canonical") {
		t.Fatalf("show should warn about divergent local export, got stderr=%s", stderr)
	}
	if !strings.Contains(stdout, "Work unit: updated") || strings.Contains(stdout, "Work unit: canonical") {
		t.Fatalf("show should render embedded canonical prompt for current vars:\n%s", stdout)
	}
}

func TestRepairLeavesPromptExportsUntouched(t *testing.T) {
	repoRoot := findRepoRoot(t)
	root := t.TempDir()
	exportPath := filepath.Join(root, "docs", "prompts", "marching-orders.md")
	if err := os.MkdirAll(filepath.Dir(exportPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "---\nprompt_name: \"marching-orders\"\ncontent_hash: \"sha256:local\"\n---\n\nlocal copy\n"
	if err := os.WriteFile(exportPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runBurpvalve(t, repoRoot, "repair", "--target", root, "--force", "--json", "docs")
	if err != nil {
		t.Fatalf("repair failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got := readTestFile(t, exportPath); got != original {
		t.Fatalf("repair should leave prompt exports untouched:\n%s", got)
	}
}
