package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBeadsPreflightIsReadOnlyAndReportsDeliverySequence(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	writeCLIFile(t, root, "src/app.go", "package app\n")
	run(t, root, "git", "add", "src/app.go")
	before := runOutput(t, root, "git", "status", "--short")
	installFakeBR(t, "in_progress")

	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--root", root, "--json", "br-delivery")
	if err != nil {
		t.Fatalf("beads preflight should be advisory, not fatal for in-progress bead: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("beads preflight should not write stderr: %s", stderr)
	}
	after := runOutput(t, root, "git", "status", "--short")
	if after != before {
		t.Fatalf("beads preflight mutated git state\nbefore=%s\nafter=%s", before, after)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode beads preflight: %v\n%s", err, stdout)
	}
	if got.Mutating || got.Status != "action_needed" || !got.BRAvailable {
		t.Fatalf("unexpected preflight status: %#v", got)
	}
	if len(got.Beads) != 1 || got.Beads[0].ID != "br-delivery" || got.Beads[0].Status != "in_progress" {
		t.Fatalf("preflight did not report br state: %#v", got.Beads)
	}
	if !containsString(got.StagedPayloadPaths, "src/app.go") {
		t.Fatalf("preflight missing staged payload: %#v", got.StagedPayloadPaths)
	}
	if !containsString(got.NextSteps, "Run br sync --flush-only.") {
		t.Fatalf("preflight next steps missing br sync: %#v", got.NextSteps)
	}
	if len(got.Warnings) == 0 || !strings.Contains(got.Warnings[0], "close it only after") {
		t.Fatalf("preflight should warn about non-closed delivery bead: %#v", got.Warnings)
	}
}

func TestBeadsPreflightRequiresBRForBeadsSpecificCommand(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--json", "br-missing")
	if err == nil {
		t.Fatalf("beads preflight without br should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("json preflight should keep diagnostics in stdout, stderr=%s", stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode missing br preflight: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" || got.BRAvailable || len(got.NextSteps) == 0 {
		t.Fatalf("missing br report wrong: %#v", got)
	}
}

func TestBeadsPreflightMultipleBeadsRequireRationale(t *testing.T) {
	installFakeBR(t, "closed")
	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--json", "br-one", "br-two")
	if err == nil {
		t.Fatalf("multiple beads without rationale should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode multiple bead preflight: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" || !strings.Contains(strings.Join(got.Warnings, "\n"), "--bead-rationale is required") {
		t.Fatalf("multiple bead warning wrong: %#v", got)
	}
}

func TestBeadsPreflightMultipleBeadsAcceptsRationale(t *testing.T) {
	installFakeBR(t, "closed")
	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--json", "--bead-rationale", "same staged payload", "br-one", "br-two")
	if err != nil {
		t.Fatalf("multiple beads with rationale should pass: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode coupled bead preflight: %v\n%s", err, stdout)
	}
	if got.CoupledWorkRationale != "same staged payload" || len(got.BeadIDs) != 2 {
		t.Fatalf("coupled bead metadata wrong: %#v", got)
	}
}

func TestBeadsPreflightAdminOnlyDoesNotRequireStagedPayload(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	installFakeBR(t, "closed")
	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--root", root, "--admin-only", "--json", "admin-123")
	if err != nil {
		t.Fatalf("admin-only preflight should not require staged payload: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode admin-only preflight: %v\n%s", err, stdout)
	}
	if !got.AdminOnly || got.Status != "ready" || !containsString(got.NextSteps, "Do not fabricate implementation commit evidence for issue-only work.") {
		t.Fatalf("admin-only report wrong: %#v", got)
	}
}

func TestBeadsPreflightAdminOnlyRejectsNonBeadsStagedPayload(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	writeCLIFile(t, root, ".beads/issues.jsonl", "{}\n")
	writeCLIFile(t, root, "docs/readme.md", "delivery docs\n")
	run(t, root, "git", "add", ".beads/issues.jsonl", "docs/readme.md")
	installFakeBR(t, "closed")

	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--root", root, "--admin-only", "--json", "admin-123")
	if err == nil {
		t.Fatalf("admin-only preflight with non-Beads payload should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("json admin guard should keep diagnostics in stdout, stderr=%s", stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode admin guard preflight: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" || got.Classification != "delivery" {
		t.Fatalf("admin guard status/classification wrong: %#v", got)
	}
	if !containsString(got.NonBeadsPayloadPaths, "docs/readme.md") || !strings.Contains(strings.Join(got.Warnings, "\n"), "docs/readme.md") {
		t.Fatalf("admin guard did not name non-Beads path: %#v", got)
	}
}

func TestBeadsPreflightClassificationUsesStagedPayloadOverMetadata(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	writeCLIFile(t, root, ".beads/issues.jsonl", "{}\n")
	run(t, root, "git", "add", ".beads/issues.jsonl")
	installFakeBRJSON(t, `[{"id":"docs-123","title":"Docs bead","status":"closed","issue_type":"docs","labels":["docs"],"closed_at":"2026-07-02T12:00:00Z","close_reason":"done"}]`)

	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--root", root, "--json", "docs-123")
	if err != nil {
		t.Fatalf("metadata disagreement should warn, not block: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("classification preflight should not write stderr: %s", stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode classification preflight: %v\n%s", err, stdout)
	}
	if got.Classification != "admin" || !containsString(got.StagedPayloadPaths, ".beads/issues.jsonl") {
		t.Fatalf("classification should come from staged paths: %#v", got)
	}
	if len(got.Beads) != 1 || got.Beads[0].IssueType != "docs" || !containsString(got.Beads[0].Labels, "docs") || got.Beads[0].ClosedAt == "" || got.Beads[0].CloseReason == "" {
		t.Fatalf("inspect bead metadata not captured: %#v", got.Beads)
	}
	if !strings.Contains(strings.Join(got.Warnings, "\n"), "metadata suggests delivery work") {
		t.Fatalf("metadata disagreement warning missing: %#v", got.Warnings)
	}
	if !containsString(got.NextSteps, "Do not fabricate implementation commit evidence for issue-only work.") {
		t.Fatalf("admin classification should use admin next steps: %#v", got.NextSteps)
	}
}

func TestBeadsPreflightDeliveryCloseContextRequiresStagedPayload(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	installFakeBR(t, "closed")

	got, err := buildBeadsPreflightReport(beadsPreflightOptions{root: root, requireDeliveryPayload: true}, []string{"br-delivery"})
	if err == nil {
		t.Fatalf("close-context delivery without staged payload should fail: %#v", got)
	}
	if got.Status != "blocked" || got.Classification != "delivery" || !strings.Contains(strings.Join(got.Warnings, "\n"), "requires a staged payload") {
		t.Fatalf("close-context empty payload report wrong: %#v", got)
	}
}

func TestBeadsPreflightInvalidBeadIDReportsRecovery(t *testing.T) {
	installFailingBR(t)
	stdout, stderr, err := executeBurpvalveCommand("beads", "preflight", "--json", "br-missing")
	if err == nil {
		t.Fatalf("invalid bead id should block\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	var got beadsPreflightReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode invalid bead preflight: %v\n%s", err, stdout)
	}
	if got.Status != "blocked" || !strings.Contains(strings.Join(got.Warnings, "\n"), "br show br-missing failed") {
		t.Fatalf("invalid bead report wrong: %#v", got)
	}
}

func installFakeBR(t *testing.T, status string) {
	t.Helper()
	installFakeBRJSON(t, `[{"id":"%s","title":"Delivery bead","status":"`+status+`"}]`)
}

func installFakeBRJSON(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "br")
	writeBeadsExecutable(t, path, `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "show" ]]; then
  printf '`+body+`' "$2"
  exit 0
fi
echo "unexpected br args: $*" >&2
exit 2
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func installFailingBR(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "br")
	writeBeadsExecutable(t, path, `#!/usr/bin/env bash
echo "not found" >&2
exit 1
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeBeadsExecutable(t *testing.T, path string, body string) {
	t.Helper()
	writeCmdTestFile(t, path, body)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
